package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

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
// v4: rows skip each other's titles and are cached with their TMDB keys.
const cacheVersion = "v4"

const (
	// lastTTL keeps a row's last list to serve while its replacement builds.
	lastTTL = 7 * 24 * time.Hour
	// retryTTL holds the last list as fresh after a build that produced
	// nothing (providers down), so it isn't retried on every request.
	retryTTL = 15 * time.Minute
	// buildTimeout bounds one row build.
	buildTimeout = 110 * time.Second
)

// knownCatalogs are the row IDs the manifest can list.
var knownCatalogs = map[string]bool{
	"flickmind-ai-picks":            true,
	"flickmind-hidden-gems":         true,
	"flickmind-because-you-watched": true,
	"flickmind-anime":               true,
	"flickmind-documentary":         true,
	"flickmind-watch-again":         true,
}

// catalogEntry is a built row as cached: the metas served to the app and the
// TMDB keys ("movie:123") of the same titles, which the other rows skip.
type catalogEntry struct {
	Metas []stremio.Meta `json:"metas"`
	Keys  []string       `json:"keys"`
}

func (e catalogEntry) response() stremio.CatalogResponse {
	if e.Metas == nil {
		return stremio.CatalogResponse{Metas: []stremio.Meta{}}
	}
	return stremio.CatalogResponse{Metas: e.Metas}
}

// rowKey is a row's cache key. It carries a fingerprint of the settings, so
// a row built with old settings is never served once the config page saves
// new ones, including one that was still building at the time.
func rowKey(cfg *store.UserConfig, catalogID, mediaType string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", cfg.UserID, cacheVersion, settingsFingerprint(cfg), catalogID, mediaType)
}

func lastKey(rowKey string) string { return rowKey + ":last" }

// settingsFingerprint hashes everything that changes what a row contains.
func settingsFingerprint(cfg *store.UserConfig) string {
	b, _ := json.Marshal([]any{
		cfg.Genres, cfg.ContentTypes, cfg.Language, cfg.Mood, cfg.MinRating,
		cfg.YearFrom, cfg.YearTo, cfg.MaxResults, cfg.RecommendationSource, cfg.TraktConnected,
		cfg.GroqKey != "", cfg.DeepSeekKey != "", cfg.GeminiKey != "",
	})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}

// handleCatalog answers from the cache. When a row has expired it answers
// with the row's last list and rebuilds it in the background: a rebuild
// takes 10-50 s, longer than Stremio and Nuvio wait, so they used to give up
// (and the build was thrown away with the request). Only a row that was
// never built makes the app wait.
func (s *Server) handleCatalog(c *gin.Context) {
	userID := c.Param("userId")
	mediaType := c.Param("type")
	catalogID := strings.TrimSuffix(c.Param("id"), ".json")

	if !knownCatalogs[catalogID] {
		c.JSON(200, catalogEntry{}.response())
		return
	}
	cfg, err := s.store.GetUserConfig(c.Request.Context(), userID)
	if err != nil {
		c.JSON(200, catalogEntry{}.response())
		return
	}
	store.NormalizeConfig(cfg)

	key := rowKey(cfg, catalogID, mediaType)
	var entry catalogEntry
	if s.cache.Get(key, &entry) {
		c.JSON(200, entry.response())
		return
	}

	built := s.rebuildRow(cfg, catalogID, mediaType)
	if s.cache.Get(lastKey(key), &entry) && len(entry.Metas) > 0 {
		c.JSON(200, entry.response())
		return
	}
	select {
	case res := <-built:
		entry, _ = res.Val.(catalogEntry)
		c.JSON(200, entry.response())
	case <-c.Request.Context().Done():
	}
}

// rebuildRow builds a row at most once at a time, detached from the request
// that asked for it, and caches the result.
func (s *Server) rebuildRow(cfg *store.UserConfig, catalogID, mediaType string) <-chan singleflight.Result {
	key := rowKey(cfg, catalogID, mediaType)
	return s.rowFlights.DoChan(key, func() (any, error) {
		// One row per user at a time, saved before the slot is given up so
		// the next row sees this row's new list.
		s.aiSem.acquire(cfg.UserID)
		defer s.aiSem.release(cfg.UserID)

		entry := s.safeBuild(cfg, catalogID, mediaType)
		s.storeRow(cfg, key, catalogID, entry)
		return entry, nil
	})
}

