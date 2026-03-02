package tmdb

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ngenohkevin/flickmind/internal/ai"
)

func (c *Client) FindInTMDB(ctx context.Context, rec ai.Recommendation) *SearchResult {
	var result *SearchResult

	// Search movies
	if rec.Type == "movie" || rec.Type == "anime" {
		result = c.searchMovieWithRetry(ctx, rec.Title, rec.Year)
	}

	// Search TV
	if result == nil && (rec.Type == "series" || rec.Type == "anime") {
		result = c.searchTVWithRetry(ctx, rec.Title, rec.Year)
	}

	// If type was movie but no results, try TV (and vice versa)
	if result == nil && rec.Type == "movie" {
		result = c.searchTVWithRetry(ctx, rec.Title, rec.Year)
	} else if result == nil && rec.Type == "series" {
		result = c.searchMovieWithRetry(ctx, rec.Title, rec.Year)
	}

	if result == nil {
		log.Printf("[TMDB] Could not find: %q (%d)", rec.Title, rec.Year)
		return nil
	}

	// Fetch IMDB ID (non-fatal on error)
	imdbID, err := c.GetExternalIDs(ctx, result.ID, result.MediaType)
	if err == nil && imdbID != "" {
		result.IMDBId = imdbID
	}

	return result
}

func (c *Client) searchMovieWithRetry(ctx context.Context, title string, year int) *SearchResult {
	movies, err := c.SearchMovies(ctx, title, year)
	if err == nil && len(movies) > 0 {
		var match *tmdbMovieResult
		for i := range movies {
			if extractYear(movies[i].ReleaseDate) == year {
				match = &movies[i]
				break
			}
		}
		if match == nil {
			match = &movies[0]
		}
		return movieToResult(match)
	}
	// Retry without year if AI gave a slightly wrong year
	if year > 0 {
		movies, err = c.SearchMovies(ctx, title, 0)
		if err == nil && len(movies) > 0 {
			return movieToResult(&movies[0])
		}
	}
	return nil
}

func (c *Client) searchTVWithRetry(ctx context.Context, title string, year int) *SearchResult {
	shows, err := c.SearchTV(ctx, title, year)
	if err == nil && len(shows) > 0 {
		var match *tmdbTVResult
		for i := range shows {
			if extractYear(shows[i].FirstAirDate) == year {
				match = &shows[i]
				break
			}
		}
		if match == nil {
			match = &shows[0]
		}
		return tvToResult(match)
	}
	// Retry without year if AI gave a slightly wrong year
	if year > 0 {
		shows, err = c.SearchTV(ctx, title, 0)
		if err == nil && len(shows) > 0 {
			return tvToResult(&shows[0])
		}
	}
	return nil
}

func (c *Client) EnrichRecommendations(ctx context.Context, recs []ai.Recommendation) []SearchResult {
	type indexedResult struct {
		index  int
		result *SearchResult
	}

	var mu sync.Mutex
	var results []indexedResult
	var wg sync.WaitGroup

	sem := make(chan struct{}, 8) // limit concurrency

	for i, rec := range recs {
		wg.Add(1)
		go func(idx int, r ai.Recommendation) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := c.FindInTMDB(ctx, r)
			if result != nil {
				result.Reason = r.Reason
				mu.Lock()
				results = append(results, indexedResult{idx, result})
				mu.Unlock()
			}
		}(i, rec)
	}
	wg.Wait()

	// Sort by original index to preserve AI ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	// Deduplicate by ID
	seen := make(map[int]bool)
	var enriched []SearchResult
	for _, r := range results {
		if !seen[r.result.ID] {
			seen[r.result.ID] = true
			enriched = append(enriched, *r.result)
		}
	}

	return enriched
}

