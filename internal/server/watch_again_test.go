package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

var waNow = time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)

// waLib is a fake TMDB keyed "movie:<id>" / "tv:<id>".
type waLib map[string]*tmdb.Details

func (l waLib) lookup(mediaType string, id int) *tmdb.Details {
	return l[fmt.Sprintf("%s:%d", mediaType, id)]
}

func waMovie(lib waLib, id int, title, lastWatched string, rating float64, votes int, genres ...string) trakt.WatchedItem {
	if len(genres) == 0 {
		genres = []string{"Drama"}
	}
	lib[fmt.Sprintf("movie:%d", id)] = &tmdb.Details{
		SearchResult: tmdb.SearchResult{ID: id, Title: title, MediaType: "movie", VoteAverage: rating, VoteCount: votes, IMDBId: fmt.Sprintf("tt%07d", id)},
		GenreNames:   genres,
	}
	return trakt.WatchedItem{Title: title, MediaType: "movie", TMDBID: id, LastWatchedAt: lastWatched, Plays: 1}
}

func waShow(lib waLib, id int, title, lastWatched string, plays, episodes int, genres ...string) trakt.WatchedItem {
	if len(genres) == 0 {
		genres = []string{"Drama"}
	}
	lib[fmt.Sprintf("tv:%d", id)] = &tmdb.Details{
		SearchResult:     tmdb.SearchResult{ID: id, Title: title, MediaType: "tv", VoteAverage: 8.0, VoteCount: 5000, IMDBId: fmt.Sprintf("tt%07d", id)},
		GenreNames:       genres,
		NumberOfEpisodes: episodes,
	}
	return trakt.WatchedItem{Title: title, MediaType: "show", TMDBID: id, LastWatchedAt: lastWatched, Plays: plays}
}

func waTitles(rs []tmdb.SearchResult) map[string]bool {
	out := map[string]bool{}
	for _, r := range rs {
		out[r.Title] = true
	}
	return out
}

const old = "2026-01-01T00:00:00.000Z"

func TestWatchAgainNeeds90Days(t *testing.T) {
	lib := waLib{}
	w := []trakt.WatchedItem{
		waMovie(lib, 1, "Recent", "2026-09-01T00:00:00.000Z", 7.5, 1000),
		waMovie(lib, 2, "Old", "2026-06-01T00:00:00.000Z", 7.5, 1000),
	}
	got := waTitles(chooseWatchAgain(w, nil, lib.lookup, "movie", waNow, 30))
	if got["Recent"] || !got["Old"] {
		t.Errorf("want only Old, got %v", got)
	}
}

func TestWatchAgainRatingFloor(t *testing.T) {
	lib := waLib{}
	w := []trakt.WatchedItem{
		waMovie(lib, 1, "LowRating", old, 6.9, 1000),
		waMovie(lib, 2, "FewVotes", old, 7.0, 99),
		waMovie(lib, 3, "JustIn", old, 7.0, 100),
	}
	got := waTitles(chooseWatchAgain(w, nil, lib.lookup, "movie", waNow, 30))
	if got["LowRating"] || got["FewVotes"] || !got["JustIn"] {
		t.Errorf("want only JustIn, got %v", got)
	}
}

func TestWatchAgainSkipsAlwaysExcludedGenres(t *testing.T) {
	lib := waLib{}
	w := []trakt.WatchedItem{
		waShow(lib, 1, "Island", old, 50, 50, "Reality"),
		waShow(lib, 2, "Drama Show", old, 50, 50, "Drama"),
	}
	got := waTitles(chooseWatchAgain(w, nil, lib.lookup, "series", waNow, 30))
	if got["Island"] || !got["Drama Show"] {
		t.Errorf("want only Drama Show, got %v", got)
	}
}

func TestWatchAgainShowsNeed80Percent(t *testing.T) {
	lib := waLib{}
	w := []trakt.WatchedItem{
		waShow(lib, 1, "Dropped", old, 39, 50),
		waShow(lib, 2, "Finished", old, 40, 50),
		waShow(lib, 3, "Rewatched", old, 70, 50),
	}
	got := waTitles(chooseWatchAgain(w, nil, lib.lookup, "series", waNow, 30))
	if got["Dropped"] || !got["Finished"] || !got["Rewatched"] {
		t.Errorf("want Finished and Rewatched, got %v", got)
	}
}

func TestWatchAgainSkipsLowTraktRatings(t *testing.T) {
	lib := waLib{}
	w := []trakt.WatchedItem{
		waMovie(lib, 1, "Disliked", old, 8.0, 1000),
		waMovie(lib, 2, "Liked", old, 8.0, 1000),
	}
	ratings := map[string]int{"movie:1": 5, "movie:2": 6}
	got := waTitles(chooseWatchAgain(w, ratings, lib.lookup, "movie", waNow, 30))
	if got["Disliked"] || !got["Liked"] {
		t.Errorf("want only Liked, got %v", got)
	}
}

func TestWatchAgainIsStableWithinADayAndChangesAcrossDays(t *testing.T) {
	lib := waLib{}
	var w []trakt.WatchedItem
	for i := 1; i <= 20; i++ {
		w = append(w, waMovie(lib, i, fmt.Sprintf("Film %d", i), old, 7.0+float64(i%5)/10, 1000))
	}
	order := func(now time.Time) string {
		var s string
		for _, r := range chooseWatchAgain(w, nil, lib.lookup, "movie", now, 30) {
			s += r.Title + ","
		}
		return s
	}
	if order(waNow) != order(waNow.Add(3*time.Hour)) {
		t.Error("order changed within a day")
	}
	if order(waNow) == order(waNow.Add(24*time.Hour)) {
		t.Error("order did not change the next day")
	}
}

func TestWatchAgainCapsAtWant(t *testing.T) {
	lib := waLib{}
	var w []trakt.WatchedItem
	for i := 1; i <= 50; i++ {
		w = append(w, waMovie(lib, i, fmt.Sprintf("Film %d", i), old, 7.5, 1000))
	}
	if n := len(chooseWatchAgain(w, nil, lib.lookup, "movie", waNow, 30)); n != 30 {
		t.Errorf("got %d titles, want 30", n)
	}
}
