package server

import (
	"testing"

	"github.com/ngenohkevin/flickmind/internal/tmdb"
)

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "••••"},
		{"sk-abc123xyz", "••••3xyz"},
		{"abcd", "••••"},
		{"a", "••••"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := maskKey(tt.input)
			if result != tt.expected {
				t.Errorf("maskKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFilterByType_Movie(t *testing.T) {
	results := []tmdb.SearchResult{
		{ID: 1, Title: "Movie A", MediaType: "movie"},
		{ID: 2, Title: "Show B", MediaType: "tv"},
		{ID: 3, Title: "Movie C", MediaType: "movie"},
	}

	metas := filterByType(results, "movie")
	if len(metas) != 2 {
		t.Fatalf("expected 2 movies, got %d", len(metas))
	}
	if metas[0].Name != "Movie A" {
		t.Errorf("expected Movie A, got %s", metas[0].Name)
	}
	if metas[1].Name != "Movie C" {
		t.Errorf("expected Movie C, got %s", metas[1].Name)
	}
}

func TestFilterByType_Series(t *testing.T) {
	results := []tmdb.SearchResult{
		{ID: 1, Title: "Movie A", MediaType: "movie"},
		{ID: 2, Title: "Show B", MediaType: "tv"},
	}

	metas := filterByType(results, "series")
	if len(metas) != 1 {
		t.Fatalf("expected 1 series, got %d", len(metas))
	}
	if metas[0].Name != "Show B" {
		t.Errorf("expected Show B, got %s", metas[0].Name)
	}
}

func TestFilterByType_EmptyMediaType(t *testing.T) {
	results := []tmdb.SearchResult{
		{ID: 1, Title: "Movie A", MediaType: "movie"},
		{ID: 2, Title: "Show B", MediaType: "tv"},
	}

	metas := filterByType(results, "")
	if len(metas) != 2 {
		t.Fatalf("expected all results with empty type, got %d", len(metas))
	}
}

func TestFilterByType_EmptyResults(t *testing.T) {
	metas := filterByType(nil, "movie")
	if metas == nil {
		t.Fatal("filterByType should return empty slice, not nil")
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 metas, got %d", len(metas))
	}
}
