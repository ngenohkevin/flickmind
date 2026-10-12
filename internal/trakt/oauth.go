package trakt

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authorizeURL is Trakt's OAuth host since its 2026 auth migration.
const authorizeURL = "https://auth.trakt.tv/oauth/authorize"

// ErrInvalidGrant means Trakt no longer accepts the refresh token, so the
// connection can't recover without the user reconnecting.
var ErrInvalidGrant = errors.New("trakt: refresh token rejected")

// NewPKCE returns a fresh code verifier and its S256 code challenge. Trakt
// signs users in with PKCE; the client secret is deprecated for sign-in.
func NewPKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf) // 64 chars of A-Z a-z 0-9 - _
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// AuthorizeURL builds the sign-in URL. state must be a random, single-use
// value the callback can verify; codeChallenge comes from NewPKCE.
func (c *Client) AuthorizeURL(redirectURI, state, codeChallenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"scope":                 {"public offline_access"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return authorizeURL + "?" + q.Encode()
}

// ExchangeCode trades an authorization code for tokens, proving the sign-in
// with the PKCE code verifier instead of a client secret.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	body := map[string]string{
		"code":          code,
		"client_id":     c.clientID,
		"redirect_uri":  redirectURI,
		"grant_type":    "authorization_code",
		"code_verifier": codeVerifier,
	}
	return c.tokenRequest(ctx, body)
}

// RefreshToken trades a single-use refresh token for a new token pair.
func (c *Client) RefreshToken(ctx context.Context, refreshToken, redirectURI string) (*TokenResponse, error) {
	body := map[string]string{
		"refresh_token": refreshToken,
		"client_id":     c.clientID,
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/oauth/token", bytes.NewReader(jsonBody))
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
		if (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized) &&
			strings.Contains(string(respBody), "invalid_grant") {
			return nil, fmt.Errorf("%w: %s", ErrInvalidGrant, string(respBody))
		}
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
