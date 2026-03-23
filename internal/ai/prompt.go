package ai

import (
	"fmt"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/store"
)

// contentTypesForMediaType filters content types to match the requested Stremio media type.
// Anime/documentary are excluded — they have their own dedicated catalogs now.
func contentTypesForMediaType(contentTypes []string, mediaType string) []string {
	var filtered []string
	for _, ct := range contentTypes {
		switch ct {
		case "anime", "documentary":
			// Skip — these have dedicated catalog rows now
			continue
		case "movie":
			if mediaType == "movie" || mediaType == "" {
				filtered = append(filtered, ct)
			}
		case "series":
			if mediaType == "series" || mediaType == "" {
				filtered = append(filtered, ct)
			}
		}
	}
	return filtered
}

func buildSystemPrompt(maxResults int, mediaType string) string {
	// Always strict type based on mediaType
	typeInstruction := "movies or TV shows"
	if mediaType == "movie" {
		typeInstruction = "movies only (no TV series)"
	} else if mediaType == "series" {
		typeInstruction = "TV series only (no movies)"
	}

	// Use matching example type so the model doesn't get confused
	exampleType := "movie"
	if mediaType == "series" {
		exampleType = "series"
	}

	return fmt.Sprintf(`Recommend exactly %d titles (%s). Only real, released titles. Diversify by director/country/tone.

Return ONLY valid JSON, no markdown:
{"recommendations":[{"title":"Exact Title","year":2020,"type":"%s","reason":"Why this matches"}]}`, maxResults, typeInstruction, exampleType)
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
	parts = append(parts, buildSystemPrompt(requestCount, mediaType))

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
	parts = append(parts, buildSystemPrompt(requestCount, mediaType))
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
	parts = append(parts, buildSystemPrompt(requestCount, mediaType))

	parts = append(parts, fmt.Sprintf("\nUSER'S WATCHLIST (titles they saved to watch later): %s", strings.Join(watchlistTitles, ", ")))
	parts = append(parts, "Analyze the patterns in their watchlist (genres, themes, directors, tone).")
	parts = append(parts, fmt.Sprintf("Recommend %d NEW titles they haven't added yet but would love based on their taste.", requestCount))
	parts = append(parts, "Do NOT recommend titles already in their watchlist.")

	if len(cfg.Genres) > 0 {
		parts = append(parts, fmt.Sprintf("\nPREFERRED GENRES (stick to these): %s", strings.Join(cfg.Genres, ", ")))
		parts = append(parts, "IMPORTANT: Only recommend titles that fit within the preferred genres above. Do NOT recommend genres outside this list (e.g., if Horror is not listed, do not recommend horror titles).")
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
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (also exclude these): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildFocusedPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string, focusType string) string {
	requestCount := overRequest(cfg.MaxResults)

	var focusInstruction string
	switch focusType {
	case "anime":
		focusInstruction = "Recommend anime and animated movies AND TV series (both types mixed). ONLY anime/animated titles — no live-action."
	case "documentary":
		focusInstruction = "Recommend documentary movies AND TV series (both types mixed). ONLY documentaries — no fiction."
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(`Recommend exactly %d titles (movies and TV series, both types).
%s Only real, released titles. Diversify by director/country/tone.

Return ONLY valid JSON, no markdown:
{"recommendations":[{"title":"Exact Title","year":2020,"type":"movie","reason":"Why this matches"}]}
Use "type":"movie" for films and "type":"series" for TV shows.`, requestCount, focusInstruction))

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
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nRECENTLY WATCHED (exclude these): %s", strings.Join(watchHistory, ", ")))
		parts = append(parts, "Recommend titles similar to what they've watched but NEW to them.")
	} else {
		parts = append(parts, "\nRecommend a personalized mix based on the preferences above.")
	}

	return strings.Join(parts, "\n")
}

func BuildBecauseYouWatchedPrompt(recentTitle string, cfg *store.UserConfig, mediaType string) string {
	requestCount := overRequest(cfg.MaxResults)
	filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType)
	var parts []string
	parts = append(parts, buildSystemPrompt(requestCount, mediaType))
	parts = append(parts, fmt.Sprintf("\nThe user just watched: \"%s\"", recentTitle))
	parts = append(parts, fmt.Sprintf("Recommend %d titles that someone who loved this would also enjoy.", requestCount))
	parts = append(parts, "Consider: similar themes, tone, director style, era, and genre.")

	if len(filtered) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	return strings.Join(parts, "\n")
}
