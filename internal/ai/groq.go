package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type GroqProvider struct{}

func (g *GroqProvider) Name() string { return "groq" }

func (g *GroqProvider) GetRecommendations(ctx context.Context, apiKey, prompt string) ([]Recommendation, error) {
	return openAICompatibleRequest(ctx, openAIConfig{
		URL:      "https://api.groq.com/openai/v1/chat/completions",
		Model:    "llama-3.1-8b-instant",
		APIKey:   apiKey,
		Prompt:   prompt,
		Provider: "groq",
		JSONMode: true,
	})
}

type DeepSeekProvider struct{}

func (d *DeepSeekProvider) Name() string { return "deepseek" }

func (d *DeepSeekProvider) GetRecommendations(ctx context.Context, apiKey, prompt string) ([]Recommendation, error) {
	return openAICompatibleRequest(ctx, openAIConfig{
		URL:       "https://api.deepseek.com/v1/chat/completions",
		Model:     "deepseek-chat",
		APIKey:    apiKey,
		Prompt:    prompt,
		Provider:  "deepseek",
		MaxTokens: 2500,
	})
}

type openAIConfig struct {
	URL       string
	Model     string
	APIKey    string
	Prompt    string
	Provider  string
	MaxTokens int
	JSONMode  bool
}

func openAICompatibleRequest(ctx context.Context, cfg openAIConfig) ([]Recommendation, error) {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4000
	}

	body := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a movie/TV recommendation engine. Respond with valid JSON only."},
			{"role": "user", "content": cfg.Prompt},
		},
		"temperature": 0.7,
		"max_tokens":  maxTokens,
	}

	if cfg.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.URL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", cfg.Provider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s API error %d: %s", cfg.Provider, resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("%s returned empty response", cfg.Provider)
	}

	if result.Choices[0].FinishReason == "length" {
		log.Printf("[%s] WARNING: response truncated (hit max_tokens=%d)", cfg.Provider, maxTokens)
	}

	recs := ParseAIResponse(result.Choices[0].Message.Content, cfg.Provider)
	if len(recs) == 0 {
		return nil, fmt.Errorf("%s returned no valid recommendations", cfg.Provider)
	}

	return recs, nil
}
