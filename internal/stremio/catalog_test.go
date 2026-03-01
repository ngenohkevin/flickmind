package stremio

import (
	"testing"

	"github.com/ngenohkevin/flickmind/internal/tmdb"
)

func TestTMDBResultToMeta_Movie(t *testing.T) {
	r := tmdb.SearchResult{
		ID:           123,
		Title:        "Inception",
		MediaType:    "movie",
		PosterPath:   "/poster.jpg",
		BackdropPath: "/backdrop.jpg",
		Overview:     "A mind-bending thriller.",
		VoteAverage:  8.8,
		Year:         2010,
	}

	meta := TMDBResultToMeta(r, "Great sci-fi")

	if meta.ID != "tmdb:123" {
		t.Errorf("expected id tmdb:123, got %s", meta.ID)
	}
	if meta.Name != "Inception" {
		t.Errorf("expected name Inception, got %s", meta.Name)
	}
	if meta.Type != "movie" {
		t.Errorf("expected type movie, got %s", meta.Type)
	}
	if meta.Poster != "https://image.tmdb.org/t/p/w500/poster.jpg" {
		t.Errorf("unexpected poster URL: %s", meta.Poster)
	}
	if meta.Background != "https://image.tmdb.org/t/p/w1280/backdrop.jpg" {
		t.Errorf("unexpected background URL: %s", meta.Background)
	}
	if meta.Year != "2010" {
		t.Errorf("expected year 2010, got %s", meta.Year)
	}
	if meta.IMDBRating != "8.8" {
		t.Errorf("expected rating 8.8, got %s", meta.IMDBRating)
	}
	if meta.Description != "Great sci-fi\n\nA mind-bending thriller." {
		t.Errorf("unexpected description: %s", meta.Description)
	}
}

func TestTMDBResultToMeta_TV(t *testing.T) {
	r := tmdb.SearchResult{
		ID:        456,
		Title:     "Breaking Bad",
		MediaType: "tv",
		Year:      2008,
	}

	meta := TMDBResultToMeta(r, "")

	if meta.Type != "series" {
		t.Errorf("expected type series for TV, got %s", meta.Type)
	}
}

func TestTMDBResultToMeta_EmptyPoster(t *testing.T) {
	r := tmdb.SearchResult{
		ID:        789,
		Title:     "No Poster",
		MediaType: "movie",
	}

	meta := TMDBResultToMeta(r, "")

	if meta.Poster != "" {
		t.Errorf("expected empty poster, got %s", meta.Poster)
	}
	if meta.Background != "" {
		t.Errorf("expected empty background, got %s", meta.Background)
	}
}

func TestTMDBResultToMeta_ZeroYear(t *testing.T) {
	r := tmdb.SearchResult{
		ID:        100,
		Title:     "Unknown Year",
		MediaType: "movie",
		Year:      0,
	}

	meta := TMDBResultToMeta(r, "")

	if meta.Year != "" {
		t.Errorf("expected empty year for 0, got %s", meta.Year)
	}
}

func TestTMDBResultToMeta_ZeroRating(t *testing.T) {
	r := tmdb.SearchResult{
		ID:          101,
		Title:       "Unrated",
		MediaType:   "movie",
		VoteAverage: 0,
	}

	meta := TMDBResultToMeta(r, "")

	if meta.IMDBRating != "" {
		t.Errorf("expected empty rating for 0, got %s", meta.IMDBRating)
	}
}

func TestTMDBResultToMeta_ReasonOnly(t *testing.T) {
	r := tmdb.SearchResult{
		ID:        102,
		Title:     "No Overview",
		MediaType: "movie",
	}

	meta := TMDBResultToMeta(r, "A great recommendation")

	if meta.Description != "A great recommendation" {
		t.Errorf("expected reason only in description, got %s", meta.Description)
	}
}

func TestTMDBResultToMeta_OverviewOnly(t *testing.T) {
	r := tmdb.SearchResult{
		ID:        103,
		Title:     "Has Overview",
		MediaType: "movie",
		Overview:  "Interesting plot.",
	}

	meta := TMDBResultToMeta(r, "")

	if meta.Description != "Interesting plot." {
		t.Errorf("expected overview only, got %s", meta.Description)
	}
}
