package ai

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

type Recommendation struct {
	Title  string `json:"title"`
	Year   int    `json:"year"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type Provider interface {
	Name() string
	GetRecommendations(ctx context.Context, apiKey, prompt string) ([]Recommendation, error)
}

type ProviderResult struct {
	Recommendations []Recommendation
	ProviderName    string
	IsFallback      bool
}

type ProviderEntry struct {
	Provider Provider
	APIKey   string
}

// providerTimeout returns a per-provider timeout so fallbacks get a fair chance.
// DeepSeek is slower and needs more time.
func providerTimeout(name string) time.Duration {
	if name == "deepseek" {
		return 45 * time.Second
	}
	return 30 * time.Second
}

func GetRecommendations(ctx context.Context, providers []ProviderEntry, prompt string) (*ProviderResult, error) {
	var lastErr error

	for i, entry := range providers {
		if entry.APIKey == "" {
			continue
		}

		// Check if parent context is already done before trying next provider
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}

		log.Printf("[AI] Trying provider: %s", entry.Provider.Name())

		providerCtx, cancel := context.WithTimeout(ctx, providerTimeout(entry.Provider.Name()))
		recs, err := withRetry(providerCtx, func() ([]Recommendation, error) {
			return entry.Provider.GetRecommendations(providerCtx, entry.APIKey, prompt)
		}, entry.Provider.Name())
		cancel()

		if err != nil {
			log.Printf("[AI] Provider %s failed: %v", entry.Provider.Name(), err)
			lastErr = err

			if isNonRetriable(err) {
				continue
			}
			continue
		}

		if len(recs) == 0 {
			log.Printf("[AI] Provider %s returned no recommendations", entry.Provider.Name())
			continue
		}

		log.Printf("[AI] Got %d recommendations from %s", len(recs), entry.Provider.Name())
		return &ProviderResult{
			Recommendations: recs,
			ProviderName:    entry.Provider.Name(),
			IsFallback:      i > 0,
		}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, errors.New("no AI providers configured")
}

func isNonRetriable(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not configured") ||
		strings.Contains(msg, "api key") ||
		strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "429")
}

func withRetry(ctx context.Context, fn func() ([]Recommendation, error), providerName string) ([]Recommendation, error) {
	const maxRetries = 2
	var lastErr error
	delay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		recs, err := fn()
		if err == nil {
			return recs, nil
		}

		lastErr = err
		if isNonRetriable(err) {
			return nil, err
		}

		if attempt < maxRetries {
			log.Printf("[%s] Attempt %d failed, retrying in %v...", providerName, attempt+1, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * math.Pow(2, 1))
			if delay > 3*time.Second {
				delay = 3 * time.Second
			}
		}
	}

	return nil, lastErr
}
