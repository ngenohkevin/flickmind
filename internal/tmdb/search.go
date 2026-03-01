package tmdb

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/ngenohkevin/flickmind/internal/ai"
)

func (c *Client) FindInTMDB(ctx context.Context, rec ai.Recommendation) *SearchResult {
	// Search movies
	if rec.Type == "movie" || rec.Type == "anime" {
		movies, err := c.SearchMovies(ctx, rec.Title, rec.Year)
		if err == nil && len(movies) > 0 {
			// Try exact year match first
			var match *tmdbMovieResult
			for i := range movies {
				if extractYear(movies[i].ReleaseDate) == rec.Year {
					match = &movies[i]
					break
				}
			}
			if match == nil {
				match = &movies[0]
			}
			return movieToResult(match)
		}
	}

	// Search TV
	if rec.Type == "series" || rec.Type == "anime" {
		shows, err := c.SearchTV(ctx, rec.Title, rec.Year)
		if err == nil && len(shows) > 0 {
			var match *tmdbTVResult
			for i := range shows {
				if extractYear(shows[i].FirstAirDate) == rec.Year {
					match = &shows[i]
					break
				}
			}
			if match == nil {
				match = &shows[0]
			}
			return tvToResult(match)
		}
	}

	// If type was movie but no results, try TV (and vice versa)
	if rec.Type == "movie" {
		shows, err := c.SearchTV(ctx, rec.Title, rec.Year)
		if err == nil && len(shows) > 0 {
			return tvToResult(&shows[0])
		}
	} else if rec.Type == "series" {
		movies, err := c.SearchMovies(ctx, rec.Title, rec.Year)
		if err == nil && len(movies) > 0 {
			return movieToResult(&movies[0])
		}
	}

	log.Printf("[TMDB] Could not find: %q (%d)", rec.Title, rec.Year)
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

	sem := make(chan struct{}, 5) // limit concurrency

	for i, rec := range recs {
		wg.Add(1)
		go func(idx int, r ai.Recommendation) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := c.FindInTMDB(ctx, r)
			if result != nil {
				result.Overview = r.Reason + "\n\n" + result.Overview
				mu.Lock()
				results = append(results, indexedResult{idx, result})
				mu.Unlock()
			}
		}(i, rec)
	}
	wg.Wait()

	// Deduplicate by ID, keep order
	seen := make(map[int]bool)
	var enriched []SearchResult
	// Sort by original index to preserve AI ordering
	for i := 0; i < len(recs); i++ {
		for _, r := range results {
			if r.index == i && !seen[r.result.ID] {
				seen[r.result.ID] = true
				enriched = append(enriched, *r.result)
			}
		}
	}

	return enriched
}

// DiscoverFallback returns popular content from TMDB discover when no AI is available
func (c *Client) DiscoverFallback(ctx context.Context, mediaType string, genres []string, minRating float64) []SearchResult {
	params := url.Values{
		"sort_by":                {"popularity.desc"},
		"vote_count.gte":        {"100"},
		"include_adult":         {"false"},
		"language":              {"en-US"},
	}

	if minRating > 0 {
		params.Set("vote_average.gte", fmt.Sprintf("%.1f", minRating))
	}

	genreIDs := mapGenreNamesToIDs(genres, mediaType)
	if len(genreIDs) > 0 {
		params.Set("with_genres", strings.Join(genreIDs, ","))
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
