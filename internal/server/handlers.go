package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ngenohkevin/flickmind/internal/ai"
	"github.com/ngenohkevin/flickmind/internal/stremio"
	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// Health

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "service": "flickmind"})
}

// Config API

type configRequest struct {
	GroqKey      string   `json:"groqKey"`
	DeepSeekKey  string   `json:"deepseekKey"`
	GeminiKey    string   `json:"geminiKey"`
	Genres       []string `json:"genres"`
	ContentTypes []string `json:"contentTypes"`
	Language     string   `json:"language"`
	Mood         string   `json:"mood"`
	MinRating    float64  `json:"minRating"`
}

func (s *Server) handleCreateConfig(c *gin.Context) {
	var req configRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	userID := uuid.New().String()[:8]

	if err := s.store.CreateUser(c.Request.Context(), userID); err != nil {
		log.Printf("[ERROR] Create user: %v", err)
		c.JSON(500, gin.H{"error": "failed to create user"})
		return
	}

	cfg := &store.UserConfig{
		UserID:       userID,
		GroqKey:      req.GroqKey,
		DeepSeekKey:  req.DeepSeekKey,
		GeminiKey:    req.GeminiKey,
		Genres:       req.Genres,
		ContentTypes: req.ContentTypes,
		Language:     req.Language,
		Mood:         req.Mood,
		MinRating:    req.MinRating,
	}

	if err := s.store.SaveUserConfig(c.Request.Context(), cfg); err != nil {
		log.Printf("[ERROR] Save config: %v", err)
		c.JSON(500, gin.H{"error": "failed to save config"})
		return
	}

	c.JSON(200, gin.H{
		"userId":    userID,
		"addonURL":  fmt.Sprintf("%s/%s/manifest.json", s.cfg.BaseURL, userID),
		"configURL": fmt.Sprintf("%s/configure/%s", s.cfg.FrontendURL, userID),
	})
}

func (s *Server) handleGetConfig(c *gin.Context) {
	userID := c.Param("userId")

	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}

	c.JSON(200, gin.H{
		"userId":         cfg.UserID,
		"groqKey":        maskKey(cfg.GroqKey),
		"deepseekKey":    maskKey(cfg.DeepSeekKey),
		"geminiKey":      maskKey(cfg.GeminiKey),
		"traktConnected": cfg.TraktConnected,
		"genres":         cfg.Genres,
		"contentTypes":   cfg.ContentTypes,
		"language":       cfg.Language,
		"mood":           cfg.Mood,
		"minRating":      cfg.MinRating,
		"hasTrakt":       s.cfg.TraktClientID != "",
	})
}

func (s *Server) handleUpdateConfig(c *gin.Context) {
	userID := c.Param("userId")

	exists, err := s.store.UserExists(c.Request.Context(), userID)
	if err != nil || !exists {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}

	var req configRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	existing, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to read config"})
		return
	}

	// Only update keys if new value provided (not masked)
	if req.GroqKey != "" && !strings.HasPrefix(req.GroqKey, "••••") {
		existing.GroqKey = req.GroqKey
	}
	if req.DeepSeekKey != "" && !strings.HasPrefix(req.DeepSeekKey, "••••") {
		existing.DeepSeekKey = req.DeepSeekKey
	}
	if req.GeminiKey != "" && !strings.HasPrefix(req.GeminiKey, "••••") {
		existing.GeminiKey = req.GeminiKey
	}

	existing.Genres = req.Genres
	existing.ContentTypes = req.ContentTypes
	existing.Language = req.Language
	existing.Mood = req.Mood
	existing.MinRating = req.MinRating

	if err := s.store.SaveUserConfig(c.Request.Context(), existing); err != nil {
		log.Printf("[ERROR] Update config: %v", err)
		c.JSON(500, gin.H{"error": "failed to update config"})
		return
	}

	s.cache.InvalidatePrefix(userID + ":")
	c.JSON(200, gin.H{"status": "ok"})
}

// Trakt OAuth

func (s *Server) handleTraktAuth(c *gin.Context) {
	userID := c.Param("userId")
	if s.traktClient == nil {
		c.JSON(400, gin.H{"error": "Trakt not configured"})
		return
	}

	redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
	authURL := s.traktClient.AuthorizeURL(redirectURI, userID)
	c.Redirect(302, authURL)
}

