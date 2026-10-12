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
	"time"
	"unicode"

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

	// Fetch IMDB ID: use movie detail endpoint for movies (faster), external_ids for TV
	if result.MediaType == "movie" {
		imdbID, err := c.GetMovieIMDBId(ctx, result.ID)
		if err == nil && imdbID != "" {
			result.IMDBId = imdbID
		}
	} else {
		imdbID, err := c.GetExternalIDs(ctx, result.ID, result.MediaType)
		if err == nil && imdbID != "" {
			result.IMDBId = imdbID
		}
	}

	return result
}

// candidate is a search hit reduced to what title matching needs.
type candidate struct {
	title, original string
	year, votes     int
}

// searchMovieWithRetry searches with the AI's year first, then without it
// (AI years are often off by one), and only accepts a hit whose title matches.
func (c *Client) searchMovieWithRetry(ctx context.Context, title string, year int) *SearchResult {
	pick := func(movies []tmdbMovieResult, tolerance int) *SearchResult {
		cands := make([]candidate, len(movies))
		for i, m := range movies {
			cands[i] = candidate{m.Title, m.OriginalTitle, extractYear(m.ReleaseDate), m.VoteCount}
		}
		if i := bestMatch(cands, title, year, tolerance); i >= 0 {
			return movieToResult(&movies[i])
		}
		return nil
	}

	if year > 0 {
		if movies, err := c.SearchMovies(ctx, title, year); err == nil {
			if r := pick(movies, 0); r != nil {
				return r
			}
		}
	}
	if movies, err := c.SearchMovies(ctx, title, 0); err == nil {
		return pick(movies, 1)
	}
	return nil
}

func (c *Client) searchTVWithRetry(ctx context.Context, title string, year int) *SearchResult {
	pick := func(shows []tmdbTVResult, tolerance int) *SearchResult {
		cands := make([]candidate, len(shows))
		for i, sh := range shows {
			cands[i] = candidate{sh.Name, sh.OriginalName, extractYear(sh.FirstAirDate), sh.VoteCount}
		}
		if i := bestMatch(cands, title, year, tolerance); i >= 0 {
			return tvToResult(&shows[i])
		}
		return nil
	}

	if year > 0 {
		if shows, err := c.SearchTV(ctx, title, year); err == nil {
			if r := pick(shows, 0); r != nil {
				return r
			}
		}
	}
	if shows, err := c.SearchTV(ctx, title, 0); err == nil {
		return pick(shows, 1)
	}
	return nil
}

