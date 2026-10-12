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
	"github.com/ngenohkevin/flickmind/internal/prefs"
	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/stremio"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// Health

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "service": "flickmind"})
}

// Config API

type configRequest struct {
	// Keys are pointers so an absent field (keep) differs from "" (provider
	// switched off on the config page: remove the key).
	GroqKey              *string  `json:"groqKey"`
	DeepSeekKey          *string  `json:"deepseekKey"`
	GeminiKey            *string  `json:"geminiKey"`
	Genres               []string `json:"genres"`
	ContentTypes         []string `json:"contentTypes"`
	Language             string   `json:"language"`
	Mood                 string   `json:"mood"`
	MinRating            float64  `json:"minRating"`
	YearFrom             int      `json:"yearFrom"`
	YearTo               int      `json:"yearTo"`
	MaxResults           int      `json:"maxResults"`
	RecommendationSource string   `json:"recommendationSource"`
	RefreshInterval      string   `json:"refreshInterval"`
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
		UserID:               userID,
		GroqKey:              mergeKey("", req.GroqKey),
		DeepSeekKey:          mergeKey("", req.DeepSeekKey),
		GeminiKey:            mergeKey("", req.GeminiKey),
		Genres:               req.Genres,
		ContentTypes:         req.ContentTypes,
		Language:             req.Language,
		Mood:                 req.Mood,
		MinRating:            req.MinRating,
		YearFrom:             req.YearFrom,
		YearTo:               req.YearTo,
		MaxResults:           req.MaxResults,
		RecommendationSource: req.RecommendationSource,
		RefreshInterval:      req.RefreshInterval,
	}
	store.NormalizeConfig(cfg)

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
		"userId":               cfg.UserID,
		"groqKey":              maskKey(cfg.GroqKey),
		"deepseekKey":          maskKey(cfg.DeepSeekKey),
		"geminiKey":            maskKey(cfg.GeminiKey),
		"traktConnected":       cfg.TraktConnected,
		"genres":               cfg.Genres,
		"contentTypes":         cfg.ContentTypes,
		"language":             cfg.Language,
		"mood":                 cfg.Mood,
		"minRating":            cfg.MinRating,
		"yearFrom":             cfg.YearFrom,
		"yearTo":               cfg.YearTo,
		"maxResults":           cfg.MaxResults,
		"recommendationSource": cfg.RecommendationSource,
		"refreshInterval":      cfg.RefreshInterval,
		"hasTrakt":             s.cfg.TraktClientID != "",
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

	existing.GroqKey = mergeKey(existing.GroqKey, req.GroqKey)
	existing.DeepSeekKey = mergeKey(existing.DeepSeekKey, req.DeepSeekKey)
	existing.GeminiKey = mergeKey(existing.GeminiKey, req.GeminiKey)

	existing.Genres = req.Genres
	existing.ContentTypes = req.ContentTypes
	existing.Language = req.Language
	existing.Mood = req.Mood
	existing.MinRating = req.MinRating
	existing.YearFrom = req.YearFrom
	existing.YearTo = req.YearTo
	if req.MaxResults > 0 {
		existing.MaxResults = req.MaxResults
	}
	if req.RecommendationSource != "" {
		existing.RecommendationSource = req.RecommendationSource
	}
	if req.RefreshInterval != "" {
		existing.RefreshInterval = req.RefreshInterval
	}
	store.NormalizeConfig(existing)

	if err := s.store.SaveUserConfig(c.Request.Context(), existing); err != nil {
		log.Printf("[ERROR] Update config: %v", err)
		c.JSON(500, gin.H{"error": "failed to update config"})
		return
	}

	s.cache.InvalidatePrefix(userID + ":")
	c.JSON(200, gin.H{"status": "ok"})
}

// Trakt OAuth

// traktSignIn is what the OAuth callback needs to finish a sign-in, keyed by
// the random state sent to Trakt.
type traktSignIn struct {
	UserID       string `json:"userId"`
	CodeVerifier string `json:"codeVerifier"`
}

const traktSignInTTL = 15 * time.Minute

func traktSignInKey(state string) string { return "trakt-signin:" + state }

func (s *Server) handleTraktAuth(c *gin.Context) {
	userID := c.Param("userId")
	if s.traktClient == nil {
		c.JSON(400, gin.H{"error": "Trakt not configured"})
		return
	}
	if exists, err := s.store.UserExists(c.Request.Context(), userID); err != nil || !exists {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}

	verifier, challenge, err := trakt.NewPKCE()
	if err != nil {
		c.String(500, "Failed to start Trakt sign-in")
		return
	}
	// The state is a random one-time token rather than the user ID, so the
	// callback can tell a sign-in it started from a forged one.
	state := uuid.New().String()
	s.cache.Set(traktSignInKey(state), traktSignIn{UserID: userID, CodeVerifier: verifier}, traktSignInTTL)

	redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
	c.Redirect(302, s.traktClient.AuthorizeURL(redirectURI, state, challenge))
}

