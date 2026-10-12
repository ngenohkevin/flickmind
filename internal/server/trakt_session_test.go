package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	client := trakt.NewClient("id").WithBaseURL(ts.URL)
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

func TestTraktSignIn_PKCERoundTrip(t *testing.T) {
	var challenge string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		sum := sha256.Sum256([]byte(body["code_verifier"]))
		switch {
		case body["client_secret"] != "":
			t.Error("PKCE sign-in must not send a client secret")
			w.WriteHeader(400)
		case body["code"] != "trakt-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge:
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"invalid code"}`))
		default:
			_ = json.NewEncoder(w).Encode(trakt.TokenResponse{AccessToken: "a1", RefreshToken: "r1", ExpiresIn: 604800, CreatedAt: int(time.Now().Unix())})
		}
	}))
	defer ts.Close()

	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := NewForTest(&config.Config{BaseURL: "http://api.local", FrontendURL: "http://front.local"}, ms, nil,
		trakt.NewClient("cid").WithBaseURL(ts.URL), cache.New(""))

	// 1. Start sign-in: redirect to Trakt with PKCE and a random state.
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/trakt/auth/u1", nil))
	if w.Code != 302 {
		t.Fatalf("auth: %d", w.Code)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	q := loc.Query()
	challenge = q.Get("code_challenge")
	state := q.Get("state")
	if loc.Host != "auth.trakt.tv" || challenge == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize redirect missing PKCE: %s", loc)
	}
	if state == "" || state == "u1" {
		t.Fatalf("state must be a random token, got %q", state)
	}

	// 2. Callback: exchange with the stored verifier, save tokens.
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/trakt/callback?code=trakt-code&state="+state, nil))
	if w.Code != 302 || w.Header().Get("Location") != "http://front.local/configure/u1?trakt=connected" {
		t.Fatalf("callback: %d %s %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if got := ms.configs["u1"]; got.TraktAccessToken != "a1" || got.TraktRefreshToken != "r1" || !got.TraktConnected {
		t.Errorf("tokens not saved: %+v", got)
	}

	// 3. The state is single-use, and unknown states are rejected.
	for _, st := range []string{state, "forged"} {
		w = httptest.NewRecorder()
		srv.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/trakt/callback?code=trakt-code&state="+st, nil))
		if w.Code != 400 {
			t.Errorf("state %q: expected 400, got %d", st, w.Code)
		}
	}
}

func TestTraktSignIn_DeclinedRedirectsWithError(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := NewForTest(&config.Config{BaseURL: "http://api.local", FrontendURL: "http://front.local"}, ms, nil, trakt.NewClient("cid"), cache.New(""))

	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/trakt/auth/u1", nil))
	loc, _ := url.Parse(w.Header().Get("Location"))

	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/trakt/callback?error=access_denied&state="+loc.Query().Get("state"), nil))
	if w.Header().Get("Location") != "http://front.local/configure/u1?trakt=error" {
		t.Errorf("declined sign-in should return to the config page with an error, got %d %s", w.Code, w.Header().Get("Location"))
	}
}