// DiscoverFallback returns popular content from TMDB discover when no AI is available
func (c *Client) DiscoverFallback(ctx context.Context, mediaType string, genres []string, minRating float64, yearFrom, yearTo int) []SearchResult {
	params := url.Values{
		"sort_by":        {"popularity.desc"},
		"vote_count.gte": {"100"},
		"include_adult":  {"false"},
		"language":       {"en-US"},
	}

	if minRating > 0 {
		params.Set("vote_average.gte", fmt.Sprintf("%.1f", minRating))
	}

	genreIDs := mapGenreNamesToIDs(genres, mediaType)
	if len(genreIDs) > 0 {
		params.Set("with_genres", strings.Join(genreIDs, ","))
	}

	// Apply year range filters
	if yearFrom > 0 || yearTo > 0 {
		if mediaType == "series" {
			if yearFrom > 0 {
				params.Set("first_air_date.gte", fmt.Sprintf("%d-01-01", yearFrom))
			}
			if yearTo > 0 {
				params.Set("first_air_date.lte", fmt.Sprintf("%d-12-31", yearTo))
			}
		} else {
			if yearFrom > 0 {
				params.Set("primary_release_date.gte", fmt.Sprintf("%d-01-01", yearFrom))
			}
			if yearTo > 0 {
				params.Set("primary_release_date.lte", fmt.Sprintf("%d-12-31", yearTo))
			}
		}
	}

	var results []SearchResult

	if mediaType == "movie" || mediaType == "" {
		movies, err := c.DiscoverMovies(ctx, params)
		if err == nil {
			for i := range movies {
				results = append(results, *movieToResult(&movies[i]))
			}
		}
	}

	if mediaType == "series" || mediaType == "" {
		shows, err := c.DiscoverTV(ctx, params)
		if err == nil {
			for i := range shows {
				results = append(results, *tvToResult(&shows[i]))
			}
		}
	}

	return results
}

func movieToResult(m *tmdbMovieResult) *SearchResult {
	return &SearchResult{
		ID:           m.ID,
		Title:        m.Title,
		MediaType:    "movie",
		PosterPath:   m.PosterPath,
		BackdropPath: m.BackdropPath,
		Overview:     m.Overview,
		VoteAverage:  m.VoteAverage,
		VoteCount:    m.VoteCount,
		Year:         extractYear(m.ReleaseDate),
		GenreIDs:     m.GenreIDs,
		Popularity:   m.Popularity,
	}
}

func tvToResult(t *tmdbTVResult) *SearchResult {
	return &SearchResult{
		ID:           t.ID,
		Title:        t.Name,
		MediaType:    "tv",
		PosterPath:   t.PosterPath,
		BackdropPath: t.BackdropPath,
		Overview:     t.Overview,
		VoteAverage:  t.VoteAverage,
		VoteCount:    t.VoteCount,
		Year:         extractYear(t.FirstAirDate),
		GenreIDs:     t.GenreIDs,
		Popularity:   t.Popularity,
	}
}

func extractYear(dateStr string) int {
	if len(dateStr) >= 4 {
		if y, err := strconv.Atoi(dateStr[:4]); err == nil {
			return y
		}
	}
	return 0
}

// Genre name to TMDB ID mapping
var movieGenreMap = map[string]int{
	"action": 28, "adventure": 12, "animation": 16, "comedy": 35,
	"crime": 80, "documentary": 99, "drama": 18, "fantasy": 14,
	"horror": 27, "mystery": 9648, "romance": 10749, "sci-fi": 878,
	"science fiction": 878, "thriller": 53, "war": 10752,
}

var tvGenreMap = map[string]int{
	"action": 10759, "adventure": 10759, "animation": 16, "comedy": 35,
	"crime": 80, "documentary": 99, "drama": 18, "fantasy": 10765,
	"mystery": 9648, "sci-fi": 10765, "science fiction": 10765,
	"war": 10768,
}

// Reverse maps: TMDB genre ID → display name
var movieGenreIDToName = map[int]string{
	28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy",
	80: "Crime", 99: "Documentary", 18: "Drama", 14: "Fantasy",
	27: "Horror", 9648: "Mystery", 10749: "Romance", 878: "Sci-Fi",
	53: "Thriller", 10752: "War", 36: "History", 10402: "Music",
	10751: "Family", 37: "Western",
}

var tvGenreIDToName = map[int]string{
	10759: "Action & Adventure", 16: "Animation", 35: "Comedy",
	80: "Crime", 99: "Documentary", 18: "Drama", 10765: "Sci-Fi & Fantasy",
	9648: "Mystery", 10768: "War & Politics", 10762: "Kids",
	10763: "News", 10764: "Reality", 10766: "Soap", 10767: "Talk",
	37: "Western",
}

// GenreIDsToNames converts TMDB genre IDs to human-readable names.
func GenreIDsToNames(ids []int, mediaType string) []string {
	m := movieGenreIDToName
	if mediaType == "tv" {
		m = tvGenreIDToName
	}
	var names []string
	for _, id := range ids {
		if name, ok := m[id]; ok {
			names = append(names, name)
		}
	}
	return names
}

func mapGenreNamesToIDs(names []string, mediaType string) []string {
	genreMap := movieGenreMap
	if mediaType == "series" {
		genreMap = tvGenreMap
	}

	var ids []string
	seen := make(map[int]bool)
	for _, name := range names {
		if id, ok := genreMap[strings.ToLower(name)]; ok && !seen[id] {
			seen[id] = true
			ids = append(ids, fmt.Sprintf("%d", id))
		}
	}
	return ids
}
