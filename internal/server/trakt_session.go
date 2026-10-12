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

func formatTitles(items []trakt.WatchedItem) []string {
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = fmt.Sprintf("%s (%d)", item.Title, item.Year)
	}
	return titles
}
