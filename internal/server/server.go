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
	store       *store.Store
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
		cache:      cache.New(),
	}

	if cfg.TraktClientID != "" && cfg.TraktClientSecret != "" {
		s.traktClient = trakt.NewClient(cfg.TraktClientID, cfg.TraktClientSecret)
	}

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
	router.GET("/:userId/catalog/:type/:id.json", s.handleCatalog)

	s.router = router
	return s
}

func (s *Server) Router() *gin.Engine {
	return s.router
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