func (s *Server) handleTraktCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if s.traktClient == nil {
		c.String(500, "Trakt not configured")
		return
	}

	var signIn traktSignIn
	if state == "" || !s.cache.Get(traktSignInKey(state), &signIn) {
		c.String(400, "This Trakt sign-in link has expired or was already used. Go back to the FlickMind config page and connect Trakt again.")
		return
	}
	s.cache.InvalidatePrefix(traktSignInKey(state))
	configURL := fmt.Sprintf("%s/configure/%s", s.cfg.FrontendURL, signIn.UserID)

	if code == "" {
		// The user declined on Trakt, or Trakt returned an error.
		log.Printf("[Trakt] %s: sign-in not completed: %s %s", signIn.UserID, c.Query("error"), c.Query("error_description"))
		c.Redirect(302, configURL+"?trakt=error")
		return
	}

	redirectURI := s.cfg.BaseURL + "/api/trakt/callback"
	tokenResp, err := s.traktClient.ExchangeCode(c.Request.Context(), code, redirectURI, signIn.CodeVerifier)
	if err != nil {
		log.Printf("[ERROR] Trakt token exchange: %v", err)
		c.Redirect(302, configURL+"?trakt=error")
		return
	}

	expiresAt := trakt.TokenExpiresAt(tokenResp)
	if err := s.store.SaveTraktTokens(c.Request.Context(), signIn.UserID, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt); err != nil {
		log.Printf("[ERROR] Save trakt tokens: %v", err)
		c.Redirect(302, configURL+"?trakt=error")
		return
	}

	s.cache.InvalidatePrefix(signIn.UserID + ":")
	// Redirect back to the Next.js frontend config page
	c.Redirect(302, configURL+"?trakt=connected")
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

// Stremio Configure Redirect

func (s *Server) handleConfigureRedirect(c *gin.Context) {
	userID := c.Param("userId")
	c.Redirect(302, fmt.Sprintf("%s/configure/%s", s.cfg.FrontendURL, userID))
}

// Stremio Manifest

func (s *Server) handleManifest(c *gin.Context) {
	userID := c.Param("userId")

	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		cfg = &store.UserConfig{UserID: userID}
	}
	store.NormalizeConfig(cfg)

	manifest := stremio.BuildManifest(s.cfg.BaseURL, s.cfg.FrontendURL, userID, cfg)
	c.JSON(200, manifest)
}

// Stremio Catalog

// cacheVersion is part of every catalog cache key; bump it when the pipeline
// changes so catalogs built by the old logic aren't served after a deploy.
const cacheVersion = "v2"

func (s *Server) handleCatalog(c *gin.Context) {
	userID := c.Param("userId")
	mediaType := c.Param("type")
	catalogID := c.Param("id")

	catalogID = strings.TrimSuffix(catalogID, ".json")

	cacheKey := fmt.Sprintf("%s:%s:%s:%s", userID, cacheVersion, catalogID, mediaType)

	var cached stremio.CatalogResponse
	if s.cache.Get(cacheKey, &cached) {
		c.JSON(200, cached)
		return
	}

	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		c.JSON(200, stremio.CatalogResponse{Metas: []stremio.Meta{}})
		return
	}
	store.NormalizeConfig(cfg)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 110*time.Second)
	defer cancel()

	// Limit concurrent AI calls per user to avoid provider rate limits.
	// Placed after cache check so cached responses return immediately.
	s.aiSem.acquire(userID)
	defer s.aiSem.release(userID)

	var metas []stremio.Meta

	switch catalogID {
	case "flickmind-ai-picks":
		metas = s.getAIPicks(ctx, cfg, mediaType)
	case "flickmind-hidden-gems":
		metas = s.getHiddenGems(ctx, cfg, mediaType)
	case "flickmind-because-you-watched":
		metas = s.getBecauseYouWatched(ctx, cfg, mediaType)
	case "flickmind-anime":
		metas = s.getFocusedPicks(ctx, cfg, ai.KindAnime)
	case "flickmind-documentary":
		metas = s.getFocusedPicks(ctx, cfg, ai.KindDocumentary)
	default:
		c.JSON(200, stremio.CatalogResponse{Metas: []stremio.Meta{}})
		return
	}

	resp := stremio.CatalogResponse{Metas: metas}

	if len(metas) > 0 {
		ttl := store.ParseRefreshInterval(cfg.RefreshInterval)
		if catalogID == "flickmind-hidden-gems" && ttl < 6*time.Hour {
			ttl = 6 * time.Hour
		}
		s.cache.Set(cacheKey, resp, ttl)
	}

	c.JSON(200, resp)
}

