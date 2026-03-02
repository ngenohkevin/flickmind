package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/ngenohkevin/flickmind/internal/config"
	"github.com/ngenohkevin/flickmind/internal/server"
	"github.com/ngenohkevin/flickmind/internal/store"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if cfg.TMDBAPIKey == "" {
		log.Println("[WARN] TMDB_API_KEY not set, TMDB features will not work")
	}

	pool, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Database: %v", err)
	}
	defer pool.Close()

	srv := server.New(cfg, pool)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] FlickMind listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server: %v", err)
		}
	}()

	<-sigChan
	log.Println("[INFO] Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	srv.Close()

	log.Println("[INFO] FlickMind stopped")
}
