package ai

import (
	"fmt"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/prefs"
	"github.com/ngenohkevin/flickmind/internal/store"
)

// PromptKind selects which catalog a prompt is written for.
type PromptKind int

const (
	KindPicks PromptKind = iota
	KindWatchlist
	KindGems
	KindBecauseYouWatched
	KindAnime
	KindDocumentary
)

// PromptInput carries per-request context. Zero values give the defaults.
type PromptInput struct {
	MediaType    string   // "movie", "series", or "" for both
	WatchHistory []string // "Title (Year)"; excluded from results
	Watchlist    []string // watchlist mode: the titles to learn taste from
	SeedTitle    string   // Because You Watched: the title to build from
	Count        int      // titles to request; 0 means OverRequest(cfg.MaxResults)
	Exclude      []string // titles already suggested earlier in this refresh
}

// maxRequest caps how many titles one prompt asks for, so a 50-result
// catalog stays inside the providers' output token limits.
const maxRequest = 60

// OverRequest returns how many titles to ask for so that enough survive TMDB
// matching and the settings filters to fill n results.
func OverRequest(n int) int {
	r := n + n/2
	if r < n+8 {
		r = n + 8
	}
	if r > maxRequest {
		r = maxRequest
	}
	return r
}

// RulesKind maps a prompt kind to the prefs rules it must honor.
func RulesKind(kind PromptKind) prefs.Kind {
	switch kind {
	case KindGems:
		return prefs.Gems
	case KindAnime:
		return prefs.Anime
	case KindDocumentary:
		return prefs.Documentary
	}
	return prefs.Picks
}

// BuildPrompt writes the full prompt for one catalog request.
func BuildPrompt(kind PromptKind, cfg *store.UserConfig, in PromptInput) string {
	count := in.Count
	if count <= 0 {
		count = OverRequest(cfg.MaxResults)
	}
	rules := prefs.For(cfg, RulesKind(kind))

	var parts []string
	switch kind {
	case KindAnime, KindDocumentary:
		parts = append(parts, focusedHeader(count, kind, len(rules.Languages) == 0))
	default:
		parts = append(parts, buildSystemPrompt(count, in.MediaType, len(rules.Languages) == 0))
	}

	switch kind {
	case KindGems:
		parts = append(parts, "\nSPECIAL FOCUS: Hidden Gems — underrated, lesser-known quality titles.")
		parts = append(parts, fmt.Sprintf("Aim for critically acclaimed titles with fewer than %s votes on TMDB — not mainstream.", formatThousands(prefs.GemsMaxVotes)))
		parts = append(parts, "Avoid: blockbusters, franchise films, widely-known titles.")
	case KindWatchlist:
		parts = append(parts, fmt.Sprintf("\nUSER'S WATCHLIST (titles they saved to watch later): %s", strings.Join(in.Watchlist, ", ")))
		parts = append(parts, "Analyze the patterns in their watchlist (genres, themes, directors, tone).")
		parts = append(parts, fmt.Sprintf("Recommend %d NEW titles they haven't added yet but would love based on their taste.", count))
		parts = append(parts, "Do NOT recommend titles already in their watchlist.")
	case KindBecauseYouWatched:
		parts = append(parts, fmt.Sprintf("\nThe user just watched: \"%s\"", in.SeedTitle))
		parts = append(parts, fmt.Sprintf("Recommend %d titles that someone who loved this would also enjoy.", count))
		parts = append(parts, "Consider: similar themes, tone, director style, era, and genre.")
	}

	parts = append(parts, hardRules(cfg, rules, kind, in.MediaType)...)
	if cfg.Mood != "" {
		parts = append(parts, fmt.Sprintf("MOOD: %s", cfg.Mood))
	}

	if len(in.WatchHistory) > 0 {
		parts = append(parts, fmt.Sprintf("\nRECENTLY WATCHED (exclude these): %s", strings.Join(in.WatchHistory, ", ")))
		if kind != KindWatchlist && kind != KindBecauseYouWatched {
			parts = append(parts, "Recommend titles similar to what they've watched but NEW to them.")
		}
	} else if kind == KindPicks || kind == KindAnime || kind == KindDocumentary {
		parts = append(parts, "\nRecommend a personalized mix based on the preferences above.")
	}
	if len(in.Exclude) > 0 {
		parts = append(parts, fmt.Sprintf("ALREADY SUGGESTED (do not repeat): %s", strings.Join(in.Exclude, ", ")))
	}
	// Strict rules made gpt-oss answer {"error":"impossible..."} or refuse
	// outright; the server filters anyway, so ask for best effort.
	parts = append(parts, "\nIf fewer titles fit, return as many as you can. Always return the JSON list — never an error, refusal or apology.")

	return strings.Join(parts, "\n")
}

