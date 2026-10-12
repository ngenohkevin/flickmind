package store

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeConfig_CleansValues(t *testing.T) {
	cfg := &UserConfig{
		GroqKey:              "  gsk_abc\n",
		Genres:               []string{"action", "Science Fiction", "ACTION", "Bogus", " thriller "},
		ContentTypes:         []string{"Movie", "podcast", "movie"},
		Language:             "EN",
		Mood:                 "Dark",
		MinRating:            12,
		YearFrom:             2026,
		YearTo:               2010,
		MaxResults:           500,
		RecommendationSource: "nonsense",
		RefreshInterval:      "5m",
	}
	NormalizeConfig(cfg)

	if cfg.GroqKey != "gsk_abc" {
		t.Errorf("GroqKey = %q, want trimmed", cfg.GroqKey)
	}
	if want := []string{"Action", "Sci-Fi", "Thriller"}; !reflect.DeepEqual(cfg.Genres, want) {
		t.Errorf("Genres = %v, want %v", cfg.Genres, want)
	}
	if want := []string{"movie"}; !reflect.DeepEqual(cfg.ContentTypes, want) {
		t.Errorf("ContentTypes = %v, want %v", cfg.ContentTypes, want)
	}
	if cfg.Language != "en" || cfg.Mood != "dark" {
		t.Errorf("Language/Mood = %q/%q", cfg.Language, cfg.Mood)
	}
	if cfg.MinRating != 10 {
		t.Errorf("MinRating = %v, want clamped 10", cfg.MinRating)
	}
	if cfg.YearFrom != 2010 || cfg.YearTo != 2026 {
		t.Errorf("years = %d-%d, want swapped 2010-2026", cfg.YearFrom, cfg.YearTo)
	}
	if cfg.MaxResults != MaxResultsCap {
		t.Errorf("MaxResults = %d, want %d", cfg.MaxResults, MaxResultsCap)
	}
	if cfg.RecommendationSource != "preferences" || cfg.RefreshInterval != "2h" {
		t.Errorf("source/interval = %q/%q", cfg.RecommendationSource, cfg.RefreshInterval)
	}
}

func TestNormalizeConfig_Defaults(t *testing.T) {
	cfg := &UserConfig{Language: "any", YearTo: 3000, MinRating: -1}
	NormalizeConfig(cfg)

	if want := []string{"movie", "series"}; !reflect.DeepEqual(cfg.ContentTypes, want) {
		t.Errorf("ContentTypes = %v, want %v", cfg.ContentTypes, want)
	}
	if cfg.Language != "" {
		t.Errorf("Language = %q, want any (empty)", cfg.Language)
	}
	if cfg.MaxResults != DefaultResults {
		t.Errorf("MaxResults = %d, want %d", cfg.MaxResults, DefaultResults)
	}
	if want := time.Now().Year() + 1; cfg.YearTo != want {
		t.Errorf("YearTo = %d, want clamped %d", cfg.YearTo, want)
	}
	if cfg.MinRating != 0 {
		t.Errorf("MinRating = %v, want 0", cfg.MinRating)
	}
}

func TestNormalizeConfig_KeepsValidConfig(t *testing.T) {
	cfg := &UserConfig{
		Genres: []string{"Crime"}, ContentTypes: []string{"series", "anime"}, Language: "ko",
		MinRating: 7.5, YearFrom: 2000, MaxResults: 30, RecommendationSource: "watchlist", RefreshInterval: "6h",
	}
	want := *cfg
	NormalizeConfig(cfg)
	if !reflect.DeepEqual(*cfg, want) {
		t.Errorf("valid config changed:\n got %+v\nwant %+v", *cfg, want)
	}
}
