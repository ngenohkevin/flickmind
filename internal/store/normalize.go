package store

import (
	"math"
	"strings"
	"time"
)

var canonicalGenres = map[string]string{
	"action": "Action", "adventure": "Adventure", "animation": "Animation",
	"comedy": "Comedy", "crime": "Crime", "documentary": "Documentary",
	"drama": "Drama", "fantasy": "Fantasy", "horror": "Horror",
	"mystery": "Mystery", "romance": "Romance", "sci-fi": "Sci-Fi",
	"science fiction": "Sci-Fi", "scifi": "Sci-Fi", "thriller": "Thriller",
	"war": "War",
}

var validContentTypes = map[string]bool{"movie": true, "series": true, "anime": true, "documentary": true}

var validLanguages = map[string]bool{
	"en": true, "es": true, "fr": true, "de": true, "ja": true,
	"ko": true, "zh": true, "hi": true, "it": true, "pt": true,
}

var validMoods = map[string]bool{
	"cozy": true, "thrilling": true, "mind-bending": true, "feel-good": true, "dark": true,
	"romantic": true, "funny": true, "epic": true, "nostalgic": true,
}

var validRefreshIntervals = map[string]bool{"1h": true, "2h": true, "6h": true, "12h": true, "24h": true}

const (
	MinResults     = 10
	MaxResultsCap  = 50
	DefaultResults = 25
	minYear        = 1900
)

// NormalizeConfig coerces a config into values the recommendation pipeline
// can rely on: known genres/types/languages/moods only, ratings and years in
// range, results within bounds, and API keys without stray whitespace.
func NormalizeConfig(cfg *UserConfig) {
	cfg.GroqKey = stripSpace(cfg.GroqKey)
	cfg.DeepSeekKey = stripSpace(cfg.DeepSeekKey)
	cfg.GeminiKey = stripSpace(cfg.GeminiKey)

	cfg.Genres = normalizeGenres(cfg.Genres)
	cfg.ContentTypes = normalizeContentTypes(cfg.ContentTypes)

	cfg.Language = strings.ToLower(strings.TrimSpace(cfg.Language))
	if cfg.Language == "any" || !validLanguages[cfg.Language] {
		cfg.Language = ""
	}

	cfg.Mood = strings.ToLower(strings.TrimSpace(cfg.Mood))
	if !validMoods[cfg.Mood] {
		cfg.Mood = ""
	}

	if math.IsNaN(cfg.MinRating) || cfg.MinRating < 0 {
		cfg.MinRating = 0
	}
	if cfg.MinRating > 10 {
		cfg.MinRating = 10
	}

	maxYear := time.Now().Year() + 1
	cfg.YearFrom = clampYear(cfg.YearFrom, maxYear)
	cfg.YearTo = clampYear(cfg.YearTo, maxYear)
	if cfg.YearFrom > 0 && cfg.YearTo > 0 && cfg.YearFrom > cfg.YearTo {
		cfg.YearFrom, cfg.YearTo = cfg.YearTo, cfg.YearFrom
	}

	switch {
	case cfg.MaxResults <= 0:
		cfg.MaxResults = DefaultResults
	case cfg.MaxResults < MinResults:
		cfg.MaxResults = MinResults
	case cfg.MaxResults > MaxResultsCap:
		cfg.MaxResults = MaxResultsCap
	}

	if cfg.RecommendationSource != "watchlist" {
		cfg.RecommendationSource = "preferences"
	}
	if !validRefreshIntervals[cfg.RefreshInterval] {
		cfg.RefreshInterval = "2h"
	}
}

func stripSpace(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func normalizeGenres(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range in {
		name, ok := canonicalGenres[strings.ToLower(strings.TrimSpace(g))]
		if ok && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func normalizeContentTypes(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, ct := range in {
		ct = strings.ToLower(strings.TrimSpace(ct))
		if validContentTypes[ct] && !seen[ct] {
			seen[ct] = true
			out = append(out, ct)
		}
	}
	if len(out) == 0 {
		return []string{"movie", "series"}
	}
	return out
}

func clampYear(y, maxYear int) int {
	switch {
	case y <= 0:
		return 0
	case y < minYear:
		return minYear
	case y > maxYear:
		return maxYear
	}
	return y
}
