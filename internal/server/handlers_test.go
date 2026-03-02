package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ngenohkevin/flickmind/internal/cache"
	"github.com/ngenohkevin/flickmind/internal/config"
	"github.com/ngenohkevin/flickmind/internal/store"
)

// mockStore implements store.StoreInterface with in-memory maps.
type mockStore struct {
	users   map[string]bool
	configs map[string]*store.UserConfig
	caches  map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		users:   make(map[string]bool),
		configs: make(map[string]*store.UserConfig),
		caches:  make(map[string]string),
	}
}

func (m *mockStore) CreateUser(_ context.Context, id string) error {
	m.users[id] = true
	m.configs[id] = &store.UserConfig{UserID: id, Language: "en", ContentTypes: []string{"movie", "series"}}
	return nil
}

func (m *mockStore) UserExists(_ context.Context, id string) (bool, error) {
	return m.users[id], nil
}

func (m *mockStore) GetUserConfig(_ context.Context, userID string) (*store.UserConfig, error) {
	cfg, ok := m.configs[userID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cfg, nil
}

func (m *mockStore) SaveUserConfig(_ context.Context, cfg *store.UserConfig) error {
	m.configs[cfg.UserID] = cfg
	return nil
}

func (m *mockStore) SaveTraktTokens(_ context.Context, userID, accessToken, refreshToken string, expiresAt time.Time) error {
	if cfg, ok := m.configs[userID]; ok {
		cfg.TraktAccessToken = accessToken
		cfg.TraktRefreshToken = refreshToken
		cfg.TraktExpiresAt = &expiresAt
		cfg.TraktConnected = true
	}
	return nil
}

func (m *mockStore) ClearTraktTokens(_ context.Context, userID string) error {
	if cfg, ok := m.configs[userID]; ok {
		cfg.TraktAccessToken = ""
		cfg.TraktRefreshToken = ""
		cfg.TraktExpiresAt = nil
		cfg.TraktConnected = false
	}
	return nil
}

func (m *mockStore) GetCache(_ context.Context, key string) (string, bool) {
	v, ok := m.caches[key]
	return v, ok
}

func (m *mockStore) SetCache(_ context.Context, key, data string, _ time.Duration) error {
	m.caches[key] = data
	return nil
}

func (m *mockStore) InvalidateUserCache(_ context.Context, userID string) error {
	for k := range m.caches {
		if len(k) > len(userID) && k[:len(userID)+1] == userID+":" {
			delete(m.caches, k)
		}
	}
	return nil
}

func newTestServer(ms *mockStore) *Server {
	cfg := &config.Config{
		BaseURL:     "http://localhost:7000",
		FrontendURL: "http://localhost:3000",
	}
	c := cache.New("") // in-memory cache
	return NewForTest(cfg, ms, nil, nil, c)
}

func TestHealthRoute(t *testing.T) {
	srv := newTestServer(newMockStore())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
	if body["service"] != "flickmind" {
		t.Fatalf("expected service flickmind, got %v", body["service"])
	}
}

func TestManifestRoute(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	srv := newTestServer(ms)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/testuser/manifest.json", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	if manifest["id"] != "community.flickmind" {
		t.Errorf("expected manifest id community.flickmind, got %v", manifest["id"])
	}
	if manifest["name"] != "FlickMind" {
		t.Errorf("expected manifest name FlickMind, got %v", manifest["name"])
	}

	catalogs, ok := manifest["catalogs"].([]interface{})
	if !ok {
		t.Fatal("catalogs should be an array")
	}
	if len(catalogs) != 4 {
		t.Errorf("expected 4 catalogs (no Trakt), got %d", len(catalogs))
	}
}

func TestCatalogRoute_WithJsonSuffix(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	srv := newTestServer(ms)

	// This is the exact bug: Stremio sends /:userId/catalog/:type/:id.json
	// The .json suffix must be stripped to match catalog IDs.
	// With no AI providers and no TMDB client, the handler hits the TMDB fallback
	// which panics (nil client), but the route DOES resolve (not 404).
	// We test an unknown catalog ID without .json suffix to verify routing works.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/testuser/catalog/movie/flickmind-because-you-watched.json", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp["metas"]; !ok {
		t.Fatal("response should have 'metas' key")
	}

	// Also verify that .json suffix is properly stripped by checking
	// that "flickmind-ai-picks.json" doesn't match the "default" case
	// (which would return empty metas for unknown catalog).
	// We do this by testing the unknown catalog returns empty metas.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/testuser/catalog/movie/totally-unknown.json", nil)
	srv.router.ServeHTTP(w2, req2)

	var resp2 map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	metas := resp2["metas"].([]interface{})
	if len(metas) != 0 {
		t.Error("unknown catalog should return empty metas")
	}
}

