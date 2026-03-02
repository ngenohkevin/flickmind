package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserConfig struct {
	UserID               string     `json:"userId"`
	GroqKey              string     `json:"groqKey,omitempty"`
	DeepSeekKey          string     `json:"deepseekKey,omitempty"`
	GeminiKey            string     `json:"geminiKey,omitempty"`
	TraktAccessToken     string     `json:"-"`
	TraktRefreshToken    string     `json:"-"`
	TraktExpiresAt       *time.Time `json:"-"`
	TraktConnected       bool       `json:"traktConnected"`
	Genres               []string   `json:"genres"`
	ContentTypes         []string   `json:"contentTypes"`
	Language             string     `json:"language"`
	Mood                 string     `json:"mood"`
	MinRating            float64    `json:"minRating"`
	YearFrom             int        `json:"yearFrom"`
	YearTo               int        `json:"yearTo"`
	MaxResults           int        `json:"maxResults"`
	RecommendationSource string     `json:"recommendationSource"`
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateUser(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_config (user_id) VALUES ($1)`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UserExists(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, id).Scan(&count)
	return count > 0, err
}

func (s *Store) GetUserConfig(ctx context.Context, userID string) (*UserConfig, error) {
	cfg := &UserConfig{UserID: userID}
	var genres, contentTypes string
	var traktExpiresAt *time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT groq_key, deepseek_key, gemini_key,
		       trakt_access_token, trakt_refresh_token, trakt_expires_at,
		       genres, content_types, language, mood, min_rating,
		       year_from, year_to, max_results, recommendation_source
		FROM user_config WHERE user_id = $1`, userID).Scan(
		&cfg.GroqKey, &cfg.DeepSeekKey, &cfg.GeminiKey,
		&cfg.TraktAccessToken, &cfg.TraktRefreshToken, &traktExpiresAt,
		&genres, &contentTypes, &cfg.Language, &cfg.Mood, &cfg.MinRating,
		&cfg.YearFrom, &cfg.YearTo, &cfg.MaxResults, &cfg.RecommendationSource,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, err
	}

	cfg.TraktExpiresAt = traktExpiresAt
	cfg.TraktConnected = cfg.TraktAccessToken != ""
	cfg.Genres = splitCSV(genres)
	cfg.ContentTypes = splitCSV(contentTypes)
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 25
	}
	if cfg.RecommendationSource == "" {
		cfg.RecommendationSource = "preferences"
	}

	return cfg, nil
}

func (s *Store) SaveUserConfig(ctx context.Context, cfg *UserConfig) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_config SET
		    groq_key = $1, deepseek_key = $2, gemini_key = $3,
		    genres = $4, content_types = $5, language = $6, mood = $7, min_rating = $8,
		    year_from = $9, year_to = $10, max_results = $11, recommendation_source = $12,
		    updated_at = NOW()
		WHERE user_id = $13`,
		cfg.GroqKey, cfg.DeepSeekKey, cfg.GeminiKey,
		joinCSV(cfg.Genres), joinCSV(cfg.ContentTypes), cfg.Language, cfg.Mood, cfg.MinRating,
		cfg.YearFrom, cfg.YearTo, cfg.MaxResults, cfg.RecommendationSource,
		cfg.UserID,
	)
	return err
}

func (s *Store) SaveTraktTokens(ctx context.Context, userID, accessToken, refreshToken string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_config SET
		    trakt_access_token = $1, trakt_refresh_token = $2, trakt_expires_at = $3,
		    updated_at = NOW()
		WHERE user_id = $4`,
		accessToken, refreshToken, expiresAt, userID,
	)
	return err
}

func (s *Store) ClearTraktTokens(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_config SET
		    trakt_access_token = '', trakt_refresh_token = '', trakt_expires_at = NULL,
		    updated_at = NOW()
		WHERE user_id = $1`, userID,
	)
	return err
}

// Cache operations

func (s *Store) GetCache(ctx context.Context, key string) (string, bool) {
	var data string
	err := s.pool.QueryRow(ctx, `
		SELECT data FROM recommendation_cache
		WHERE cache_key = $1 AND expires_at > NOW()`, key).Scan(&data)
	if err != nil {
		return "", false
	}
	return data, true
}

func (s *Store) SetCache(ctx context.Context, key, data string, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO recommendation_cache (cache_key, data, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (cache_key) DO UPDATE SET data = $2, expires_at = $3`,
		key, data, expiresAt)
	return err
}

func (s *Store) InvalidateUserCache(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM recommendation_cache WHERE cache_key LIKE $1`, userID+":%")
	return err
}

func (s *Store) CleanExpiredCache(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM recommendation_cache WHERE expires_at <= NOW()`)
	return err
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

func joinCSV(ss []string) string {
	return strings.Join(ss, ",")
}
