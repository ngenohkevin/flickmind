package tmdb

import (
	"strings"
	"testing"
	"time"
)

func TestGenreIDsToNames_Movie(t *testing.T) {
	names := GenreIDsToNames([]int{28, 878, 18}, "movie")
	if len(names) != 3 {
		t.Fatalf("expected 3 genres, got %d", len(names))
	}
	if names[0] != "Action" {
		t.Errorf("expected Action, got %s", names[0])
	}
	if names[1] != "Sci-Fi" {
		t.Errorf("expected Sci-Fi, got %s", names[1])
	}
	if names[2] != "Drama" {
		t.Errorf("expected Drama, got %s", names[2])
	}
}

func TestGenreIDsToNames_TV(t *testing.T) {
	names := GenreIDsToNames([]int{10759, 18}, "tv")
	if len(names) != 2 {
		t.Fatalf("expected 2 genres, got %d", len(names))
	}
	if names[0] != "Action & Adventure" {
		t.Errorf("expected Action & Adventure, got %s", names[0])
	}
	if names[1] != "Drama" {
		t.Errorf("expected Drama, got %s", names[1])
	}
}

func TestGenreIDsToNames_UnknownIDs(t *testing.T) {
	names := GenreIDsToNames([]int{99999, 88888}, "movie")
	if len(names) != 0 {
		t.Errorf("expected 0 genres for unknown IDs, got %d", len(names))
	}
}

func TestGenreIDsToNames_Empty(t *testing.T) {
	names := GenreIDsToNames(nil, "movie")
	if names != nil {
		t.Errorf("expected nil for nil input, got %v", names)
	}
}

func TestBestMatch_RejectsPopularWrongTitle(t *testing.T) {
	// Searching "The Captain" (2017) ranks the cartoon first by popularity.
	cands := []candidate{
		{title: "Captain Underpants: The First Epic Movie", year: 2017, votes: 1500},
		{title: "The Captain", original: "Der Hauptmann", year: 2017, votes: 300},
	}
	if got := bestMatch(cands, "The Captain", 2017, 0); got != 1 {
		t.Errorf("bestMatch = %d, want 1 (The Captain)", got)
	}
}

func TestBestMatch_NoTitleMatchReturnsNone(t *testing.T) {
	cands := []candidate{
		{title: "There Must Be a Safe Space to Load the Building Materials", year: 2018},
		{title: "The Last Witch Hunter", year: 2015, votes: 4000},
	}
	if got := bestMatch(cands, "The Load", 2018, 1); got != -1 {
		t.Errorf("bestMatch = %d, want -1", got)
	}
	if got := bestMatch(cands, "The Witch", 2015, 1); got != -1 {
		t.Errorf("bestMatch = %d, want -1 for The Witch", got)
	}
}

func TestBestMatch_SameTitlePrefersMoreVotes(t *testing.T) {
	cands := []candidate{
		{title: "Under the Skin", year: 2013, votes: 3},
		{title: "Under the Skin", year: 2013, votes: 4200},
	}
	if got := bestMatch(cands, "Under the Skin", 2013, 0); got != 1 {
		t.Errorf("bestMatch = %d, want 1 (the well-known one)", got)
	}
}

func TestBestMatch_YearToleranceAndOriginalTitle(t *testing.T) {
	cands := []candidate{
		{title: "The Bureau", original: "Le Bureau des Légendes", year: 2015, votes: 600},
	}
	if got := bestMatch(cands, "Le Bureau des Légendes", 2016, 1); got != 0 {
		t.Errorf("original title within 1 year should match, got %d", got)
	}
	if got := bestMatch(cands, "The Bureau", 2018, 1); got != -1 {
		t.Errorf("3 years off should not match, got %d", got)
	}
	if got := bestMatch(cands, "the bureau!", 0, 0); got != 0 {
		t.Errorf("unknown year + punctuation should match, got %d", got)
	}
}

func TestBestMatch_SubtitleSameYear(t *testing.T) {
	cands := []candidate{{title: "Borat: Cultural Learnings of America for Make Benefit Glorious Nation of Kazakhstan", year: 2006}}
	if got := bestMatch(cands, "Borat", 2006, 0); got != 0 {
		t.Errorf("base title before colon should match in the same year, got %d", got)
	}
}

func TestConfigGenres_TVMergedGenres(t *testing.T) {
	got := ConfigGenres([]int{10765, 10759, 18}, "tv")
	want := []string{"sci-fi", "fantasy", "action", "adventure", "drama"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ConfigGenres = %v, want %v", got, want)
	}
	if got := ConfigGenres([]int{27, 878}, "movie"); strings.Join(got, ",") != "horror,sci-fi" {
		t.Errorf("movie ConfigGenres = %v", got)
	}
}

func TestDiscoverParams_PerTypeDatesAndLanguage(t *testing.T) {
	o := DiscoverOpts{Genres: []string{"Action"}, YearFrom: 2010, YearTo: 2026, Language: "en", MinRating: 7}
	tv := discoverParams(o, "series")
	if tv.Get("first_air_date.gte") != "2010-01-01" || tv.Get("primary_release_date.gte") != "" {
		t.Errorf("series should filter first_air_date: %v", tv)
	}
	if tv.Get("with_genres") != "10759" {
		t.Errorf("series Action should map to 10759, got %q", tv.Get("with_genres"))
	}
	mv := discoverParams(o, "movie")
	if mv.Get("primary_release_date.lte") != "2026-12-31" || mv.Get("with_genres") != "28" {
		t.Errorf("movie params wrong: %v", mv)
	}
	if mv.Get("with_original_language") != "en" || mv.Get("vote_average.gte") != "7.0" {
		t.Errorf("language/rating missing: %v", mv)
	}
}

func TestDiscoverParams_GemsWindow(t *testing.T) {
	cutoff := time.Date(2025, 10, 12, 0, 0, 0, 0, time.UTC)
	p := discoverParams(DiscoverOpts{YearTo: 2026, MinVotes: 300, MaxVotes: 5000, ReleasedBefore: cutoff}, "movie")
	if p.Get("primary_release_date.lte") != "2025-10-12" {
		t.Errorf("release cutoff should tighten YearTo, got %q", p.Get("primary_release_date.lte"))
	}
	if p.Get("vote_count.gte") != "300" || p.Get("vote_count.lte") != "5000" {
		t.Errorf("vote window = %s..%s", p.Get("vote_count.gte"), p.Get("vote_count.lte"))
	}
}