// catalogRequest describes one catalog build.
type catalogRequest struct {
	tag       string
	kind      ai.PromptKind
	mediaType string // "movie" or "series"; "" for the mixed anime/documentary rows
	input     ai.PromptInput
	blocked   []trakt.WatchedItem // titles never to show: watched, and the watchlist in watchlist mode
	// discover fills a shortfall (or an AI failure) from TMDB discover.
	// Because You Watched skips it: popular titles aren't "similar to X".
	discover bool
}

func (s *Server) getAIPicks(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	history := s.fetchWatchHistory(ctx, cfg)
	req := catalogRequest{tag: "AI Picks", kind: ai.KindPicks, mediaType: mediaType, blocked: history, discover: true}
	req.input.WatchHistory = formatTitles(history)

	if cfg.RecommendationSource == "watchlist" {
		if watchlist := s.fetchWatchlist(ctx, cfg); len(watchlist) > 0 {
			req.tag, req.kind = "Watchlist Picks", ai.KindWatchlist
			req.input.Watchlist = formatTitles(watchlist)
			req.blocked = append(req.blocked, watchlist...)
		} else {
			log.Printf("[Watchlist Picks] %s: no watchlist available (Trakt connected: %v), using preferences", cfg.UserID, cfg.TraktConnected)
		}
	}
	return s.buildCatalog(ctx, cfg, req)
}

func (s *Server) getHiddenGems(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	history := s.fetchWatchHistory(ctx, cfg)
	req := catalogRequest{tag: "Hidden Gems", kind: ai.KindGems, mediaType: mediaType, blocked: history, discover: true}
	req.input.WatchHistory = formatTitles(history)
	return s.buildCatalog(ctx, cfg, req)
}

func (s *Server) getBecauseYouWatched(ctx context.Context, cfg *store.UserConfig, mediaType string) []stremio.Meta {
	if !cfg.TraktConnected || s.traktClient == nil {
		return nil
	}

	history := s.fetchWatchHistory(ctx, cfg)
	if len(history) == 0 {
		return nil
	}

	req := catalogRequest{tag: "Because You Watched", kind: ai.KindBecauseYouWatched, mediaType: mediaType, blocked: history}
	req.input.SeedTitle = formatTitles(history[:1])[0]
	return s.buildCatalog(ctx, cfg, req)
}

func (s *Server) getFocusedPicks(ctx context.Context, cfg *store.UserConfig, kind ai.PromptKind) []stremio.Meta {
	tag := "Anime Picks"
	if kind == ai.KindDocumentary {
		tag = "Documentary Picks"
	}
	history := s.fetchWatchHistory(ctx, cfg)
	req := catalogRequest{tag: tag, kind: kind, blocked: history, discover: true}
	req.input.WatchHistory = formatTitles(history)
	return s.buildCatalog(ctx, cfg, req)
}

// buildCatalog produces up to cfg.MaxResults titles that all honor the
// user's settings: AI suggestions first (with a second round to cover what the
// filter dropped), then TMDB discover for any remaining shortfall.
func (s *Server) buildCatalog(ctx context.Context, cfg *store.UserConfig, req catalogRequest) []stremio.Meta {
	rules := prefs.For(cfg, ai.RulesKind(req.kind))
	filter := newResultFilter(rules, allowedTypes(cfg, req.mediaType), req.blocked)
	want := cfg.MaxResults

	var kept []tmdb.SearchResult
	if providers := s.buildProviderChain(cfg); len(providers) > 0 {
		kept = s.aiRounds(ctx, cfg, req, providers, filter, want)
	}

	if len(kept) < want && req.discover {
		extra := s.discoverFill(ctx, req, rules, filter)
		if need := want - len(kept); len(extra) > need {
			extra = extra[:need]
		}
		if len(extra) > 0 {
			s.tmdbClient.AttachIMDBIds(ctx, extra)
			log.Printf("[%s %s] filled %d from TMDB discover", req.tag, req.mediaType, len(extra))
			kept = append(kept, extra...)
		}
	}

	return limitResults(filterByType(kept, req.mediaType), want)
}

