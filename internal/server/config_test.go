package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func postConfig(t *testing.T, srv *Server, userID, body string) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/config/"+userID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
}

func TestUpdateConfig_ProviderKeys(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	ms.configs["u1"].GroqKey = "gsk_old"
	ms.configs["u1"].DeepSeekKey = "sk-old"
	ms.configs["u1"].GeminiKey = "AIza_old"
	srv := newTestServer(ms)

	// Groq switched off on the page (""), DeepSeek unchanged (masked), Gemini absent.
	postConfig(t, srv, "u1", `{"groqKey":"","deepseekKey":"••••-old","contentTypes":["movie"]}`)

	cfg := ms.configs["u1"]
	if cfg.GroqKey != "" {
		t.Errorf("disabled provider key should be removed, got %q", cfg.GroqKey)
	}
	if cfg.DeepSeekKey != "sk-old" || cfg.GeminiKey != "AIza_old" {
		t.Errorf("masked/absent keys should be kept: deepseek=%q gemini=%q", cfg.DeepSeekKey, cfg.GeminiKey)
	}

	postConfig(t, srv, "u1", `{"groqKey":"  gsk_new\n","contentTypes":["movie"]}`)
	if got := ms.configs["u1"].GroqKey; got != "gsk_new" {
		t.Errorf("pasted key should be trimmed, got %q", got)
	}
}

func TestUpdateConfig_NormalizesSettings(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)

	postConfig(t, srv, "u1", `{"genres":["horror","Bogus","HORROR"],"contentTypes":["series","podcast"],
		"language":"KO","mood":"weird","minRating":15,"yearFrom":2026,"yearTo":2010,"maxResults":999,
		"recommendationSource":"x","refreshInterval":"5m"}`)

	cfg := ms.configs["u1"]
	if !reflect.DeepEqual(cfg.Genres, []string{"Horror"}) || !reflect.DeepEqual(cfg.ContentTypes, []string{"series"}) {
		t.Errorf("genres/types = %v / %v", cfg.Genres, cfg.ContentTypes)
	}
	if cfg.Language != "ko" || cfg.Mood != "" || cfg.MinRating != 10 {
		t.Errorf("language/mood/rating = %q/%q/%v", cfg.Language, cfg.Mood, cfg.MinRating)
	}
	if cfg.YearFrom != 2010 || cfg.YearTo != 2026 || cfg.MaxResults != 50 {
		t.Errorf("years/max = %d-%d/%d", cfg.YearFrom, cfg.YearTo, cfg.MaxResults)
	}
	if cfg.RecommendationSource != "preferences" || cfg.RefreshInterval != "2h" {
		t.Errorf("source/interval = %q/%q", cfg.RecommendationSource, cfg.RefreshInterval)
	}
}
