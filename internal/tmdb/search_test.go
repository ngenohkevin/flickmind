package tmdb

import (
	"testing"
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