func (s *Server) handleTraktCallback(c *gin.Context) {
	code := c.Query("code")
	userID := c.Query("state")

	if code == "" || userID == "" {
		c.String(400, "Missing code or state")
		return
	}

	if s.traktClient == nil {
		c.String(500, "Trakt not configured")
		return
	}

	redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
	tokenResp, err := s.traktClient.ExchangeCode(c.Request.Context(), code, redirectURI)
	if err != nil {
		log.Printf("[ERROR] Trakt token exchange: %v", err)
		c.String(500, "Failed to connect Trakt")
		return
	}

	expiresAt := trakt.TokenExpiresAt(tokenResp)
	if err := s.store.SaveTraktTokens(c.Request.Context(), userID, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt); err != nil {
		log.Printf("[ERROR] Save trakt tokens: %v", err)
		c.String(500, "Failed to save Trakt tokens")
		return
	}

	s.cache.InvalidatePrefix(userID + ":")
	// Redirect back to the Next.js frontend config page
	c.Redirect(302, fmt.Sprintf("%s/configure/%s?trakt=connected", s.cfg.FrontendURL, userID))
}

func (s *Server) handleTraktDisconnect(c *gin.Context) {
	userID := c.Param("userId")
	if err := s.store.ClearTraktTokens(c.Request.Context(), userID); err != nil {
		c.JSON(500, gin.H{"error": "failed to disconnect"})
		return
	}
	s.cache.InvalidatePrefix(userID + ":")
	c.JSON(200, gin.H{"status": "ok"})
}

// Stremio Manifest

func (s *Server) handleManifest(c *gin.Context) {
	userID := c.Param("userId")

	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		cfg = &store.UserConfig{UserID: userID}
	}

	manifest := stremio.BuildManifest(s.cfg.BaseURL, s.cfg.FrontendURL, userID, cfg)
	c.JSON(200, manifest)
}

// Stremio Catalog

func (s *Server) handleCatalog(c *gin.Context) {
	userID := c.Param("userId")
	mediaType := c.Param("type")
	catalogID := c.Param("id")

	log.Printf("[Catalog] Raw params: userId=%q type=%q id=%q", userID, mediaType, catalogID)
	catalogID = strings.TrimSuffix(catalogID, ".json")
	log.Printf("[Catalog] After trim: catalogID=%q", catalogID)

	cacheKey := fmt.Sprintf("%s:%s:%s", userID, catalogID, mediaType)

	var cached stremio.CatalogResponse
	if s.cache.Get(cacheKey, &cached) {
		log.Printf("[Catalog] Cache HIT for %s (%d metas)", cacheKey, len(cached.Metas))
		c.JSON(200, cached)
		return
	}
	log.Printf("[Catalog] Cache MISS for %s, fetching...", cacheKey)

	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		log.Printf("[Catalog] User config not found: %s", userID)
		c.JSON(200, stremio.CatalogResponse{Metas: []stremio.Meta{}})
		return
	}
	log.Printf("[Catalog] User %s has %d providers configured", userID, len(s.buildProviderChain(cfg)))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	var metas []stremio.Meta

	switch catalogID {
	case "flickmind-ai-picks":
		metas = s.getAIPicks(ctx, cfg, mediaType)
	case "flickmind-hidden-gems":
		metas = s.getHiddenGems(ctx, cfg, mediaType)
	case "flickmind-because-you-watched":
		metas = s.getBecauseYouWatched(ctx, cfg, mediaType)
	default:
		c.JSON(200, stremio.CatalogResponse{Metas: []stremio.Meta{}})
		return
	}

	log.Printf("[Catalog] Got %d metas for %s", len(metas), cacheKey)

	resp := stremio.CatalogResponse{Metas: metas}

	ttl := 2 * time.Hour
	if catalogID == "flickmind-hidden-gems" {
		ttl = 6 * time.Hour
	}
	s.cache.Set(cacheKey, resp, ttl)

	c.JSON(200, resp)
}

