package tmdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetailsMovie(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/273481" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"id":273481,"title":"Sicario","release_date":"2015-09-17","vote_average":7.4,"vote_count":10000,
			"genres":[{"id":28,"name":"Action"},{"id":53,"name":"Thriller"}],"imdb_id":"tt3397884",
			"backdrop_path":"/b.jpg","poster_path":"/p.jpg","overview":"o","original_language":"en"}`)
	}))
	defer ts.Close()

	d, err := NewClient("key").WithBaseURL(ts.URL).Details(context.Background(), "movie", 273481)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 273481 || d.Title != "Sicario" || d.Year != 2015 || d.MediaType != "movie" || d.IMDBId != "tt3397884" {
		t.Errorf("unexpected movie: %+v", d)
	}
	if d.VoteAverage != 7.4 || d.VoteCount != 10000 || fmt.Sprint(d.GenreNames) != "[Action Thriller]" {
		t.Errorf("rating/genres wrong: %+v", d)
	}
}

func TestDetailsTVCountsEpisodes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/76479" || r.URL.Query().Get("append_to_response") != "external_ids" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"id":76479,"name":"The Boys","first_air_date":"2019-07-25","vote_average":8.4,"vote_count":11000,
			"genres":[{"id":10759,"name":"Action & Adventure"}],"number_of_episodes":62,"external_ids":{"imdb_id":"tt1190634"}}`)
	}))
	defer ts.Close()

	d, err := NewClient("key").WithBaseURL(ts.URL).Details(context.Background(), "tv", 76479)
	if err != nil {
		t.Fatal(err)
	}
	if d.NumberOfEpisodes != 62 || d.IMDBId != "tt1190634" || d.Title != "The Boys" || d.MediaType != "tv" || d.Year != 2019 {
		t.Errorf("unexpected show: %+v", d)
	}
}
