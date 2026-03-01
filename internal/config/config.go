package config

import "os"

type Config struct {
	Port              string
	BaseURL           string
	FrontendURL       string
	DatabaseURL       string
	TMDBAPIKey        string
	TraktClientID     string
	TraktClientSecret string
}

func Load() *Config {
	return &Config{
		Port:              getEnv("PORT", "7000"),
		BaseURL:           getEnv("BASE_URL", "http://localhost:7000"),
		FrontendURL:       getEnv("FRONTEND_URL", "http://localhost:3000"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://flickmind:flickmind@localhost:5432/flickmind?sslmode=disable"),
		TMDBAPIKey:        os.Getenv("TMDB_API_KEY"),
		TraktClientID:     os.Getenv("TRAKT_CLIENT_ID"),
		TraktClientSecret: os.Getenv("TRAKT_CLIENT_SECRET"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