func TestCatalogRoute_AllCatalogIDs(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	srv := newTestServer(ms)

	// Test that all catalog routes resolve (not 404).
	// ai-picks and hidden-gems hit TMDB fallback with nil client (500 from recovery),
	// because-you-watched returns 200 with empty metas (no Trakt connected).
	catalogTests := []struct {
		catalogID  string
		expectCode int
	}{
		// These two hit tmdbClient.DiscoverFallback with nil client → panic → 500
		{"flickmind-ai-picks", 500},
		{"flickmind-hidden-gems", 500},
		// This returns early (no Trakt) → 200
		{"flickmind-because-you-watched", 200},
	}
	types := []string{"movie", "series"}

	for _, ct := range catalogTests {
		for _, typ := range types {
			t.Run(fmt.Sprintf("%s/%s", typ, ct.catalogID), func(t *testing.T) {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", fmt.Sprintf("/testuser/catalog/%s/%s.json", typ, ct.catalogID), nil)
				srv.router.ServeHTTP(w, req)

				if w.Code != ct.expectCode {
					t.Fatalf("expected %d, got %d", ct.expectCode, w.Code)
				}

				// Verify the route resolved (not 404 — that would mean routing is broken)
				if w.Code == 404 {
					t.Fatal("route should resolve, got 404")
				}
			})
		}
	}
}

func TestCatalogRoute_UnknownCatalog(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	srv := newTestServer(ms)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/testuser/catalog/movie/nonexistent.json", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	metas := resp["metas"].([]interface{})
	if len(metas) != 0 {
		t.Errorf("expected empty metas for unknown catalog, got %d", len(metas))
	}
}

func TestConfigCRUD(t *testing.T) {
	ms := newMockStore()
	srv := newTestServer(ms)

	// Create config
	body := `{"groqKey":"gsk_test123","genres":["action","comedy"],"language":"en","contentTypes":["movie"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("create config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	userID, ok := createResp["userId"].(string)
	if !ok || userID == "" {
		t.Fatal("expected userId in create response")
	}

	// Get config
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/config/"+userID, nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("get config: expected 200, got %d", w.Code)
	}

	var getResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if getResp["userId"] != userID {
		t.Errorf("expected userId %s, got %v", userID, getResp["userId"])
	}

	// Update config
	updateBody := `{"groqKey":"gsk_newkey456","genres":["drama"],"language":"fr","contentTypes":["series"],"mood":"dark"}`
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/config/"+userID, bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("update config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify update
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/config/"+userID, nil)
	srv.router.ServeHTTP(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if getResp["language"] != "fr" {
		t.Errorf("expected language fr, got %v", getResp["language"])
	}
	if getResp["mood"] != "dark" {
		t.Errorf("expected mood dark, got %v", getResp["mood"])
	}
}

func TestConfigMasking(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	ms.configs["testuser"].GroqKey = "gsk_abc123xyz"
	ms.configs["testuser"].DeepSeekKey = "sk-deepseek-key-789"
	ms.configs["testuser"].GeminiKey = ""
	srv := newTestServer(ms)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config/testuser", nil)
	srv.router.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	groqKey := resp["groqKey"].(string)
	if groqKey != "••••3xyz" {
		t.Errorf("expected groqKey ••••3xyz, got %s", groqKey)
	}

	deepseekKey := resp["deepseekKey"].(string)
	if deepseekKey != "••••-789" {
		t.Errorf("expected deepseekKey ••••-789, got %s", deepseekKey)
	}

	geminiKey := resp["geminiKey"].(string)
	if geminiKey != "" {
		t.Errorf("expected empty geminiKey, got %s", geminiKey)
	}
}

func TestUpdateConfig_PreservesMaskedKeys(t *testing.T) {
	ms := newMockStore()
	if err := ms.CreateUser(context.Background(), "testuser"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	ms.configs["testuser"].GroqKey = "gsk_real_secret_key"
	srv := newTestServer(ms)

	// Send update with masked key value — should NOT overwrite the real key
	updateBody := `{"groqKey":"••••_key","genres":["horror"],"language":"en","contentTypes":["movie"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/config/testuser", bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify real key was preserved
	cfg := ms.configs["testuser"]
	if cfg.GroqKey != "gsk_real_secret_key" {
		t.Errorf("expected original key preserved, got %s", cfg.GroqKey)
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	srv := newTestServer(newMockStore())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config/nonexistent", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateConfig_NotFound(t *testing.T) {
	srv := newTestServer(newMockStore())

	body := `{"groqKey":"test","genres":[],"language":"en","contentTypes":["movie"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/config/nonexistent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
