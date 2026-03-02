package ai

import (
	"fmt"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/store"
)

func hasFlexibleContentType(contentTypes []string) bool {
	for _, ct := range contentTypes {
		if ct == "anime" || ct == "documentary" {
			return true
		}
	}
	return false
}

// contentTypesForMediaType filters content types to match the requested Stremio media type.
// When flexible types (anime/documentary) are present, all types pass through.
func contentTypesForMediaType(contentTypes []string, mediaType string) []string {
	if hasFlexibleContentType(contentTypes) {
		return contentTypes
	}
	// Filter to only the relevant type to avoid contradicting the type instruction
	switch mediaType {
	case "movie":
		for _, ct := range contentTypes {
			if ct == "movie" {
				return []string{"movie"}
			}
		}
	case "series":
		for _, ct := range contentTypes {
			if ct == "series" {
				return []string{"series"}
			}
		}
	}
	return contentTypes
}

func buildSystemPrompt(maxResults int, mediaType string, contentTypes []string) string {
	typeInstruction := "movies or TV shows"
	if hasFlexibleContentType(contentTypes) {
		typeInstruction = "movies and TV series (both types welcome, especially anime series and documentary series)"
	} else if mediaType == "movie" {
		typeInstruction = "movies only (no TV series)"
	} else if mediaType == "series" {
		typeInstruction = "TV series only (no movies)"
	}

	return fmt.Sprintf(`Recommend exactly %d titles (%s). Only real, released titles. Diversify by director/country/tone.

Types: movie, series, anime (Japanese animation, movie or series).

Return ONLY a JSON array, no markdown:
[{"title":"Exact Title","year":2020,"type":"movie","reason":"Why this matches"}]`, maxResults, typeInstruction)
}

func appendYearRange(parts []string, cfg *store.UserConfig) []string {
	if cfg.YearFrom > 0 && cfg.YearTo > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: %d-%d", cfg.YearFrom, cfg.YearTo))
	} else if cfg.YearFrom > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: %d-present", cfg.YearFrom))
	} else if cfg.YearTo > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: up to %d", cfg.YearTo))
	}
	return parts
}

// overRequest returns a count ~30% higher to compensate for TMDB enrichment loss.
func overRequest(n int) int {
	r := n + n*3/10
	if r < n+5 {
		r = n + 5
	}
	return r
}

func BuildAIPicksPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string) string {
	requestCount := overRequest(cfg.MaxResults)
	filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType)
	var parts []string
	parts = append(parts, buildSystemPrompt(requestCount, mediaType, cfg.ContentTypes))

	if len(cfg.Genres) > 0 {
		parts = append(parts, fmt.Sprintf("\nPREFERRED GENRES: %s", strings.Join(cfg.Genres, ", ")))
	}
	if cfg.Mood != "" {
		parts = append(parts, fmt.Sprintf("MOOD: %s", cfg.Mood))
	}
	if cfg.Language != "" && cfg.Language != "en" {
		parts = append(parts, fmt.Sprintf("PREFERRED LANGUAGE: %s", cfg.Language))
	}
	if cfg.MinRating > 0 {
		parts = append(parts, fmt.Sprintf("MINIMUM RATING: %.1f/10", cfg.MinRating))
	}
	if len(filtered) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nRECENTLY WATCHED (exclude these): %s", strings.Join(watchHistory, ", ")))
		parts = append(parts, "Recommend titles similar to what they've watched but NEW to them.")
	} else {
		parts = append(parts, "\nRecommend a personalized mix based on the preferences above.")
	}

	return strings.Join(parts, "\n")
}

func BuildHiddenGemsPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string) string {
	requestCount := overRequest(cfg.MaxResults)
	filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType)
	var parts []string
	parts = append(parts, buildSystemPrompt(requestCount, mediaType, cfg.ContentTypes))
	parts = append(parts, "\nSPECIAL FOCUS: Hidden Gems — underrated, lesser-known quality titles.")
	parts = append(parts, "Prioritize: vote count < 5000, rating >= 7.0, critically acclaimed but not mainstream.")
	parts = append(parts, "Avoid: blockbusters, franchise films, widely-known titles.")

	if len(cfg.Genres) > 0 {
		parts = append(parts, fmt.Sprintf("\nPREFERRED GENRES: %s", strings.Join(cfg.Genres, ", ")))
	}
	if cfg.Mood != "" {
		parts = append(parts, fmt.Sprintf("MOOD: %s", cfg.Mood))
	}
	if cfg.Language != "" && cfg.Language != "en" {
		parts = append(parts, fmt.Sprintf("PREFERRED LANGUAGE: %s", cfg.Language))
	}
	if len(filtered) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (exclude): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildWatchlistPicksPrompt(cfg *store.UserConfig, watchlistTitles []string, watchHistory []string, mediaType string) string {
	requestCount := overRequest(cfg.MaxResults)
	filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType)
	var parts []string
	parts = append(parts, buildSystemPrompt(requestCount, mediaType, cfg.ContentTypes))

	parts = append(parts, fmt.Sprintf("\nUSER'S WATCHLIST (titles they saved to watch later): %s", strings.Join(watchlistTitles, ", ")))
	parts = append(parts, "Analyze the patterns in their watchlist (genres, themes, directors, tone).")
	parts = append(parts, fmt.Sprintf("Recommend %d NEW titles they haven't added yet but would love based on their taste.", requestCount))
	parts = append(parts, "Do NOT recommend titles already in their watchlist.")

	if cfg.Mood != "" {
		parts = append(parts, fmt.Sprintf("MOOD: %s", cfg.Mood))
	}
	if cfg.Language != "" && cfg.Language != "en" {
		parts = append(parts, fmt.Sprintf("PREFERRED LANGUAGE: %s", cfg.Language))
	}
	if cfg.MinRating > 0 {
		parts = append(parts, fmt.Sprintf("MINIMUM RATING: %.1f/10", cfg.MinRating))
	}
	if len(filtered) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (also exclude these): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildBecauseYouWatchedPrompt(recentTitle string, cfg *store.UserConfig, mediaType string) string {
	requestCount := overRequest(cfg.MaxResults)
	filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType)
	var parts []string
	parts = append(parts, buildSystemPrompt(requestCount, mediaType, cfg.ContentTypes))
	parts = append(parts, fmt.Sprintf("\nThe user just watched: \"%s\"", recentTitle))
	parts = append(parts, fmt.Sprintf("Recommend %d titles that someone who loved this would also enjoy.", requestCount))
	parts = append(parts, "Consider: similar themes, tone, director style, era, and genre.")

	if len(filtered) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	return strings.Join(parts, "\n")
}
