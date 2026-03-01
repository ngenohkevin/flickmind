package ai

import (
	"fmt"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/store"
)

const systemPrompt = `You are FlickMind, an expert movie and TV recommendation engine.

TASK: Given the user's preferences, recommend up to 15 titles (movies or TV shows).

CONTENT TYPES:
- movie: Feature films
- series: TV series
- anime: Japanese animation (movies or series)

RULES:
1. Only recommend real, existing titles
2. Mix content types unless user specifies otherwise
3. Prioritize content from 1990-present unless asked for classics
4. Diversify recommendations (different directors, studios, countries)
5. Match the MOOD and TONE, not just plot keywords
6. For anime requests, prefer highly-rated series
7. Consider both popular and lesser-known titles for variety
8. Never include unreleased or upcoming titles

OUTPUT FORMAT (strict JSON array, no markdown, no explanation):
[
  {
    "title": "Exact Title",
    "year": 2020,
    "type": "movie" | "series" | "anime",
    "reason": "One compelling sentence explaining why this matches"
  }
]

IMPORTANT: Return ONLY the JSON array. No markdown code blocks, no additional text.`

func BuildAIPicksPrompt(cfg *store.UserConfig, watchHistory []string) string {
	var parts []string
	parts = append(parts, systemPrompt)

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

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nRECENTLY WATCHED (exclude these): %s", strings.Join(watchHistory, ", ")))
		parts = append(parts, "Recommend titles similar to what they've watched but NEW to them.")
	} else {
		parts = append(parts, "\nRecommend a personalized mix based on the preferences above.")
	}

	return strings.Join(parts, "\n")
}

func BuildHiddenGemsPrompt(cfg *store.UserConfig, watchHistory []string) string {
	var parts []string
	parts = append(parts, systemPrompt)
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

	if len(watchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nALREADY WATCHED (exclude): %s", strings.Join(watchHistory, ", ")))
	}

	return strings.Join(parts, "\n")
}

func BuildBecauseYouWatchedPrompt(recentTitle string, cfg *store.UserConfig) string {
	var parts []string
	parts = append(parts, systemPrompt)
	parts = append(parts, fmt.Sprintf("\nThe user just watched: \"%s\"", recentTitle))
	parts = append(parts, "Recommend 15 titles that someone who loved this would also enjoy.")
	parts = append(parts, "Consider: similar themes, tone, director style, era, and genre.")

	if len(cfg.ContentTypes) > 0 {
		parts = append(parts, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(cfg.ContentTypes, ", ")))
	}

	return strings.Join(parts, "\n")
}
