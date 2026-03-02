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

	if !strings.Contains(prompt, "action") {
		t.Error("prompt should include genres")
	}
	if !strings.Contains(prompt, "exciting") {
		t.Error("prompt should include mood")
	}
	if !strings.Contains(prompt, "fr") {
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

func TestBuildAIPicksPrompt_DefaultLanguageNotIncluded(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en"})
	prompt := BuildAIPicksPrompt(cfg, nil, "movie")

	if strings.Contains(prompt, "PREFERRED LANGUAGE") {
		t.Error("default language 'en' should not add PREFERRED LANGUAGE line")
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
	if !strings.Contains(prompt, "thriller") {
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
	if !strings.Contains(prompt, "39") {
		t.Errorf("prompt should include over-requested count (39), got prompt: %s", prompt[:200])
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

func TestBuildBecauseYouWatchedPrompt_YearRange(t *testing.T) {
	cfg := cfgWithDefaults(&store.UserConfig{Language: "en", YearFrom: 2000, YearTo: 2010})
	prompt := BuildBecauseYouWatchedPrompt("Inception (2010)", cfg, "movie")

	if !strings.Contains(prompt, "YEAR RANGE: 2000-2010") {
		t.Error("because you watched prompt should include year range")
	}
}