// aiRounds asks the AI for titles, enriches them via TMDB and keeps those
// that pass the filter. A second, smaller round tops up whatever the filter
// dropped, telling the AI what it already suggested.
func (s *Server) aiRounds(ctx context.Context, cfg *store.UserConfig, req catalogRequest,
	providers []ai.ProviderEntry, filter *resultFilter, want int) []tmdb.SearchResult {
	var kept []tmdb.SearchResult
	var suggested []string

	for round := 1; round <= 2 && len(kept) < want; round++ {
		if round > 1 {
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 40*time.Second {
				break
			}
		}

		in := roundInput(req, want-len(kept), suggested)
		result, err := ai.GetRecommendations(ctx, providers, ai.BuildPrompt(req.kind, cfg, in))
		if err != nil {
			log.Printf("[%s %s] round %d: AI failed: %v", req.tag, req.mediaType, round, err)
			break
		}
		for _, r := range result.Recommendations {
			suggested = append(suggested, fmt.Sprintf("%s (%d)", r.Title, r.Year))
		}

		enriched := s.tmdbClient.EnrichRecommendations(ctx, result.Recommendations)
		accepted, summary := filter.apply(enriched)
		kept = append(kept, accepted...)
		log.Printf("[%s %s] round %d via %s: %d suggested, %d found on TMDB, %d kept (%s)",
			req.tag, req.mediaType, round, result.ProviderName, len(result.Recommendations), len(enriched), len(accepted), summary)

		// Most suggestions missing from TMDB means the model is inventing
		// titles; let the next provider take the top-up round.
		if len(enriched)*2 < len(result.Recommendations) {
			providers = demoteProvider(providers, result.ProviderName)
			log.Printf("[%s %s] %s invented most titles; trying the next provider first", req.tag, req.mediaType, result.ProviderName)
		}
	}
	return kept
}

// roundInput builds the prompt input for one AI round: the catalog's own
// context plus its row type, the count still needed and what was already suggested.
func roundInput(req catalogRequest, need int, suggested []string) ai.PromptInput {
	in := req.input
	in.MediaType = req.mediaType
	in.Count = ai.OverRequest(need)
	in.Exclude = suggested
	return in
}

// demoteProvider moves the named provider to the end of the chain.
func demoteProvider(providers []ai.ProviderEntry, name string) []ai.ProviderEntry {
	var front, back []ai.ProviderEntry
	for _, p := range providers {
		if p.Provider.Name() == name {
			back = append(back, p)
		} else {
			front = append(front, p)
		}
	}
	return append(front, back...)
}

// discoverFill pulls popular TMDB titles that pass the same filter.
func (s *Server) discoverFill(ctx context.Context, req catalogRequest, rules prefs.Rules, filter *resultFilter) []tmdb.SearchResult {
	o := tmdb.DiscoverOpts{
		MediaType: req.mediaType,
		Genres:    rules.Selected,
		MinRating: rules.MinRating,
		YearFrom:  rules.YearFrom,
		YearTo:    rules.YearTo,
		Language:  strings.Join(rules.Languages, "|"),
	}
	switch req.kind {
	case ai.KindGems:
		// Highly rated but little-voted, and old enough that the vote count
		// means something: recent blockbusters sit under any vote cap for
		// months. TV movies (specials) are skipped.
		o.MinRating = max(o.MinRating, 7.0)
		o.MinVotes = 300
		o.MaxVotes = 3000
		o.SortBy = "vote_average.desc"
		o.ReleasedBefore = time.Now().AddDate(-3, 0, 0)
		o.SkipTVMovies = true
	case ai.KindAnime, ai.KindDocumentary:
		o.Genres = []string{rules.Required}
	}

	accepted, _ := filter.apply(s.tmdbClient.DiscoverFallback(ctx, o))
	return accepted
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

func filterByType(results []tmdb.SearchResult, mediaType string) []stremio.Meta {
	var metas []stremio.Meta
	for _, r := range results {
		stremioType := "movie"
		if r.MediaType == "tv" {
			stremioType = "series"
		}
		if stremioType == mediaType || mediaType == "" {
			metas = append(metas, stremio.TMDBResultToMeta(r, r.Reason))
		}
	}
	if metas == nil {
		metas = []stremio.Meta{}
	}
	return metas
}

func limitResults(metas []stremio.Meta, max int) []stremio.Meta {
	if max > 0 && len(metas) > max {
		return metas[:max]
	}
	return metas
}

// mergeKey applies a key from the config page: absent or masked ("••••abcd",
// the value the page shows for a saved key) keeps the current key, "" removes
// it (provider switched off), anything else replaces it.
func mergeKey(current string, incoming *string) string {
	if incoming == nil || strings.HasPrefix(*incoming, "••••") {
		return current
	}
	return *incoming
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
