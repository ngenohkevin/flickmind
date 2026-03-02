package ai

import (
	"fmt"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/store"
)

func buildSystemPrompt(maxResults int, mediaType string) string {
	typeInstruction := "movies or TV shows"
	if mediaType == "movie" {
		typeInstruction = "movies only (no TV series)"
	} else if mediaType == "series" {
		typeInstruction = "TV series only (no movies)"
	}

	return fmt.Sprintf(`You are FlickMind, an expert movie and TV recommendation engine.

TASK: Given the user's preferences, recommend exactly %d titles (%s).

CONTENT TYPES:
- movie: Feature films
- series: TV series
- anime: Japanese animation (movies or series)

RULES:
1. Only recommend real, existing titles
2. Prioritize content from 1990-present unless asked for classics
3. Diversify recommendations (different directors, studios, countries)
4. Match the MOOD and TONE, not just plot keywords
5. For anime requests, prefer highly-rated series
6. Consider both popular and lesser-known titles for variety
7. Never include unreleased or upcoming titles
8. Return exactly %d results

OUTPUT FORMAT (strict JSON array, no markdown, no explanation):
[
  {
    "title": "Exact Title",
    "year": 2020,
    "type": "movie" | "series" | "anime",
    "reason": "One compelling sentence explaining why this matches"
  }
]

IMPORTANT: Return ONLY the JSON array. No markdown code blocks, no additional text.`, maxResults, typeInstruction, maxResults)
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

func BuildAIPicksPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string) string {
	var parts []string
	parts = append(parts, buildSystemPrompt(cfg.MaxResults, mediaType))

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
	if len(cfg.ContentTypes) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(cfg.ContentTypes, ", ")))
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
	var parts []string
	parts = append(parts, buildSystemPrompt(cfg.MaxResults, mediaType))
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
	if len(cfg.ContentTypes) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(cfg.ContentTypes, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (exclude): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildWatchlistPicksPrompt(cfg *store.UserConfig, watchlistTitles []string, watchHistory []string, mediaType string) string {
	var parts []string
	parts = append(parts, buildSystemPrompt(cfg.MaxResults, mediaType))

	parts = append(parts, fmt.Sprintf("\nUSER'S WATCHLIST (titles they saved to watch later): %s", strings.Join(watchlistTitles, ", ")))
	parts = append(parts, "Analyze the patterns in their watchlist (genres, themes, directors, tone).")
	parts = append(parts, fmt.Sprintf("Recommend %d NEW titles they haven't added yet but would love based on their taste.", cfg.MaxResults))
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
	if len(cfg.ContentTypes) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(cfg.ContentTypes, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (also exclude these): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildBecauseYouWatchedPrompt(recentTitle string, cfg *store.UserConfig, mediaType string) string {
	var parts []string
	parts = append(parts, buildSystemPrompt(cfg.MaxResults, mediaType))
	parts = append(parts, fmt.Sprintf("\nThe user just watched: \"%s\"", recentTitle))
	parts = append(parts, fmt.Sprintf("Recommend %d titles that someone who loved this would also enjoy.", cfg.MaxResults))
	parts = append(parts, "Consider: similar themes, tone, director style, era, and genre.")

	if len(cfg.ContentTypes) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(cfg.ContentTypes, ", ")))
	}
	parts = appendYearRange(parts, cfg)

	return strings.Join(parts, "\n")
}
