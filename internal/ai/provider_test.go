package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockProvider struct {
	name string
	fn   func(ctx context.Context, apiKey, prompt string) ([]Recommendation, error)
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) GetRecommendations(ctx context.Context, apiKey, prompt string) ([]Recommendation, error) {
	return m.fn(ctx, apiKey, prompt)
}

func TestGetRecommendations_SingleProviderSuccess(t *testing.T) {
	providers := []ProviderEntry{{
		Provider: &mockProvider{
			name: "mock",
			fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				return []Recommendation{{Title: "Test", Year: 2020, Type: "movie"}}, nil
			},
		},
		APIKey: "test-key",
	}}

	result, err := GetRecommendations(context.Background(), providers, "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "mock" {
		t.Errorf("expected provider mock, got %s", result.ProviderName)
	}
	if len(result.Recommendations) != 1 {
		t.Errorf("expected 1 rec, got %d", len(result.Recommendations))
	}
	if result.IsFallback {
		t.Error("should not be fallback for first provider")
	}
}

func TestGetRecommendations_FallbackOnFailure(t *testing.T) {
	providers := []ProviderEntry{
		{
			Provider: &mockProvider{
				name: "failing",
				fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
					return nil, errors.New("server error")
				},
			},
			APIKey: "key1",
		},
		{
			Provider: &mockProvider{
				name: "working",
				fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
					return []Recommendation{{Title: "Fallback", Year: 2020, Type: "movie"}}, nil
				},
			},
			APIKey: "key2",
		},
	}

	result, err := GetRecommendations(context.Background(), providers, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "working" {
		t.Errorf("expected working provider, got %s", result.ProviderName)
	}
	if !result.IsFallback {
		t.Error("should be marked as fallback")
	}
}

func TestGetRecommendations_SkipEmptyAPIKeys(t *testing.T) {
	called := false
	providers := []ProviderEntry{
		{
			Provider: &mockProvider{name: "skipped", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				t.Error("should not be called with empty key")
				return nil, nil
			}},
			APIKey: "",
		},
		{
			Provider: &mockProvider{name: "used", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				called = true
				return []Recommendation{{Title: "Test", Year: 2020, Type: "movie"}}, nil
			}},
			APIKey: "real-key",
		},
	}

	result, err := GetRecommendations(context.Background(), providers, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("second provider should have been called")
	}
	if result.ProviderName != "used" {
		t.Errorf("expected used, got %s", result.ProviderName)
	}
}

func TestGetRecommendations_NonRetriableError(t *testing.T) {
	attempts := 0
	providers := []ProviderEntry{{
		Provider: &mockProvider{
			name: "invalid",
			fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				attempts++
				return nil, errors.New("invalid api key")
			},
		},
		APIKey: "bad-key",
	}}

	_, err := GetRecommendations(context.Background(), providers, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("non-retriable error should only try once, got %d attempts", attempts)
	}
}

func TestGetRecommendations_AllProvidersFail(t *testing.T) {
	providers := []ProviderEntry{
		{
			Provider: &mockProvider{name: "p1", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				return nil, errors.New("invalid api key")
			}},
			APIKey: "key1",
		},
		{
			Provider: &mockProvider{name: "p2", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				return nil, errors.New("invalid api key")
			}},
			APIKey: "key2",
		},
	}

	_, err := GetRecommendations(context.Background(), providers, "test")
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Error("error should contain last error")
	}
}

func TestGetRecommendations_NoProviders(t *testing.T) {
	_, err := GetRecommendations(context.Background(), nil, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "no AI providers configured" {
		t.Errorf("expected 'no AI providers configured', got %s", err.Error())
	}
}

func TestGetRecommendations_EmptyResultsFallthrough(t *testing.T) {
	providers := []ProviderEntry{
		{
			Provider: &mockProvider{name: "empty", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				return []Recommendation{}, nil
			}},
			APIKey: "key1",
		},
		{
			Provider: &mockProvider{name: "full", fn: func(_ context.Context, _, _ string) ([]Recommendation, error) {
				return []Recommendation{{Title: "Result", Year: 2020, Type: "movie"}}, nil
			}},
			APIKey: "key2",
		},
	}

	result, err := GetRecommendations(context.Background(), providers, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "full" {
		t.Errorf("expected full provider, got %s", result.ProviderName)
	}
}

func TestGetRecommendations_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	providers := []ProviderEntry{{
		Provider: &mockProvider{
			name: "slow",
			fn: func(ctx context.Context, _, _ string) ([]Recommendation, error) {
				cancel() // cancel immediately
				return nil, errors.New("temporary error")
			},
		},
		APIKey: "key",
	}}

	_, err := GetRecommendations(ctx, providers, "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsNonRetriable(t *testing.T) {
	tests := []struct {
		err      string
		expected bool
	}{
		{"invalid api key", true},
		{"not configured", true},
		{"insufficient balance", true},
		{"server error", false},
		{"timeout", false},
		{"rate limited", false},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			result := isNonRetriable(errors.New(tt.err))
			if result != tt.expected {
				t.Errorf("isNonRetriable(%q) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestWithRetry_SucceedsOnRetry(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	recs, err := withRetry(ctx, func() ([]Recommendation, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("temporary error")
		}
		return []Recommendation{{Title: "Success", Year: 2020, Type: "movie"}}, nil
	}, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Errorf("expected 1 rec, got %d", len(recs))
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