func (s *Server) getAIPicks(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	var watchHistory []string
	if cfg.TraktConnected && s.traktClient != nil {
		watchHistory = s.fetchWatchHistory(ctx, cfg)
	}

	prompt := ai.BuildAIPicksPrompt(cfg, watchHistory)
	providers := s.buildProviderChain(cfg)

	if len(providers) > 0 {
		result, err := ai.GetRecommendations(ctx, providers, prompt)
		if err == nil {
			enriched := s.tmdbClient.EnrichRecommendations(ctx, result.Recommendations)
			return filterByType(enriched, mediaType)
		}
		log.Printf("[AI Picks] AI failed: %v, falling back to TMDB", err)
	}

	results := s.tmdbClient.DiscoverFallback(ctx, mediaType, cfg.Genres, cfg.MinRating)
	return filterByType(results, mediaType)
}

func (s *Server) getHiddenGems(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	var watchHistory []string
	if cfg.TraktConnected && s.traktClient != nil {
		watchHistory = s.fetchWatchHistory(ctx, cfg)
	}

	prompt := ai.BuildHiddenGemsPrompt(cfg, watchHistory)
	providers := s.buildProviderChain(cfg)

	if len(providers) > 0 {
		result, err := ai.GetRecommendations(ctx, providers, prompt)
		if err == nil {
			enriched := s.tmdbClient.EnrichRecommendations(ctx, result.Recommendations)
			return filterByType(enriched, mediaType)
		}
		log.Printf("[Hidden Gems] AI failed: %v, falling back to TMDB", err)
	}

	results := s.tmdbClient.DiscoverFallback(ctx, mediaType, cfg.Genres, 7.0)
	return filterByType(results, mediaType)
}

func (s *Server) getBecauseYouWatched(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	if !cfg.TraktConnected || s.traktClient == nil {
		return nil
	}

	watchHistory := s.fetchWatchHistory(ctx, cfg)
	if len(watchHistory) == 0 {
		return nil
	}

	prompt := ai.BuildBecauseYouWatchedPrompt(watchHistory[0], cfg)
	providers := s.buildProviderChain(cfg)

	if len(providers) > 0 {
		result, err := ai.GetRecommendations(ctx, providers, prompt)
		if err == nil {
			enriched := s.tmdbClient.EnrichRecommendations(ctx, result.Recommendations)
			return filterByType(enriched, mediaType)
		}
		log.Printf("[Because You Watched] AI failed: %v", err)
	}

	return nil
}

func (s *Server) buildProviderChain(cfg *store.UserConfig) []ai.ProviderEntry {
	var chain []ai.ProviderEntry

	if cfg.GroqKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.GroqProvider{}, APIKey: cfg.GroqKey})
	}
	if cfg.DeepSeekKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.DeepSeekProvider{}, APIKey: cfg.DeepSeekKey})
	}
	if cfg.GeminiKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.GeminiProvider{}, APIKey: cfg.GeminiKey})
	}

	return chain
}

func (s *Server) fetchWatchHistory(ctx context.Context, cfg *store.UserConfig) []string {
	if s.traktClient == nil || cfg.TraktAccessToken == "" {
		return nil
	}

	if cfg.TraktExpiresAt != nil && time.Now().After(*cfg.TraktExpiresAt) {
		redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
		tokenResp, err := s.traktClient.RefreshToken(ctx, cfg.TraktRefreshToken, redirectURI)
		if err != nil {
			log.Printf("[Trakt] Token refresh failed: %v", err)
			return nil
		}
		expiresAt := trakt.TokenExpiresAt(tokenResp)
		s.store.SaveTraktTokens(ctx, cfg.UserID, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt)
		cfg.TraktAccessToken = tokenResp.AccessToken
	}

	items, err := s.traktClient.GetWatchHistory(ctx, cfg.TraktAccessToken, 20)
	if err != nil {
		log.Printf("[Trakt] Watch history failed: %v", err)
		return nil
	}

	var titles []string
	for _, item := range items {
		titles = append(titles, fmt.Sprintf("%s (%d)", item.Title, item.Year))
	}
	return titles
}

func filterByType(results []tmdb.SearchResult, mediaType string) []stremio.Meta {
	var metas []stremio.Meta
	for _, r := range results {
		stremioType := "movie"
		if r.MediaType == "tv" {
			stremioType = "series"
		}
		if stremioType == mediaType || mediaType == "" {
			metas = append(metas, stremio.TMDBResultToMeta(r, ""))
		}
	}
	if metas == nil {
		metas = []stremio.Meta{}
	}
	return metas
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}