// safeBuild runs the row builder with its own deadline, which starts once
// the row has its slot, so the last of seven rows doesn't spend its budget
// waiting. A panic must not take the server down: the build runs outside
// the request, where gin's recovery doesn't reach.
func (s *Server) safeBuild(cfg *store.UserConfig, catalogID, mediaType string) (entry catalogEntry) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ERROR] building %s %s for %s: %v", catalogID, mediaType, cfg.UserID, r)
			entry = catalogEntry{}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	return s.rowBuilder(ctx, cfg, catalogID, mediaType)
}

func (s *Server) storeRow(cfg *store.UserConfig, key, catalogID string, entry catalogEntry) {
	if len(entry.Metas) > 0 {
		ttl := store.ParseRefreshInterval(cfg.RefreshInterval)
		if catalogID == "flickmind-hidden-gems" && ttl < 6*time.Hour {
			ttl = 6 * time.Hour
		}
		if catalogID == "flickmind-watch-again" {
			ttl = 24 * time.Hour // its order changes once a day
		}
		s.cache.Set(key, entry, ttl)
		s.cache.Set(lastKey(key), entry, lastTTL)
		return
	}
	var last catalogEntry
	if s.cache.Get(lastKey(key), &last) && len(last.Metas) > 0 {
		s.cache.Set(key, last, retryTTL)
	}
}

// buildRow builds one row, skipping titles the user's other rows show.
func (s *Server) buildRow(ctx context.Context, cfg *store.UserConfig, catalogID, mediaType string) catalogEntry {
	hidden := s.otherRowKeys(cfg, catalogID, mediaType)
	switch catalogID {
	case "flickmind-ai-picks":
		return s.getAIPicks(ctx, cfg, mediaType, hidden)
	case "flickmind-hidden-gems":
		return s.getHiddenGems(ctx, cfg, mediaType, hidden)
	case "flickmind-because-you-watched":
		return s.getBecauseYouWatched(ctx, cfg, mediaType, hidden)
	case "flickmind-anime":
		return s.getFocusedPicks(ctx, cfg, ai.KindAnime, hidden)
	case "flickmind-documentary":
		return s.getFocusedPicks(ctx, cfg, ai.KindDocumentary, hidden)
	case "flickmind-watch-again":
		return s.getWatchAgain(ctx, cfg, mediaType) // watched titles: no overlap with the other rows
	}
	return catalogEntry{}
}

// otherRowKeys returns the titles the user's other rows show (their last
// lists). Rows were built independently and repeated each other: Blue Ruin,
// Cold in July and Coherence sat in AI Picks, Because You Watched and Hidden
// Gems at once.
func (s *Server) otherRowKeys(cfg *store.UserConfig, catalogID, mediaType string) []string {
	var keys []string
	for _, cat := range stremio.BuildManifest(s.cfg.BaseURL, s.cfg.FrontendURL, cfg.UserID, cfg).Catalogs {
		if cat.ID == catalogID && cat.Type == mediaType {
			continue
		}
		var e catalogEntry
		if s.cache.Get(lastKey(rowKey(cfg, cat.ID, cat.Type)), &e) {
			keys = append(keys, e.Keys...)
		}
	}
	return keys
}

// catalogRequest describes one catalog build.
type catalogRequest struct {
	tag       string
	kind      ai.PromptKind
	mediaType string // "movie" or "series"; "" for the mixed anime/documentary rows
	input     ai.PromptInput
	blocked   []trakt.WatchedItem // titles never to show: watched, and the watchlist in watchlist mode
	hidden    []string            // TMDB keys the user's other rows show
	// discover fills a shortfall (or an AI failure) from TMDB discover.
	// Because You Watched skips it: popular titles aren't "similar to X".
	discover bool
}

// traktContext loads what the user has seen: recent plays (for the
// Because You Watched seed) and the full watched list (for exclusion).
type traktContext struct {
	history []trakt.WatchedItem
	watched []trakt.WatchedItem
}

func (s *Server) loadTrakt(ctx context.Context, cfg *store.UserConfig) traktContext {
	tc := traktContext{history: s.fetchWatchHistory(ctx, cfg)}
	tc.watched = s.fetchWatched(ctx, cfg)
	return tc
}

