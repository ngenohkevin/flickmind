package ai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ngenohkevin/flickmind/internal/store"
)

func cfgWithDefaults(overrides *store.UserConfig) *store.UserConfig {
	if overrides.MaxResults <= 0 {
		overrides.MaxResults = 25
	}
	return overrides
}

func TestBuildAIPicksPrompt_IncludesPreferences(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{
		Genres:       []string{"action", "sci-fi"},
		Mood:         "exciting",
		Language:     "fr",
		ContentTypes: []string{"movie"},
		MinRating:    7.5,
	})

	prompt := BuildAIPicksPrompt(cfg, nil, "movie")
	lower := strings.ToLower(prompt)

	if !strings.Contains(lower, "action") {
		t.Error("prompt should include genres")
	}
	if !strings.Contains(prompt, "exciting") {
		t.Error("prompt should include mood")
	}
	if !strings.Contains(lower, "french") {
		t.Error("prompt should include language")
	}
	if !strings.Contains(prompt, "movie") {
		t.Error("prompt should include content types")
	}
	if !strings.Contains(prompt, "7.5") {
		t.Error("prompt should include min rating")
	}
}

func TestBuildAIPicksPrompt_WithWatchHistory(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	history := []string{"Inception (2010)", "Breaking Bad (2008)"}

	prompt := BuildAIPicksPrompt(cfg, history, "movie")

	if !strings.Contains(prompt, "Inception (2010)") {
		t.Error("prompt should include watch history")
	}
	if !strings.Contains(prompt, "RECENTLY WATCHED") {
		t.Error("prompt should reference recently watched")
	}
}

func TestBuildAIPicksPrompt_WithoutWatchHistory(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})

	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if strings.Contains(prompt, "RECENTLY WATCHED") {
		t.Error("prompt should not reference recently watched when none provided")
	}
	if !strings.Contains(prompt, "personalized mix") {
		t.Error("prompt should include default personalized mix message")
	}
}

func TestBuildAIPicksPrompt_EnglishIsEnforced(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "only titles originally in English") {
		t.Errorf("English should be a hard language rule, got: %s", prompt)
	}
}

func TestBuildAIPicksPrompt_AnyLanguageHasNoRule(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: ""})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if strings.Contains(prompt, "LANGUAGE:") {
		t.Error("Any language should not add a language rule")
	}
	if !strings.Contains(prompt, "country") {
		t.Error("Any language should still diversify by country")
	}
}

func TestBuildAIPicksPrompt_ExcludesUnselectedGenres(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", Genres: []string{"Action", "Thriller"}})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "EXCLUDED GENRES") || !strings.Contains(prompt, "Horror") {
		t.Errorf("unselected Horror should be excluded, got: %s", prompt)
	}
	if strings.Contains(prompt, "Drama,") {
		t.Error("Drama is neutral and should not be excluded")
	}
}

func TestBuildBecauseYouWatchedPrompt_HonorsSettings(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", Genres: []string{"Crime"}, MinRating: 7, YearFrom: 2010})
	prompt := BuildBecauseYouWatchedPrompt("Sicario (2015)", cfg, "movie")

	for _, want := range []string{"PREFERRED GENRES", "Crime", "MINIMUM RATING: 7.0", "YEAR RANGE: 2010-present", "English"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("because-you-watched prompt missing %q", want)
		}
	}
}

func TestBuildPrompt_CountAndExclude(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildPrompt(KindPicks, cfg, PromptInput{MediaType: "movie", Count: 12, Exclude: []string{"Heat (1995)"}})

	if !strings.Contains(prompt, "Recommend exactly 12 titles") {
		t.Error("explicit count should be used")
	}
	if !strings.Contains(prompt, "ALREADY SUGGESTED (do not repeat): Heat (1995)") {
		t.Error("exclusions should be listed")
	}
}

func TestBuildPrompt_AsksForBestEffort(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", Genres: []string{"Thriller"}})
	prompt := BuildHiddenGemsPrompt(cfg, nil, "movie")
	if !strings.Contains(prompt, "never an error, refusal or apology") {
		t.Error("prompt should ask for best effort instead of a refusal")
	}
}

func TestBuildPrompt_SeasonedViewerAvoidsObviousPicks(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildPrompt(KindPicks, cfg, PromptInput{MediaType: "movie", WatchedCount: 573})
	if !strings.Contains(prompt, "already watched 573 movies and shows") {
		t.Error("a large watched list should steer away from the obvious hits")
	}
	if strings.Contains(BuildPrompt(KindPicks, cfg, PromptInput{MediaType: "movie", WatchedCount: 20}), "already watched") {
		t.Error("a small watched list shouldn't add the hint")
	}
}

func TestOverRequest(t *testing.T) {
	for _, tt := range []struct{ n, want int }{{10, 18}, {30, 45}, {50, 60}} {
		if got := OverRequest(tt.n); got != tt.want {
			t.Errorf("OverRequest(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestBuildHiddenGemsPrompt_MentionsUnderrated(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildHiddenGemsPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "underrated") && !strings.Contains(prompt, "Hidden Gems") {
		t.Error("hidden gems prompt should mention underrated or hidden gems")
	}
	if !strings.Contains(prompt, "lesser-known") {
		t.Error("hidden gems prompt should mention lesser-known")
	}
}

func TestBuildHiddenGemsPrompt_IncludesGenres(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{
		Language: "en",
		Genres:   []string{"thriller", "horror"},
	})

	prompt := BuildHiddenGemsPrompt(cfg, nil, "series")
	if !strings.Contains(strings.ToLower(prompt), "thriller") {
		t.Error("prompt should include genres")
	}
}

func TestBuildBecauseYouWatchedPrompt_IncludesTitle(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildBecauseYouWatchedPrompt("Inception (2010)", cfg, "movie")

	if !strings.Contains(prompt, "Inception (2010)") {
		t.Error("prompt should include the specific title")
	}
	if !strings.Contains(prompt, "just watched") {
		t.Error("prompt should reference what user just watched")
	}
}

func TestBuildBecauseYouWatchedPrompt_IncludesContentTypes(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{
		Language:     "en",
		ContentTypes: []string{"series"},
	})

	prompt := BuildBecauseYouWatchedPrompt("Breaking Bad (2008)", cfg, "series")
	if !strings.Contains(prompt, "series") {
		t.Error("prompt should include content types")
	}
}

