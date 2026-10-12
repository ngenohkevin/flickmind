package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GeminiProvider struct{}

func (g *GeminiProvider) Name() string { return "gemini" }

func (g *GeminiProvider) GetRecommendations(ctx context.Context, apiKey, prompt string) ([]Recommendation, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.6-flash:generateContent?key=" + apiKey

	bodyJSON, err := json.Marshal(buildGeminiBody(prompt))
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	text, err := geminiResponseText(respBody)
	if err != nil {
		return nil, err
	}

	recs := ParseAIResponse(text, "gemini")
	if len(recs) == 0 {
		return nil, fmt.Errorf("gemini returned no valid recommendations")
	}

	return recs, nil
}

// buildGeminiBody builds the generateContent request. Gemini 3 Flash thinks at
// medium effort by default; low keeps responses inside the 30s provider timeout.
// maxOutputTokens includes thinking tokens.
func buildGeminiBody(prompt string) map[string]interface{} {
	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":      0.7,
			"maxOutputTokens":  4000,
			"responseMimeType": "application/json",
			"thinkingConfig":   map[string]string{"thinkingLevel": "low"},
		},
	}
}

// geminiResponseText returns the first non-thought text part of the first candidate.
func geminiResponseText(respBody []byte) (string, error) {
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned empty response")
	}
	for _, part := range result.Candidates[0].Content.Parts {
		if !part.Thought && part.Text != "" {
			return part.Text, nil
		}
	}
	return "", fmt.Errorf("gemini returned empty response")
}
