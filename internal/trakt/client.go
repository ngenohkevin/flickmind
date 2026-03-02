package trakt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBaseURL = "https://api.trakt.tv"

type Client struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) GetWatchlist(ctx context.Context, accessToken string, limit int) ([]WatchedItem, error) {
	url := fmt.Sprintf("%s/users/me/watchlist/movies,shows?limit=%d", apiBaseURL, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trakt API error %d: %s", resp.StatusCode, string(body))
	}

	var watchlist []traktWatchlistItem
	if err := json.NewDecoder(resp.Body).Decode(&watchlist); err != nil {
		return nil, err
	}

	var items []WatchedItem
	seen := make(map[string]bool)
	for _, w := range watchlist {
		var item WatchedItem
		switch w.Type {
		case "movie":
			if w.Movie == nil {
				continue
			}
			item = WatchedItem{Title: w.Movie.Title, Year: w.Movie.Year, MediaType: "movie"}
		case "show":
			if w.Show == nil {
				continue
			}
			item = WatchedItem{Title: w.Show.Title, Year: w.Show.Year, MediaType: "show"}
		default:
			continue
		}

		key := fmt.Sprintf("%s-%d", item.Title, item.Year)
		if !seen[key] {
			seen[key] = true
			items = append(items, item)
		}
	}

	return items, nil
}

func (c *Client) GetWatchHistory(ctx context.Context, accessToken string, limit int) ([]WatchedItem, error) {
	url := fmt.Sprintf("%s/users/me/history?limit=%d", apiBaseURL, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trakt API error %d: %s", resp.StatusCode, string(body))
	}

	var history []traktHistoryItem
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		return nil, err
	}

	var items []WatchedItem
	seen := make(map[string]bool)
	for _, h := range history {
		var item WatchedItem
		switch h.Type {
		case "movie":
			if h.Movie == nil {
				continue
			}
			item = WatchedItem{Title: h.Movie.Title, Year: h.Movie.Year, MediaType: "movie"}
		case "episode":
			if h.Show == nil {
				continue
			}
			item = WatchedItem{Title: h.Show.Title, Year: h.Show.Year, MediaType: "show"}
		default:
			continue
		}

		key := fmt.Sprintf("%s-%d", item.Title, item.Year)
		if !seen[key] {
			seen[key] = true
			items = append(items, item)
		}
	}

	return items, nil
}
