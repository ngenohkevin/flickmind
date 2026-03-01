package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const authorizeURL = "https://trakt.tv/oauth/authorize"

func (c *Client) AuthorizeURL(redirectURI, state string) string {
	return fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s",
		authorizeURL, c.clientID, redirectURI, state)
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	body := map[string]string{
		"code":          code,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"redirect_uri":  redirectURI,
		"grant_type":    "authorization_code",
	}
	return c.tokenRequest(ctx, body)
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken, redirectURI string) (*TokenResponse, error) {
	body := map[string]string{
		"refresh_token": refreshToken,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"redirect_uri":  redirectURI,
		"grant_type":    "refresh_token",
	}
	return c.tokenRequest(ctx, body)
}

func (c *Client) tokenRequest(ctx context.Context, body map[string]string) (*TokenResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiBaseURL+"/oauth/token", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("trakt token error %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func TokenExpiresAt(tokenResp *TokenResponse) time.Time {
	return time.Unix(int64(tokenResp.CreatedAt)+int64(tokenResp.ExpiresIn), 0)
}
