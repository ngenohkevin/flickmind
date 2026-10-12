package server

import (
	"strings"
	"testing"

	"github.com/ngenohkevin/flickmind/internal/prefs"
	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// liveCfg mirrors the live config whose catalogs ignored its settings.
func liveCfg() *store.UserConfig {
	return &store.UserConfig{
		Genres:       []string{"Action", "Adventure", "Documentary", "Crime", "Fantasy", "Mystery", "Sci-Fi", "Thriller", "War"},
		ContentTypes: []string{"movie", "series", "documentary"},
		Language:     "en", YearFrom: 2010, YearTo: 2026, MinRating: 6.5, MaxResults: 30,
	}
}

func movie(id int, title string, year int, lang string, rating float64, genres ...int) tmdb.SearchResult {
	return tmdb.SearchResult{ID: id, Title: title, MediaType: "movie", Year: year, OriginalLanguage: lang, VoteAverage: rating, VoteCount: 1000, GenreIDs: genres}
}

func show(id int, title string, year int, lang string, rating float64, genres ...int) tmdb.SearchResult {
	r := movie(id, title, year, lang, rating, genres...)
	r.MediaType = "tv"
	return r
}

func keptTitles(rs []tmdb.SearchResult) string {
	var names []string
	for _, r := range rs {
		names = append(names, r.Title)
	}
	return strings.Join(names, ", ")
}

func TestFilter_AIPicksMoviesHonorSettings(t *testing.T) {
	cfg := liveCfg()
	f := newResultFilter(prefs.For(cfg, prefs.Picks), allowedTypes(cfg, "movie"),
		[]trakt.WatchedItem{{Title: "Sicario", Year: 2016}})

	kept, summary := f.apply([]tmdb.SearchResult{
		movie(1, "Inception", 2010, "en", 8.4, 28, 878, 12),
		movie(2, "Parasite", 2019, "ko", 8.5, 35, 53, 18),
		movie(3, "Captain Underpants: The First Epic Movie", 2017, "en", 6.6, 16, 35, 10751),
		movie(4, "Free Solo", 2018, "en", 7.9, 99, 12),
		movie(5, "Get Out", 2017, "en", 7.6, 9648, 53, 27),
		movie(6, "The Last Witch Hunter", 2015, "en", 6.0, 28, 12, 14),
		movie(7, "Sicario", 2015, "en", 7.4, 28, 80, 53),
		movie(1, "Inception", 2010, "en", 8.4, 28, 878, 12),
		movie(8, "The Secret in Their Eyes", 2009, "en", 8.0, 80, 9648, 53),
		show(9, "Breaking Bad", 2008, "en", 9.0, 18, 80),
		movie(10, "Sicario: Day of the Soldado", 2018, "en", 6.8, 28, 80, 53),
	})

	if got := keptTitles(kept); got != "Inception, Sicario: Day of the Soldado" {
		t.Errorf("kept = %q", got)
	}
	for _, want := range []string{"language 1", "genre 3", "rating 1", "watched 1", "duplicate 1", "year 1", "type 1"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q missing %q", summary, want)
		}
	}
}

func TestFilter_SeriesUseMergedTVGenres(t *testing.T) {
	cfg := liveCfg()
	f := newResultFilter(prefs.For(cfg, prefs.Picks), allowedTypes(cfg, "series"), nil)

	kept, _ := f.apply([]tmdb.SearchResult{
		show(1, "The Expanse", 2015, "en", 8.1, 18, 9648, 10765), // Sci-Fi & Fantasy counts as sci-fi
		show(2, "The Crown", 2016, "en", 8.2, 18),                // drama only: no selected genre
		show(3, "Arcane", 2021, "en", 8.7, 16, 10759, 10765),     // animation is excluded
		show(4, "Survivor", 2000, "en", 7.0, 10764),              // reality, and too old
	})

	if got := keptTitles(kept); got != "The Expanse" {
		t.Errorf("kept = %q", got)
	}
}

func TestFilter_GemsDropWellKnownTitles(t *testing.T) {
	cfg := liveCfg()
	f := newResultFilter(prefs.For(cfg, prefs.Gems), allowedTypes(cfg, "series"), nil)

	dark := show(1, "Dark", 2017, "en", 8.4, 80, 18, 10765, 9648)
	dark.VoteCount = 7000
	gem := show(2, "The Lost Room", 2010, "en", 7.7, 9648, 10765)
	gem.VoteCount = 400

	kept, summary := f.apply([]tmdb.SearchResult{dark, gem})
	if got := keptTitles(kept); got != "The Lost Room" {
		t.Errorf("kept = %q (%s)", got, summary)
	}
}

