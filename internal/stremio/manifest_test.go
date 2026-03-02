package stremio

import (
	"testing"

	"github.com/ngenohkevin/flickmind/internal/store"
)

func TestBuildManifest_BaseCatalogs(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123"}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if m.ID != "community.flickmind" {
		t.Errorf("expected id community.flickmind, got %s", m.ID)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Version)
	}
	if m.Name != "FlickMind" {
		t.Errorf("expected name FlickMind, got %s", m.Name)
	}

	if len(m.Catalogs) != 4 {
		t.Fatalf("expected 4 base catalogs, got %d", len(m.Catalogs))
	}

	// Verify catalog types and IDs
	expected := []struct{ typ, id string }{
		{"movie", "flickmind-ai-picks"},
		{"series", "flickmind-ai-picks"},
		{"movie", "flickmind-hidden-gems"},
		{"series", "flickmind-hidden-gems"},
	}
	for i, exp := range expected {
		if m.Catalogs[i].Type != exp.typ || m.Catalogs[i].ID != exp.id {
			t.Errorf("catalog %d: expected %s/%s, got %s/%s", i, exp.typ, exp.id, m.Catalogs[i].Type, m.Catalogs[i].ID)
		}
	}
}

func TestBuildManifest_TraktConnected(t *testing.T) {
	cfg := &store.UserConfig{
		UserID:         "abc123",
		TraktConnected: true,
	}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Catalogs) != 6 {
		t.Fatalf("expected 6 catalogs with Trakt, got %d", len(m.Catalogs))
	}

	// Last two should be "because you watched"
	if m.Catalogs[4].ID != "flickmind-because-you-watched" {
		t.Errorf("expected because-you-watched catalog at index 4, got %s", m.Catalogs[4].ID)
	}
	if m.Catalogs[5].ID != "flickmind-because-you-watched" {
		t.Errorf("expected because-you-watched catalog at index 5, got %s", m.Catalogs[5].ID)
	}
}

func TestBuildManifest_ConfigurationURL(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123"}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if m.BehaviorHints == nil {
		t.Fatal("BehaviorHints should not be nil")
	}
	if !m.BehaviorHints.Configurable {
		t.Error("expected configurable to be true")
	}
	expected := "http://localhost:3000/configure/abc123"
	if m.BehaviorHints.ConfigurationURL != expected {
		t.Errorf("expected ConfigurationURL %s, got %s", expected, m.BehaviorHints.ConfigurationURL)
	}
}

func TestBuildManifest_NilConfig(t *testing.T) {
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", nil)

	if len(m.Catalogs) != 4 {
		t.Fatalf("nil config should produce 4 base catalogs, got %d", len(m.Catalogs))
	}
}

func TestBuildManifest_AnimeOnlyShowsBothTypes(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123", ContentTypes: []string{"anime"}}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Types) != 2 || m.Types[0] != "movie" || m.Types[1] != "series" {
		t.Errorf("anime should map to both movie and series types, got %v", m.Types)
	}
	if len(m.Catalogs) != 4 {
		t.Errorf("expected 4 catalogs for anime, got %d", len(m.Catalogs))
	}
}

func TestBuildManifest_MovieOnly(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123", ContentTypes: []string{"movie"}}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Types) != 1 || m.Types[0] != "movie" {
		t.Errorf("expected types [movie], got %v", m.Types)
	}
	if len(m.Catalogs) != 2 {
		t.Errorf("expected 2 catalogs for movie-only, got %d", len(m.Catalogs))
	}
}

func TestBuildManifest_SeriesOnly(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123", ContentTypes: []string{"series"}}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Types) != 1 || m.Types[0] != "series" {
		t.Errorf("expected types [series], got %v", m.Types)
	}
	if len(m.Catalogs) != 2 {
		t.Errorf("expected 2 catalogs for series-only, got %d", len(m.Catalogs))
	}
}

func TestBuildManifest_DocumentaryShowsBothTypes(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123", ContentTypes: []string{"documentary"}}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Types) != 2 || m.Types[0] != "movie" || m.Types[1] != "series" {
		t.Errorf("documentary should map to both movie and series types, got %v", m.Types)
	}
}

func TestBuildManifest_LogoIsPNG(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123"}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	expected := "http://localhost:3000/apple-touch-icon.png"
	if m.Logo != expected {
		t.Errorf("expected logo %s, got %s", expected, m.Logo)
	}
}

func TestBuildManifest_ResourcesAndTypes(t *testing.T) {
	cfg := &store.UserConfig{UserID: "abc123"}
	m := BuildManifest("http://localhost:7000", "http://localhost:3000", "abc123", cfg)

	if len(m.Resources) != 1 || m.Resources[0] != "catalog" {
		t.Errorf("expected resources [catalog], got %v", m.Resources)
	}
	if len(m.Types) != 2 || m.Types[0] != "movie" || m.Types[1] != "series" {
		t.Errorf("expected types [movie, series], got %v", m.Types)
	}
}
