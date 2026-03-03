package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ngenohkevin/flickmind/internal/cache"
	"github.com/ngenohkevin/flickmind/internal/config"
	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

type Server struct {
	cfg         *config.Config
	store       store.StoreInterface
	tmdbClient  *tmdb.Client
	traktClient *trakt.Client
	cache       *cache.Cache
	router      *gin.Engine
}

func New(cfg *config.Config, pool *pgxpool.Pool) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		cfg:        cfg,
		store:      store.New(pool),
		tmdbClient: tmdb.NewClient(cfg.TMDBAPIKey),
		cache:      cache.New(cfg.RedisURL),
	}

	if cfg.TraktClientID != "" && cfg.TraktClientSecret != "" {
		s.traktClient = trakt.NewClient(cfg.TraktClientID, cfg.TraktClientSecret)
	}

	s.router = s.setupRouter()
	return s
}

func NewForTest(cfg *config.Config, st store.StoreInterface, tmdbClient *tmdb.Client, traktClient *trakt.Client, c *cache.Cache) *Server {
	gin.SetMode(gin.TestMode)

	s := &Server{
		cfg:         cfg,
		store:       st,
		tmdbClient:  tmdbClient,
		traktClient: traktClient,
		cache:       c,
	}

	s.router = s.setupRouter()
	return s
}

func (s *Server) setupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	router.Use(corsMiddleware())

	// Health
	router.GET("/health", s.handleHealth)

	// Config API
	router.POST("/api/config", s.handleCreateConfig)
	router.GET("/api/config/:userId", s.handleGetConfig)
	router.POST("/api/config/:userId", s.handleUpdateConfig)

	// Trakt OAuth
	router.GET("/api/trakt/auth/:userId", s.handleTraktAuth)
	router.GET("/api/trakt/callback", s.handleTraktCallback)
	router.POST("/api/trakt/disconnect/:userId", s.handleTraktDisconnect)

	// Stremio endpoints
	router.GET("/:userId/manifest.json", s.handleManifest)
	router.GET("/:userId/catalog/:type/:id", s.handleCatalog)
	router.GET("/:userId/catalog/:type/:id/*extra", s.handleCatalog)
	router.GET("/:userId/configure", s.handleConfigureRedirect)

	return router
}

func (s *Server) Router() *gin.Engine {
	return s.router
}

func (s *Server) Close() {
	s.cache.Close()
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
