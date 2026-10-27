package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// refreshSkew refreshes a little before expiry so a token can't lapse mid-request.
const refreshSkew = 5 * time.Minute

// traktLocks serializes token refreshes per user. Trakt refresh tokens are
// single-use: when the parallel catalog requests from one Stremio load each
// refreshed with the same token, every refresh after the first replayed a
// spent token.
type traktLocks struct {
	m sync.Map // userID -> *sync.Mutex
}

func (l *traktLocks) get(userID string) *sync.Mutex {
	mu, _ := l.m.LoadOrStore(userID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func tokenExpired(cfg *store.UserConfig) bool {
	return cfg.TraktExpiresAt != nil && time.Now().Add(refreshSkew).After(*cfg.TraktExpiresAt)
}

// traktToken returns a usable access token, refreshing it when it has
// expired (or force is set after Trakt rejected it). A rejected refresh token
// can never recover, so the connection is cleared: the config page then shows
// Trakt as disconnected and Because You Watched disappears, instead of every
// request retrying a refresh that will always fail.
func (s *Server) traktToken(ctx context.Context, cfg *store.UserConfig, force bool) (string, bool) {
	if s.traktClient == nil || cfg.TraktAccessToken == "" {
		return "", false
	}
	if !force && !tokenExpired(cfg) {
		return cfg.TraktAccessToken, true
	}

	mu := s.traktLocks.get(cfg.UserID)
	mu.Lock()
	defer mu.Unlock()

	// Another request may have refreshed while this one waited for the lock.
	stale := cfg.TraktAccessToken
	if fresh, err := s.store.GetUserConfig(ctx, cfg.UserID); err == nil {
		cfg.TraktAccessToken = fresh.TraktAccessToken
		cfg.TraktRefreshToken = fresh.TraktRefreshToken
		cfg.TraktExpiresAt = fresh.TraktExpiresAt
		cfg.TraktConnected = fresh.TraktConnected
	}
	if cfg.TraktAccessToken == "" {
		return "", false
	}
	if cfg.TraktAccessToken != stale || (!force && !tokenExpired(cfg)) {
		return cfg.TraktAccessToken, true
	}

	redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
	tokenResp, err := s.traktClient.RefreshToken(ctx, cfg.TraktRefreshToken, redirectURI)
	if err != nil {
		if errors.Is(err, trakt.ErrInvalidGrant) {
			log.Printf("[Trakt] %s: refresh token rejected, disconnecting Trakt: %v", cfg.UserID, err)
			s.disconnectTrakt(ctx, cfg)
		} else {
			log.Printf("[Trakt] %s: token refresh failed: %v", cfg.UserID, err)
		}
		return "", false
	}

	expiresAt := trakt.TokenExpiresAt(tokenResp)
	if err := s.store.SaveTraktTokens(ctx, cfg.UserID, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt); err != nil {
		log.Printf("[Trakt] %s: failed to save refreshed tokens: %v", cfg.UserID, err)
	}
	cfg.TraktAccessToken = tokenResp.AccessToken
	cfg.TraktRefreshToken = tokenResp.RefreshToken
	cfg.TraktExpiresAt = &expiresAt
	return cfg.TraktAccessToken, true
}

func (s *Server) disconnectTrakt(ctx context.Context, cfg *store.UserConfig) {
	if err := s.store.ClearTraktTokens(ctx, cfg.UserID); err != nil {
		log.Printf("[Trakt] %s: failed to clear tokens: %v", cfg.UserID, err)
	}
	cfg.TraktAccessToken, cfg.TraktRefreshToken, cfg.TraktExpiresAt = "", "", nil
	cfg.TraktConnected = false
	s.cache.InvalidatePrefix(cfg.UserID + ":")
}

// fetchTrakt runs a Trakt read with a valid token, refreshing once if Trakt
// rejects the token before its recorded expiry (e.g. revoked and re-issued).
func (s *Server) fetchTrakt(ctx context.Context, cfg *store.UserConfig, what string,
	call func(token string) ([]trakt.WatchedItem, error)) []trakt.WatchedItem {
	return traktCall(s, ctx, cfg, what, call)
}

// traktCall runs one Trakt request with the user's token, refreshing the
// token once on a 401. Errors are logged and give nil.
func traktCall[T any](s *Server, ctx context.Context, cfg *store.UserConfig, what string, call func(token string) ([]T, error)) []T {
	token, ok := s.traktToken(ctx, cfg, false)
	if !ok {
		return nil
	}
	items, err := call(token)
	if errors.Is(err, trakt.ErrUnauthorized) {
		if token, ok = s.traktToken(ctx, cfg, true); !ok {
			return nil
		}
		items, err = call(token)
	}
	if err != nil {
		log.Printf("[Trakt] %s: %s failed: %v", cfg.UserID, what, err)
		return nil
	}
	return items
}

func (s *Server) fetchWatchHistory(ctx context.Context, cfg *store.UserConfig) []trakt.WatchedItem {
	return s.fetchTrakt(ctx, cfg, "watch history", func(token string) ([]trakt.WatchedItem, error) {
		return s.traktClient.GetWatchHistory(ctx, token, 50)
	})
}

func (s *Server) fetchWatchlist(ctx context.Context, cfg *store.UserConfig) []trakt.WatchedItem {
	return s.fetchTrakt(ctx, cfg, "watchlist", func(token string) ([]trakt.WatchedItem, error) {
		return s.traktClient.GetWatchlist(ctx, token, 50)
	})
}

// fetchRatings returns the user's Trakt ratings keyed "movie:<tmdb>" /
// "show:<tmdb>" (cached like the watched list).
func (s *Server) fetchRatings(ctx context.Context, cfg *store.UserConfig) map[string]int {
	if s.traktClient == nil || cfg.TraktAccessToken == "" {
		return nil
	}
	key := cfg.UserID + ":trakt-ratings"
	var cached map[string]int
	if s.cache.Get(key, &cached) {
		return cached
	}
	items := traktCall(s, ctx, cfg, "ratings", func(token string) ([]trakt.Rating, error) {
		return s.traktClient.GetRatings(ctx, token)
	})
	out := make(map[string]int, len(items))
	for _, r := range items {
		out[fmt.Sprintf("%s:%d", r.MediaType, r.TMDBID)] = r.Rating
	}
	if len(out) > 0 {
		s.cache.Set(key, out, watchedTTL)
	}
	return out
}

// watchedTTL is how long a user's full watched list is cached; one Stremio
// load requests every catalog at once and each needs it.
const watchedTTL = time.Hour

// fetchWatched returns everything the user has watched on Trakt (cached).
// The key starts with the user ID so config changes and Trakt reconnects,
// which invalidate that prefix, drop it too.
func (s *Server) fetchWatched(ctx context.Context, cfg *store.UserConfig) []trakt.WatchedItem {
	if s.traktClient == nil || cfg.TraktAccessToken == "" {
		return nil
	}
	key := cfg.UserID + ":trakt-watched-v2" // v2: items carry LastWatchedAt and Plays (Watch Again)
	var cached []trakt.WatchedItem
	if s.cache.Get(key, &cached) {
		return cached
	}
	items := s.fetchTrakt(ctx, cfg, "watched list", func(token string) ([]trakt.WatchedItem, error) {
		return s.traktClient.GetWatched(ctx, token)
	})
	if len(items) > 0 {
		s.cache.Set(key, items, watchedTTL)
	}
	return items
}

// promptHistory picks the titles shown to the AI as taste and exclusions:
// recent plays first, then the most recently watched, up to limit. The full
// watched list is too long for a prompt; the filter enforces all of it.
func promptHistory(history, watched []trakt.WatchedItem, limit int) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range [][]trakt.WatchedItem{history, watched} {
		for _, item := range list {
			if len(out) >= limit {
				return out
			}
			title := fmt.Sprintf("%s (%d)", item.Title, item.Year)
			if !seen[title] {
				seen[title] = true
				out = append(out, title)
			}
		}
	}
	return out
}

func formatTitles(items []trakt.WatchedItem) []string {
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = fmt.Sprintf("%s (%d)", item.Title, item.Year)
	}
	return titles
}
