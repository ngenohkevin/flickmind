package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngenohkevin/flickmind/internal/cache"
	"github.com/ngenohkevin/flickmind/internal/config"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// fakeTrakt serves the token endpoint with single-use refresh tokens, like
// Trakt: "good-refresh" works once, anything else is invalid_grant.
func fakeTrakt(t *testing.T, refreshes *int32) *httptest.Server {
	t.Helper()
	var used sync.Map
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			atomic.AddInt32(refreshes, 1)
			time.Sleep(20 * time.Millisecond) // widen the race window
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, spent := used.LoadOrStore(body["refresh_token"], true); spent || body["refresh_token"] != "good-refresh" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error_description":"session not found","error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(trakt.TokenResponse{
				AccessToken: "new-access", RefreshToken: "new-refresh",
				ExpiresIn: 604800, CreatedAt: int(time.Now().Unix()),
			})
		case "/users/me/history", "/users/me/watchlist/movies,shows":
			if r.Header.Get("Authorization") != "Bearer new-access" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`[{"type":"movie","movie":{"title":"Sicario","year":2015}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func traktTestServer(t *testing.T, refreshToken string, refreshes *int32) (*Server, *mockStore) {
	t.Helper()
	ts := fakeTrakt(t, refreshes)
	t.Cleanup(ts.Close)

	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	expired := time.Now().Add(-time.Hour)
	ms.configs["u1"].TraktAccessToken = "old-access"
	ms.configs["u1"].TraktRefreshToken = refreshToken
	ms.configs["u1"].TraktExpiresAt = &expired
	ms.configs["u1"].TraktConnected = true

	client := trakt.NewClient("id", "secret").WithBaseURL(ts.URL)
	srv := NewForTest(&config.Config{BaseURL: "http://localhost:7000"}, ms, nil, client, cache.New(""))
	return srv, ms
}

func TestTraktToken_DeadRefreshDisconnects(t *testing.T) {
	var refreshes int32
	srv, ms := traktTestServer(t, "dead-refresh", &refreshes)
	cfg, _ := ms.GetUserConfig(context.Background(), "u1")

	if items := srv.fetchWatchHistory(context.Background(), cfg); items != nil {
		t.Fatalf("expected no history, got %v", items)
	}
	if cfg.TraktConnected || ms.configs["u1"].TraktConnected || ms.configs["u1"].TraktAccessToken != "" {
		t.Error("a rejected refresh token should clear the Trakt connection")
	}

	// Once disconnected, later requests don't keep hammering Trakt.
	srv.fetchWatchlist(context.Background(), cfg)
	if refreshes != 1 {
		t.Errorf("expected 1 refresh attempt, got %d", refreshes)
	}
}

func TestTraktToken_RefreshesOncePerRequest(t *testing.T) {
	var refreshes int32
	srv, ms := traktTestServer(t, "good-refresh", &refreshes)
	cfg, _ := ms.GetUserConfig(context.Background(), "u1")

	history := srv.fetchWatchHistory(context.Background(), cfg)
	watchlist := srv.fetchWatchlist(context.Background(), cfg)

	if len(history) != 1 || len(watchlist) != 1 {
		t.Fatalf("history=%v watchlist=%v", history, watchlist)
	}
	if refreshes != 1 {
		t.Errorf("watchlist after history should reuse the refreshed token; refreshes = %d", refreshes)
	}
	if got := ms.configs["u1"]; got.TraktAccessToken != "new-access" || got.TraktRefreshToken != "new-refresh" || !got.TraktConnected {
		t.Errorf("refreshed tokens not saved: %+v", got)
	}
}

func TestTraktToken_ConcurrentRequestsRefreshOnce(t *testing.T) {
	var refreshes int32
	srv, ms := traktTestServer(t, "good-refresh", &refreshes)

	// A Stremio home load fires every catalog at once, each with its own
	// stale copy of the config.
	var wg sync.WaitGroup
	results := make([]int, 6)
	for i := range results {
		cfg, _ := ms.GetUserConfig(context.Background(), "u1")
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = len(srv.fetchWatchHistory(context.Background(), cfg))
		}(i)
	}
	wg.Wait()

	if refreshes != 1 {
		t.Errorf("expected exactly 1 refresh, got %d", refreshes)
	}
	for i, n := range results {
		if n != 1 {
			t.Errorf("request %d got %d items", i, n)
		}
	}
	if !ms.configs["u1"].TraktConnected {
		t.Error("connection should survive concurrent refreshes")
	}
}