func TestFilter_AnimeRowIgnoresLanguageButRequiresAnimation(t *testing.T) {
	cfg := liveCfg()
	cfg.ContentTypes = []string{"movie", "anime"}
	f := newResultFilter(prefs.For(cfg, prefs.Anime), allowedTypes(cfg, ""), nil)

	kept, _ := f.apply([]tmdb.SearchResult{
		movie(1, "Your Name.", 2016, "ja", 8.5, 16, 10749, 18), // romance not selected
		movie(2, "Promare", 2019, "ja", 7.5, 16, 28, 878),
		movie(3, "Shoplifters", 2018, "ja", 7.9, 18, 80),              // live action
		show(4, "Attack on Titan", 2013, "ja", 8.7, 16, 10759, 10765), // TV not selected
	})

	if got := keptTitles(kept); got != "Promare" {
		t.Errorf("kept = %q", got)
	}
}

func TestFilter_DocumentaryRow(t *testing.T) {
	cfg := liveCfg()
	f := newResultFilter(prefs.For(cfg, prefs.Documentary), allowedTypes(cfg, ""), nil)

	kept, _ := f.apply([]tmdb.SearchResult{
		movie(1, "Free Solo", 2018, "en", 7.9, 99, 12),
		show(2, "Planet Earth II", 2016, "en", 8.6, 99),
		movie(3, "Man on Wire", 2008, "en", 7.4, 99),        // before 2010
		movie(4, "Honeyland", 2019, "mk", 7.6, 99),          // not English
		movie(5, "The Social Network", 2010, "en", 7.4, 18), // not a documentary
	})

	if got := keptTitles(kept); got != "Free Solo, Planet Earth II" {
		t.Errorf("kept = %q", got)
	}
}

func TestFilter_AnyLanguageAndNoGenres(t *testing.T) {
	cfg := &store.UserConfig{ContentTypes: []string{"movie", "series"}}
	f := newResultFilter(prefs.For(cfg, prefs.Picks), allowedTypes(cfg, "movie"), nil)

	kept, _ := f.apply([]tmdb.SearchResult{
		movie(1, "Parasite", 2019, "ko", 8.5, 35, 53, 18),
		movie(2, "Amélie", 2001, "fr", 7.9, 35, 10749),
	})
	if len(kept) != 2 {
		t.Errorf("no settings should keep everything, kept %q", keptTitles(kept))
	}
}

func TestFilter_GemsNeedQualityAndVotes(t *testing.T) {
	cfg := liveCfg()
	cfg.MinRating = 0
	f := newResultFilter(prefs.For(cfg, prefs.Gems), allowedTypes(cfg, "movie"), nil)

	oneVote := movie(1, "Echoes of Silence", 2025, "en", 10, 53)
	oneVote.VoteCount = 1
	weak := movie(2, "A Dark Reflection", 2015, "en", 4.4, 53)
	good := movie(3, "Coherence", 2013, "en", 6.8, 878, 53)

	kept, summary := f.apply([]tmdb.SearchResult{oneVote, weak, good})
	if got := keptTitles(kept); got != "Coherence" {
		t.Errorf("kept = %q (%s)", got, summary)
	}
}

func TestFilter_BlocksWatchedByTMDBID(t *testing.T) {
	cfg := liveCfg()
	f := newResultFilter(prefs.For(cfg, prefs.Picks), allowedTypes(cfg, "series"), []trakt.WatchedItem{
		{Title: "Le Bureau des Légendes", Year: 2015, MediaType: "show", TMDBID: 62852},
	})

	kept, summary := f.apply([]tmdb.SearchResult{
		show(62852, "The Bureau", 2015, "en", 8.2, 80, 18),     // same show, different title
		show(1, "The Night Of", 2016, "en", 8.0, 80, 18, 9648), // not watched
	})
	if got := keptTitles(kept); got != "The Night Of" {
		t.Errorf("kept = %q (%s)", got, summary)
	}
}

func TestPromptHistory_RecentFirstDedupedAndCapped(t *testing.T) {
	history := []trakt.WatchedItem{{Title: "Andor", Year: 2022}, {Title: "Andor", Year: 2022}}
	watched := []trakt.WatchedItem{{Title: "Andor", Year: 2022}, {Title: "Sicario", Year: 2015}, {Title: "Arrival", Year: 2016}}

	got := promptHistory(history, watched, 2)
	if strings.Join(got, "|") != "Andor (2022)|Sicario (2015)" {
		t.Errorf("promptHistory = %v", got)
	}
}
