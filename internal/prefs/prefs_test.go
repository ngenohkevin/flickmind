package prefs

import (
	"reflect"
	"testing"

	"github.com/ngenohkevin/flickmind/internal/store"
)

// kevinCfg mirrors the live config that motivated these rules.
func kevinCfg() *store.UserConfig {
	return &store.UserConfig{
		Genres:       []string{"Action", "Adventure", "Documentary", "Crime", "Fantasy", "Mystery", "Sci-Fi", "Thriller", "War"},
		ContentTypes: []string{"movie", "series", "documentary"},
		Language:     "en", YearFrom: 2010, YearTo: 2026, MinRating: 6.5,
	}
}

func TestFor_PicksExcludesUnselectedGenres(t *testing.T) {
	r := For(kevinCfg(), Picks)

	wantSelected := []string{"action", "adventure", "crime", "fantasy", "mystery", "sci-fi", "thriller", "war"}
	if !reflect.DeepEqual(r.Selected, wantSelected) {
		t.Errorf("Selected = %v, want %v (documentary has its own row)", r.Selected, wantSelected)
	}
	wantExcluded := []string{"animation", "comedy", "documentary", "horror", "romance", "family", "kids", "news", "reality", "soap", "talk"}
	if !reflect.DeepEqual(r.Excluded, wantExcluded) {
		t.Errorf("Excluded = %v, want %v", r.Excluded, wantExcluded)
	}
	if !reflect.DeepEqual(r.Languages, []string{"en"}) {
		t.Errorf("Languages = %v, want [en]", r.Languages)
	}
	if r.YearFrom != 2010 || r.YearTo != 2026 || r.MinRating != 6.5 {
		t.Errorf("year/rating not carried over: %+v", r)
	}
	if r.MaxVotes != 0 {
		t.Errorf("picks should not cap votes, got %d", r.MaxVotes)
	}
}

func TestFor_DramaIsNeverExcluded(t *testing.T) {
	r := For(&store.UserConfig{Genres: []string{"Thriller"}}, Picks)
	for _, g := range r.Excluded {
		if g == "drama" {
			t.Fatal("drama must stay neutral")
		}
	}
}

func TestFor_NoGenresMeansAnyGenre(t *testing.T) {
	r := For(&store.UserConfig{}, Picks)
	if len(r.Selected) != 0 {
		t.Errorf("Selected = %v, want none", r.Selected)
	}
	want := []string{"kids", "news", "reality", "soap", "talk"}
	if !reflect.DeepEqual(r.Excluded, want) {
		t.Errorf("Excluded = %v, want only %v", r.Excluded, want)
	}
}

func TestFor_FamilyAllowedWithComedyOrAnimation(t *testing.T) {
	r := For(&store.UserConfig{Genres: []string{"Comedy"}}, Picks)
	for _, g := range r.Excluded {
		if g == "family" {
			t.Fatal("family should be allowed when comedy is selected")
		}
	}
}

func TestFor_GemsCapsVotesAndSetsQualityFloor(t *testing.T) {
	r := For(kevinCfg(), Gems)
	if r.MaxVotes != GemsMaxVotes || r.MinVotes != GemsMinVotes {
		t.Errorf("votes = %d..%d, want %d..%d", r.MinVotes, r.MaxVotes, GemsMinVotes, GemsMaxVotes)
	}
	if r.MinRating != GemsMinRating {
		t.Errorf("MinRating = %v, want %v", r.MinRating, GemsMinRating)
	}
	cfg := kevinCfg()
	cfg.MinRating = 8
	if r := For(cfg, Gems); r.MinRating != 8 {
		t.Errorf("a higher user minimum should win, got %v", r.MinRating)
	}
}

func TestFor_Anime(t *testing.T) {
	r := For(kevinCfg(), Anime)
	if r.Required != "animation" {
		t.Errorf("Required = %q, want animation", r.Required)
	}
	if len(r.Languages) != 0 {
		t.Errorf("anime must not enforce language, got %v", r.Languages)
	}
	if !reflect.DeepEqual(r.Excluded, []string{"horror", "romance"}) {
		t.Errorf("Excluded = %v, want [horror romance]", r.Excluded)
	}
}

func TestFor_Documentary(t *testing.T) {
	r := For(kevinCfg(), Documentary)
	if r.Required != "documentary" || len(r.Excluded) != 0 || len(r.Selected) != 0 {
		t.Errorf("unexpected documentary rules: %+v", r)
	}
	if !reflect.DeepEqual(r.Languages, []string{"en"}) {
		t.Errorf("Languages = %v, want [en]", r.Languages)
	}
}

func TestLanguages(t *testing.T) {
	if Languages("") != nil {
		t.Error("empty language should mean any")
	}
	if !reflect.DeepEqual(Languages("zh"), []string{"zh", "cn"}) {
		t.Error("zh should also accept cn")
	}
}

func TestForMedia_GemsCapIsLowerForSeries(t *testing.T) {
	// Shows collect far fewer TMDB votes than films: Andor has ~2.3k,
	// a real gem like The Shadow Line under 100.
	if r := ForMedia(kevinCfg(), Gems, "series"); r.MaxVotes != GemsMaxVotesSeries {
		t.Errorf("series MaxVotes = %d, want %d", r.MaxVotes, GemsMaxVotesSeries)
	}
	if r := ForMedia(kevinCfg(), Gems, "movie"); r.MaxVotes != GemsMaxVotes {
		t.Errorf("movie MaxVotes = %d, want %d", r.MaxVotes, GemsMaxVotes)
	}
	if r := ForMedia(kevinCfg(), Picks, "series"); r.MaxVotes != 0 {
		t.Errorf("picks must not cap votes, got %d", r.MaxVotes)
	}
	if GemsMaxVotesSeries >= GemsMaxVotes {
		t.Error("the series cap should be the lower one")
	}
}

func TestFor_EveryRowHasAQualityFloor(t *testing.T) {
	cfg := kevinCfg()
	cfg.MinRating = 0 // "any rating" on the config page
	for _, kind := range []Kind{Picks, Gems, Anime, Documentary} {
		if r := For(cfg, kind); r.QualityFloor != QualityFloor {
			t.Errorf("kind %d: QualityFloor = %v, want %v", kind, r.QualityFloor, QualityFloor)
		}
	}
}
