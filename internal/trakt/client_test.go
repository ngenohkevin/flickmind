package trakt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetWatched_FollowsPagesForMoviesAndShows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		page := r.URL.Query().Get("page")
		switch r.URL.Path {
		case "/sync/watched/movies":
			w.Header().Set("X-Pagination-Page-Count", "2")
			if page == "1" {
				fmt.Fprint(w, `[{"last_watched_at":"2026-01-01T00:00:00.000Z","movie":{"title":"Sicario","year":2015,"ids":{"tmdb":273481}}}]`)
			} else {
				fmt.Fprint(w, `[{"last_watched_at":"2026-09-01T00:00:00.000Z","movie":{"title":"Arrival","year":2016,"ids":{"tmdb":329865}}}]`)
			}
		case "/sync/watched/shows":
			if r.URL.Query().Get("extended") != "noseasons" {
				t.Error("shows should be fetched without per-episode detail")
			}
			w.Header().Set("X-Pagination-Page-Count", "1")
			fmt.Fprint(w, `[{"last_watched_at":"2026-05-01T00:00:00.000Z","show":{"title":"The Boys","year":2019,"ids":{"tmdb":76479}}}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()

	items, err := NewClient("cid").WithBaseURL(ts.URL).GetWatched(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items across pages, got %d: %+v", len(items), items)
	}
	order := items[0].Title + "," + items[1].Title + "," + items[2].Title
	if order != "Arrival,The Boys,Sicario" {
		t.Errorf("expected most recently watched first, got %s", order)
	}
	if items[1].MediaType != "show" || items[1].TMDBID != 76479 {
		t.Errorf("show not parsed: %+v", items[1])
	}
}

func TestGetWatched_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer ts.Close()
	if _, err := NewClient("cid").WithBaseURL(ts.URL).GetWatched(context.Background(), "bad"); err != ErrUnauthorized {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestGetWatchedKeepsLastWatchedAndPlays(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		switch r.URL.Path {
		case "/sync/watched/movies":
			fmt.Fprint(w, `[{"plays":2,"last_watched_at":"2026-02-01T00:00:00.000Z","movie":{"title":"Sicario","year":2015,"ids":{"tmdb":273481}}}]`)
		case "/sync/watched/shows":
			fmt.Fprint(w, `[{"plays":40,"last_watched_at":"2026-03-01T00:00:00.000Z","show":{"title":"The Boys","year":2019,"ids":{"tmdb":76479}}}]`)
		}
	}))
	defer ts.Close()

	items, err := NewClient("cid").WithBaseURL(ts.URL).GetWatched(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "The Boys" {
		t.Fatalf("expected The Boys (newest) first, got %+v", items)
	}
	if items[0].Plays != 40 || items[0].LastWatchedAt != "2026-03-01T00:00:00.000Z" {
		t.Errorf("show lost plays/last watched: %+v", items[0])
	}
	if items[1].Plays != 2 || items[1].LastWatchedAt != "2026-02-01T00:00:00.000Z" {
		t.Errorf("movie lost plays/last watched: %+v", items[1])
	}
}

func TestGetRatingsReadsMoviesAndShows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/sync/ratings/movies":
			fmt.Fprint(w, `[{"rated_at":"2026-01-01T00:00:00.000Z","rating":9,"type":"movie","movie":{"title":"Sicario","year":2015,"ids":{"tmdb":273481}}}]`)
		case "/sync/ratings/shows":
			fmt.Fprint(w, `[{"rated_at":"2026-01-02T00:00:00.000Z","rating":4,"type":"show","show":{"title":"The Boys","year":2019,"ids":{"tmdb":76479}}}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()

	got, err := NewClient("cid").WithBaseURL(ts.URL).GetRatings(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := []Rating{{MediaType: "movie", TMDBID: 273481, Rating: 9}, {MediaType: "show", TMDBID: 76479, Rating: 4}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
