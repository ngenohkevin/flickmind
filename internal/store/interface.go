package store

import (
	"context"
	"time"
)

// StoreInterface abstracts Store for testing.
type StoreInterface interface {
	CreateUser(ctx context.Context, id string) error
	UserExists(ctx context.Context, id string) (bool, error)
	GetUserConfig(ctx context.Context, userID string) (*UserConfig, error)
	SaveUserConfig(ctx context.Context, cfg *UserConfig) error
	SaveTraktTokens(ctx context.Context, userID, accessToken, refreshToken string, expiresAt time.Time) error
	ClearTraktTokens(ctx context.Context, userID string) error
	GetCache(ctx context.Context, key string) (string, bool)
	SetCache(ctx context.Context, key, data string, ttl time.Duration) error
	InvalidateUserCache(ctx context.Context, userID string) error
}