func buildSystemPrompt(count int, mediaType string, anyLanguage bool) string {
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

	return fmt.Sprintf(`Recommend exactly %d titles (%s). Only real, released titles that exist on IMDb — never invent a title. %s

Return ONLY valid JSON, no markdown:
{"recommendations":[{"title":"Exact Title","year":2020,"type":"%s","reason":"Why this matches"}]}
Use each title's original release year (first air year for series).`, count, typeInstruction, diversify(anyLanguage), exampleType)
}

func focusedHeader(count int, kind PromptKind, anyLanguage bool) string {
	focusInstruction := "Recommend anime and animated movies AND TV series (both types mixed). ONLY anime/animated titles — no live-action."
	if kind == KindDocumentary {
		focusInstruction = "Recommend documentary movies AND TV series (both types mixed). ONLY documentaries — no fiction."
	}
	return fmt.Sprintf(`Recommend exactly %d titles (movies and TV series, both types).
%s Only real, released titles that exist on IMDb — never invent a title. %s

Return ONLY valid JSON, no markdown:
{"recommendations":[{"title":"Exact Title","year":2020,"type":"movie","reason":"Why this matches"}]}
Use "type":"movie" for films and "type":"series" for TV shows. Use each title's original release year.`, count, focusInstruction, diversify(anyLanguage))
}

func diversify(anyLanguage bool) string {
	if anyLanguage {
		return "Diversify by director/country/tone."
	}
	return "Diversify by director/era/tone."
}

// hardRules states the user's settings as requirements. The server enforces
// the same rules against TMDB data, so anything that breaks them is dropped.
func hardRules(cfg *store.UserConfig, rules prefs.Rules, kind PromptKind, mediaType string) []string {
	lines := []string{"\nRULES — titles that break these are filtered out:"}

	if len(rules.Selected) > 0 {
		lines = append(lines, fmt.Sprintf("PREFERRED GENRES (each title must fit at least one): %s", displayList(rules.Selected)))
	}
	if len(rules.Excluded) > 0 {
		lines = append(lines, fmt.Sprintf("EXCLUDED GENRES (never recommend titles in these): %s", displayList(rules.Excluded)))
	}
	if len(rules.Languages) > 0 {
		lines = append(lines, fmt.Sprintf("LANGUAGE: only titles originally in %s (%s)", prefs.LanguageName(cfg.Language), cfg.Language))
	}
	if rules.MinRating > 0 {
		lines = append(lines, fmt.Sprintf("MINIMUM RATING: %.1f/10", rules.MinRating))
	}
	lines = appendYearRange(lines, rules.YearFrom, rules.YearTo)

	if kind != KindAnime && kind != KindDocumentary {
		if filtered := contentTypesForMediaType(cfg.ContentTypes, mediaType); len(filtered) > 0 {
			lines = append(lines, fmt.Sprintf("CONTENT TYPES: %s only", strings.Join(filtered, ", ")))
		}
	}

	if len(lines) == 1 {
		return nil
	}
	return lines
}

// contentTypesForMediaType filters content types to match the requested Stremio media type.
// Anime/documentary are excluded — they have their own dedicated catalogs now.
func contentTypesForMediaType(contentTypes []string, mediaType string) []string {
	var filtered []string
	for _, ct := range contentTypes {
		switch ct {
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

func appendYearRange(parts []string, from, to int) []string {
	const note = " (by original release year — titles outside this range are rejected)"
	if from > 0 && to > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: %d-%d%s", from, to, note))
	} else if from > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: %d-present%s", from, note))
	} else if to > 0 {
		parts = append(parts, fmt.Sprintf("YEAR RANGE: up to %d%s", to, note))
	}
	return parts
}

func displayList(genres []string) string {
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = prefs.DisplayGenre(g)
	}
	return strings.Join(names, ", ")
}

func formatThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	return s[:len(s)-3] + "," + s[len(s)-3:]
}

// The builders below keep the original call shapes.

func BuildAIPicksPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string) string {
	return BuildPrompt(KindPicks, cfg, PromptInput{MediaType: mediaType, WatchHistory: watchHistory})
}

func BuildHiddenGemsPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string) string {
	return BuildPrompt(KindGems, cfg, PromptInput{MediaType: mediaType, WatchHistory: watchHistory})
}

func BuildWatchlistPicksPrompt(cfg *store.UserConfig, watchlistTitles []string, watchHistory []string, mediaType string) string {
	return BuildPrompt(KindWatchlist, cfg, PromptInput{MediaType: mediaType, Watchlist: watchlistTitles, WatchHistory: watchHistory})
}

func BuildFocusedPrompt(cfg *store.UserConfig, watchHistory []string, mediaType string, focusType string) string {
	kind := KindAnime
	if focusType == "documentary" {
		kind = KindDocumentary
	}
	return BuildPrompt(kind, cfg, PromptInput{MediaType: mediaType, WatchHistory: watchHistory})
}

func BuildBecauseYouWatchedPrompt(recentTitle string, cfg *store.UserConfig, mediaType string) string {
	return BuildPrompt(KindBecauseYouWatched, cfg, PromptInput{MediaType: mediaType, SeedTitle: recentTitle})
}