func TestBuildAIPicksPrompt_NoPreferences(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "Recommend exactly") {
		t.Error("prompt should contain system prompt even without preferences")
	}
	if !strings.Contains(prompt, "personalized mix") {
		t.Error("prompt should use default message when no specific preferences")
	}
}

func TestBuildAIPicksPrompt_MediaTypeMovie(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "movies only") {
		t.Error("movie mediaType should instruct AI to return movies only")
	}
}

func TestBuildAIPicksPrompt_MediaTypeSeries(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildAIPicksPrompt(cfg, nil, "series")

	if !strings.Contains(prompt, "TV series only") {
		t.Error("series mediaType should instruct AI to return TV series only")
	}
}

func TestBuildAIPicksPrompt_MaxResults(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", MaxResults: 30})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	// Over-requests by ~30%: 30 + 9 = 39
	if !strings.Contains(prompt, "45") {
		t.Errorf("prompt should include over-requested count (45), got prompt: %s", prompt[:200])
	}
}

func TestBuildAIPicksPrompt_YearRange(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearFrom: 2015, YearTo: 2020})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "YEAR RANGE: 2015-2020") {
		t.Error("prompt should include year range")
	}
}

func TestBuildAIPicksPrompt_YearFromOnly(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearFrom: 2010})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "YEAR RANGE: 2010-present") {
		t.Error("prompt should include year from with present")
	}
}

func TestBuildAIPicksPrompt_YearToOnly(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearTo: 2000})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, fmt.Sprintf("YEAR RANGE: up to %d", 2000)) {
		t.Error("prompt should include year to")
	}
}

func TestBuildHiddenGemsPrompt_YearRange(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearFrom: 1990, YearTo: 2005})
	prompt := BuildHiddenGemsPrompt(cfg, nil, "movie")

	if !strings.Contains(prompt, "YEAR RANGE: 1990-2005") {
		t.Error("hidden gems prompt should include year range")
	}
}

func TestBuildFocusedPrompt_Anime(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildFocusedPrompt(cfg, nil, "", "anime")

	if !strings.Contains(prompt, "anime") {
		t.Error("anime focused prompt should mention anime")
	}
	if !strings.Contains(prompt, "no live-action") {
		t.Error("anime focused prompt should exclude live-action")
	}
	if !strings.Contains(prompt, "movies and TV series") {
		t.Error("focused prompt should ask for both types")
	}
}

func TestBuildFocusedPrompt_Documentary(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildFocusedPrompt(cfg, nil, "", "documentary")

	if !strings.Contains(prompt, "documentary") {
		t.Error("documentary focused prompt should mention documentary")
	}
	if !strings.Contains(prompt, "no fiction") {
		t.Error("documentary focused prompt should exclude fiction")
	}
}

func TestBuildFocusedPrompt_WithWatchHistory(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	history := []string{"Spirited Away (2001)"}
	prompt := BuildFocusedPrompt(cfg, history, "", "anime")

	if !strings.Contains(prompt, "Spirited Away (2001)") {
		t.Error("focused prompt should include watch history")
	}
	if !strings.Contains(prompt, "RECENTLY WATCHED") {
		t.Error("focused prompt should reference recently watched")
	}
}

func TestBuildAIPicksPrompt_NoAnimeDocFocusInBase(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{
		Language:     "en",
		ContentTypes: []string{"movie", "anime", "documentary"},
	})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if strings.Contains(prompt, "anime") {
		t.Error("base AI picks prompt should NOT contain anime focus")
	}
	if strings.Contains(prompt, "documentary") {
		t.Error("base AI picks prompt should NOT contain documentary focus")
	}
}

func TestBuildBecauseYouWatchedPrompt_YearRange(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearFrom: 2000, YearTo: 2010})
	prompt := BuildBecauseYouWatchedPrompt("Inception (2010)", cfg, "movie")

	if !strings.Contains(prompt, "YEAR RANGE: 2000-2010") {
		t.Error("because you watched prompt should include year range")
	}
}

func TestBuildHiddenGemsPrompt_VoteCapMatchesTheRow(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	if p := BuildHiddenGemsPrompt(cfg, nil, "series"); !strings.Contains(p, "fewer than 1,000 votes") {
		t.Error("series gems prompt should state the series vote cap")
	}
	if p := BuildHiddenGemsPrompt(cfg, nil, "movie"); !strings.Contains(p, "fewer than 5,000 votes") {
		t.Error("movie gems prompt should state the film vote cap")
	}
}

func TestBuildPrompt_StatesTheQualityFloor(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	if p := BuildAIPicksPrompt(cfg, nil, "movie"); !strings.Contains(p, "under 6.0") {
		t.Error("prompt should ask to skip poorly reviewed titles when no minimum is set")
	}
	cfg.MinRating = 7
	if p := BuildAIPicksPrompt(cfg, nil, "movie"); strings.Contains(p, "under 6.0") {
		t.Error("a higher user minimum already covers the floor")
	}
}