// apply fills a request's exclusions and prompt context from Trakt.
func (tc traktContext) apply(req *catalogRequest) {
	req.blocked = append(append(req.blocked, tc.history...), tc.watched...)
	req.input.WatchHistory = promptHistory(tc.history, tc.watched, promptHistoryLimit)
	req.input.WatchedCount = len(tc.watched)
}

// promptHistoryLimit caps the watched titles listed in a prompt.
const promptHistoryLimit = 60

func (s *Server) getAIPicks(ctx context.Context, cfg *store.UserConfig, mediaType string, hidden []string) catalogEntry {
	tc := s.loadTrakt(ctx, cfg)
	req := catalogRequest{tag: "AI Picks", kind: ai.KindPicks, mediaType: mediaType, hidden: hidden, discover: true}
	tc.apply(&req)

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

func (s *Server) getHiddenGems(ctx context.Context, cfg *store.UserConfig, mediaType string, hidden []string) catalogEntry {
	req := catalogRequest{tag: "Hidden Gems", kind: ai.KindGems, mediaType: mediaType, hidden: hidden, discover: true}
	s.loadTrakt(ctx, cfg).apply(&req)
	return s.buildCatalog(ctx, cfg, req)
}

func (s *Server) getBecauseYouWatched(ctx context.Context, cfg *store.UserConfig, mediaType string, hidden []string) catalogEntry {
	if !cfg.TraktConnected || s.traktClient == nil {
		return catalogEntry{}
	}

	tc := s.loadTrakt(ctx, cfg)
	if len(tc.history) == 0 {
		return catalogEntry{}
	}

	req := catalogRequest{tag: "Because You Watched", kind: ai.KindBecauseYouWatched, mediaType: mediaType, hidden: hidden}
	tc.apply(&req)
	req.input.WatchHistory = nil // the seed title is the context here
	req.input.SeedTitle = formatTitles(tc.history[:1])[0]
	return s.buildCatalog(ctx, cfg, req)
}

func (s *Server) getFocusedPicks(ctx context.Context, cfg *store.UserConfig, kind ai.PromptKind, hidden []string) catalogEntry {
	tag := "Anime Picks"
	if kind == ai.KindDocumentary {
		tag = "Documentary Picks"
	}
	req := catalogRequest{tag: tag, kind: kind, hidden: hidden, discover: true}
	s.loadTrakt(ctx, cfg).apply(&req)
	return s.buildCatalog(ctx, cfg, req)
}

// buildCatalog produces up to cfg.MaxResults titles that all honor the
// user's settings: AI suggestions first (with a second round to cover what the
// filter dropped), then TMDB discover for any remaining shortfall.
func (s *Server) buildCatalog(ctx context.Context, cfg *store.UserConfig, req catalogRequest) catalogEntry {
	rules := prefs.ForMedia(cfg, ai.RulesKind(req.kind), req.mediaType)
	filter := newResultFilter(rules, allowedTypes(cfg, req.mediaType), req.blocked)
	filter.hide(req.hidden)
	want := cfg.MaxResults

	var kept []tmdb.SearchResult
	if providers := s.buildProviderChain(cfg); len(providers) > 0 {
		kept = s.aiRounds(ctx, cfg, req, providers, filter, want)
	}

	if len(kept) > 0 && len(kept) < want {
		extra := s.relatedFill(ctx, kept, filter)
		if need := want - len(kept); len(extra) > need {
			extra = extra[:need]
		}
		if len(extra) > 0 {
			s.tmdbClient.AttachIMDBIds(ctx, extra)
			log.Printf("[%s %s] filled %d from TMDB recommendations", req.tag, req.mediaType, len(extra))
			kept = append(kept, extra...)
		}
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

	final := typed(kept, req.mediaType)
	if want > 0 && len(final) > want {
		final = final[:want]
	}
	entry := catalogEntry{Metas: toMetas(final)}
	for _, r := range final {
		entry.Keys = append(entry.Keys, seenKey(r))
	}
	return entry
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
		if len(filter.belowBar) > 0 {
			log.Printf("[%s %s] round %d under the rating bar: %s", req.tag, req.mediaType, round, strings.Join(filter.belowBar, ", "))
		}

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

// relatedSeeds caps how many kept titles relatedFill asks TMDB about.
const relatedSeeds = 8

// relatedFill returns TMDB's recommendations for the titles the AI kept that
// pass the row's filter, those recommended for the most seeds first. It runs
// before discover: ranked by TMDB rating, discover's lists were topped by
// shows with small fan bases (Power Book IV, AEW Dynamite in Hidden Gems).
func (s *Server) relatedFill(ctx context.Context, seeds []tmdb.SearchResult, filter *resultFilter) []tmdb.SearchResult {
	var lists [][]tmdb.SearchResult
	for _, seed := range seeds[:min(len(seeds), relatedSeeds)] {
		if ctx.Err() != nil {
			break
		}
		lists = append(lists, s.tmdbClient.Recommendations(ctx, seed.MediaType, seed.ID))
	}
	accepted, _ := filter.apply(rankRelated(lists))
	return accepted
}

// rankRelated merges recommendation lists: titles recommended for more
// seeds first, then in the order first seen.
func rankRelated(lists [][]tmdb.SearchResult) []tmdb.SearchResult {
	count := map[string]int{}
	var order []tmdb.SearchResult
	for _, list := range lists {
		for _, r := range list {
			k := seenKey(r)
			if count[k] == 0 {
				order = append(order, r)
			}
			count[k]++
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		return count[seenKey(order[i])] > count[seenKey(order[j])]
	})
	return order
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
		// months. TV movies (specials) are skipped. Shows collect far fewer
		// votes than films, so they get lower bounds: the film bounds filled
		// the row with Andor, Outer Banks and 9-1-1.
		o.MinRating = max(o.MinRating, 7.0)
		// Films over 2,000 votes were titles most people have heard of
		// (Togo, The Woman King, Just Mercy at 2.4-2.6k).
		o.MinVotes, o.MaxVotes = 300, 2000
		if req.mediaType == "series" {
			o.MinVotes, o.MaxVotes = 80, prefs.GemsMaxVotesSeries
		}
		o.SortBy = "vote_average.desc"
		o.ReleasedBefore = time.Now().AddDate(-3, 0, 0)
		o.SkipTVMovies = true
	case ai.KindAnime, ai.KindDocumentary:
		o.Genres = []string{rules.Required}
	default:
		// Best rated rather than most popular: popularity filled AI Picks
		// with network procedurals (Blue Bloods, Hawaii Five-0, 9-1-1).
		// Acclaimed titles the user has seen are dropped by the filter.
		o.MinRating = max(o.MinRating, 7.0)
		o.MinVotes = 1000
		if req.mediaType == "series" {
			o.MinVotes = 300
		}
		o.SortBy = "vote_average.desc"
	}

	accepted, _ := filter.apply(s.tmdbClient.DiscoverFallback(ctx, o))
	return accepted
}

func (s *Server) buildProviderChain(cfg *store.UserConfig) []ai.ProviderEntry {
	var chain []ai.ProviderEntry

	// DeepSeek first: over five days its suggestions existed on TMDB 75-100%
	// of the time against Groq's 45-68% ("The Terror: Crimson Sky"), and
	// Groq's free tier ran out of tokens per minute when the rows refreshed
	// together. Groq stays as the fallback.
	if cfg.DeepSeekKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.DeepSeekProvider{}, APIKey: cfg.DeepSeekKey})
	}
	if cfg.GroqKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.GroqProvider{}, APIKey: cfg.GroqKey})
	}
	if cfg.GeminiKey != "" {
		chain = append(chain, ai.ProviderEntry{Provider: &ai.GeminiProvider{}, APIKey: cfg.GeminiKey})
	}

	return chain
}

// typed keeps the results a row of mediaType ("movie"/"series") may show;
// "" keeps all.
func typed(results []tmdb.SearchResult, mediaType string) []tmdb.SearchResult {
	var out []tmdb.SearchResult
	for _, r := range results {
		stremioType := "movie"
		if r.MediaType == "tv" {
			stremioType = "series"
		}
		if stremioType == mediaType || mediaType == "" {
			out = append(out, r)
		}
	}
	return out
}

func toMetas(results []tmdb.SearchResult) []stremio.Meta {
	metas := []stremio.Meta{}
	for _, r := range results {
		metas = append(metas, stremio.TMDBResultToMeta(r, r.Reason))
	}
	return metas
}

func filterByType(results []tmdb.SearchResult, mediaType string) []stremio.Meta {
	return toMetas(typed(results, mediaType))
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