// bestMatch returns the index of the candidate that is really the title the AI
// meant, or -1. Search results are ranked by popularity, not by title, so
// taking the first hit returned "Captain Underpants" for "The Captain". A
// candidate must match the title (or its original-language title); among
// matches the closest year wins, then the most-voted (two 2013 films are
// called "Under the Skin"). tolerance is the allowed year difference when the
// AI gave a year.
func bestMatch(cands []candidate, title string, year, tolerance int) int {
	want := normTitle(title)
	wantBase := normTitle(beforeColon(title))
	if want == "" {
		return -1
	}

	best, bestScore := -1, -1
	for i, c := range cands {
		yearDiff := 0
		if year > 0 {
			yearDiff = abs(c.year - year)
			if yearDiff > tolerance {
				continue
			}
		}

		titleScore := 0
		switch {
		case normTitle(c.title) == want || normTitle(c.original) == want:
			titleScore = 2
		case yearDiff == 0 && (normTitle(beforeColon(c.title)) == want || normTitle(c.title) == wantBase):
			// "Borat" vs "Borat: Cultural Learnings of America...", same year
			titleScore = 1
		default:
			continue
		}

		// Title first, then year closeness, then votes as the tiebreaker.
		score := titleScore*1_000_000_000 + (10-yearDiff)*10_000_000 + min(c.votes, 9_999_999)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

// normTitle lowercases a title and keeps only letters and digits, so
// punctuation, accents in separators, and "&" vs "and" don't block a match.
func normTitle(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", "and")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func beforeColon(s string) string {
	if i := strings.Index(s, ":"); i > 0 {
		return s[:i]
	}
	return s
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (c *Client) EnrichRecommendations(ctx context.Context, recs []ai.Recommendation) []SearchResult {
	type indexedResult struct {
		index  int
		result *SearchResult
	}

	var mu sync.Mutex
	var results []indexedResult
	var wg sync.WaitGroup

	sem := make(chan struct{}, 16) // limit concurrency (TMDB allows 40 req/10s)

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

// AttachIMDBIds looks up IMDB IDs for discover results (which don't carry
// them), so stream addons can resolve the titles.
func (c *Client) AttachIMDBIds(ctx context.Context, results []SearchResult) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i := range results {
		if results[i].IMDBId != "" {
			continue
		}
		wg.Add(1)
		go func(r *SearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var id string
			var err error
			if r.MediaType == "movie" {
				id, err = c.GetMovieIMDBId(ctx, r.ID)
			} else {
				id, err = c.GetExternalIDs(ctx, r.ID, r.MediaType)
			}
			if err == nil {
				r.IMDBId = id
			}
		}(&results[i])
	}
	wg.Wait()
}

// NormTitle normalizes a title for comparison (lowercase letters and digits only).
func NormTitle(s string) string { return normTitle(s) }

// DiscoverOpts narrows the TMDB discover fallback to the user's settings.
type DiscoverOpts struct {
	MediaType string // "movie", "series", or "" for both
	Genres    []string
	MinRating float64
	YearFrom  int
	YearTo    int
	Language  string // TMDB original language code(s), pipe-separated; "" for any
	MaxVotes  int    // drop titles with more votes than this (hidden gems); 0 for no cap
	MinVotes  int    // minimum votes; defaults to 100
	SortBy    string // TMDB sort order; defaults to popularity.desc
	// ReleasedBefore, when set, also caps the release date (tighter than YearTo).
	ReleasedBefore time.Time
	SkipTVMovies   bool // exclude TMDB's "TV Movie" genre (specials, re-cuts)
}

// DiscoverFallback returns popular content from TMDB discover when no AI is available
func (c *Client) DiscoverFallback(ctx context.Context, o DiscoverOpts) []SearchResult {
	var results []SearchResult

	// TMDB returns 20 results per page; the settings filter drops many, so
	// fetch up to 5 pages to fill larger requests
	maxPages := 5

	if o.MediaType == "movie" || o.MediaType == "" {
		params := discoverParams(o, "movie")
		for page := 1; page <= maxPages; page++ {
			params.Set("page", strconv.Itoa(page))
			movies, err := c.DiscoverMovies(ctx, params)
			if err != nil || len(movies) == 0 {
				break
			}
			for i := range movies {
				results = append(results, *movieToResult(&movies[i]))
			}
		}
	}

	if o.MediaType == "series" || o.MediaType == "" {
		params := discoverParams(o, "series")
		for page := 1; page <= maxPages; page++ {
			params.Set("page", strconv.Itoa(page))
			shows, err := c.DiscoverTV(ctx, params)
			if err != nil || len(shows) == 0 {
				break
			}
			for i := range shows {
				results = append(results, *tvToResult(&shows[i]))
			}
		}
	}

	return results
}

// discoverParams builds discover filters for one media type; movie and TV use
// different genre IDs and date fields.
func discoverParams(o DiscoverOpts, mediaType string) url.Values {
	sortBy := o.SortBy
	if sortBy == "" {
		sortBy = "popularity.desc"
	}
	minVotes := o.MinVotes
	if minVotes <= 0 {
		minVotes = 100
	}
	params := url.Values{
		"sort_by":        {sortBy},
		"vote_count.gte": {strconv.Itoa(minVotes)},
		"include_adult":  {"false"},
		"language":       {"en-US"},
	}

	if o.MaxVotes > 0 {
		params.Set("vote_count.lte", strconv.Itoa(o.MaxVotes))
	}
	if o.MinRating > 0 {
		params.Set("vote_average.gte", fmt.Sprintf("%.1f", o.MinRating))
	}
	if genreIDs := mapGenreNamesToIDs(o.Genres, mediaType); len(genreIDs) > 0 {
		params.Set("with_genres", strings.Join(genreIDs, "|"))
	}
	if o.Language != "" {
		params.Set("with_original_language", o.Language)
	}
	if o.SkipTVMovies && mediaType == "movie" {
		params.Set("without_genres", "10770")
	}

	dateField := "primary_release_date"
	if mediaType == "series" {
		dateField = "first_air_date"
	}
	if o.YearFrom > 0 {
		params.Set(dateField+".gte", fmt.Sprintf("%d-01-01", o.YearFrom))
	}
	if o.YearTo > 0 {
		params.Set(dateField+".lte", fmt.Sprintf("%d-12-31", o.YearTo))
	}
	if !o.ReleasedBefore.IsZero() {
		before := o.ReleasedBefore.Format("2006-01-02")
		if cur := params.Get(dateField + ".lte"); cur == "" || before < cur {
			params.Set(dateField+".lte", before)
		}
	}
	return params
}

func movieToResult(m *tmdbMovieResult) *SearchResult {
	return &SearchResult{
		ID:               m.ID,
		Title:            m.Title,
		OriginalLanguage: m.OriginalLanguage,
		MediaType:        "movie",
		PosterPath:       m.PosterPath,
		BackdropPath:     m.BackdropPath,
		Overview:         m.Overview,
		VoteAverage:      m.VoteAverage,
		VoteCount:        m.VoteCount,
		Year:             extractYear(m.ReleaseDate),
		GenreIDs:         m.GenreIDs,
		Popularity:       m.Popularity,
	}
}

func tvToResult(t *tmdbTVResult) *SearchResult {
	return &SearchResult{
		ID:               t.ID,
		Title:            t.Name,
		OriginalLanguage: t.OriginalLanguage,
		MediaType:        "tv",
		PosterPath:       t.PosterPath,
		BackdropPath:     t.BackdropPath,
		Overview:         t.Overview,
		VoteAverage:      t.VoteAverage,
		VoteCount:        t.VoteCount,
		Year:             extractYear(t.FirstAirDate),
		GenreIDs:         t.GenreIDs,
		Popularity:       t.Popularity,
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

// movieConfigGenres and tvConfigGenres map TMDB genre IDs to the lowercase genre names used in
// user config. TV merges some genres ("Sci-Fi & Fantasy"), so an ID can map
// to more than one name.
var movieConfigGenres = map[int][]string{
	28: {"action"}, 12: {"adventure"}, 16: {"animation"}, 35: {"comedy"},
	80: {"crime"}, 99: {"documentary"}, 18: {"drama"}, 14: {"fantasy"},
	27: {"horror"}, 9648: {"mystery"}, 10749: {"romance"}, 878: {"sci-fi"},
	53: {"thriller"}, 10752: {"war"}, 10751: {"family"}, 10402: {"music"},
	36: {"history"}, 37: {"western"},
}

var tvConfigGenres = map[int][]string{
	10759: {"action", "adventure"}, 16: {"animation"}, 35: {"comedy"},
	80: {"crime"}, 99: {"documentary"}, 18: {"drama"}, 10765: {"sci-fi", "fantasy"},
	9648: {"mystery"}, 10768: {"war"}, 10751: {"family"}, 10762: {"kids"},
	10763: {"news"}, 10764: {"reality"}, 10766: {"soap"}, 10767: {"talk"},
	37: {"western"},
}

// ConfigGenres converts TMDB genre IDs to lowercase config genre names.
func ConfigGenres(ids []int, mediaType string) []string {
	m := movieConfigGenres
	if mediaType == "tv" {
		m = tvConfigGenres
	}
	var names []string
	for _, id := range ids {
		names = append(names, m[id]...)
	}
	return names
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
