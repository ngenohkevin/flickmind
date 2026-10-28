package trakt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.trakt.tv"

// ErrUnauthorized means Trakt rejected the access token (expired or revoked).
var ErrUnauthorized = errors.New("trakt: access token rejected")

type Client struct {
	clientID   string
	baseURL    string
	httpClient *http.Client
}

// NewClient needs only the app's client ID: sign-in uses PKCE, which Trakt
// requires for new apps instead of a client secret.
func NewClient(clientID string) *Client {
	return &Client{
		clientID:   clientID,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// WithBaseURL points the client at another API host (used by tests).
func (c *Client) WithBaseURL(u string) *Client {
	c.baseURL = u
	return c
}

func (c *Client) GetWatchlist(ctx context.Context, accessToken string, limit int) ([]WatchedItem, error) {
	url := fmt.Sprintf("%s/users/me/watchlist/movies,shows?limit=%d", c.baseURL, limit)

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

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
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
			item = WatchedItem{Title: w.Movie.Title, Year: w.Movie.Year, MediaType: "movie", TMDBID: w.Movie.IDs.TMDB}
		case "show":
			if w.Show == nil {
				continue
			}
			item = WatchedItem{Title: w.Show.Title, Year: w.Show.Year, MediaType: "show", TMDBID: w.Show.IDs.TMDB}
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
	url := fmt.Sprintf("%s/users/me/history?limit=%d", c.baseURL, limit)

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

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
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
			item = WatchedItem{Title: h.Movie.Title, Year: h.Movie.Year, MediaType: "movie", TMDBID: h.Movie.IDs.TMDB}
		case "episode":
			if h.Show == nil {
				continue
			}
			item = WatchedItem{Title: h.Show.Title, Year: h.Show.Year, MediaType: "show", TMDBID: h.Show.IDs.TMDB}
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

// maxWatchedPages bounds how far GetWatched pages (100 items per page).
const maxWatchedPages = 50

// GetWatched returns everything the user has ever watched, movies and shows,
// most recently watched first. History only covers recent plays (a binge of
// one show fills it), so this is what exclusion has to use.
func (c *Client) GetWatched(ctx context.Context, accessToken string) ([]WatchedItem, error) {
	var all []WatchedItem
	for _, kind := range []string{"movies", "shows"} {
		for page := 1; page <= maxWatchedPages; page++ {
			// Shows come with their seasons: Watch Again counts the episodes watched.
			path := fmt.Sprintf("/sync/watched/%s?page=%d&limit=100", kind, page)
			items, pageCount, err := c.getWatchedPage(ctx, accessToken, path)
			if err != nil {
				return nil, err
			}
			all = append(all, items...)
			if page >= pageCount || len(items) == 0 {
				break
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].LastWatchedAt > all[j].LastWatchedAt })
	return all, nil
}

// GetRatings returns the user's Trakt ratings for movies and shows. Watch
// Again leaves out anything rated 5 or lower.
func (c *Client) GetRatings(ctx context.Context, accessToken string) ([]Rating, error) {
	all := []Rating{} // none rated is an answer, not nil (nil means the read failed)
	for _, kind := range []string{"movies", "shows"} {
		req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/sync/ratings/"+kind, nil)
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
		var raw []traktRatingItem
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			resp.Body.Close()
			return nil, ErrUnauthorized
		case resp.StatusCode != 200:
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("trakt API error %d: %s", resp.StatusCode, string(body))
		default:
			err = json.NewDecoder(resp.Body).Decode(&raw)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
		}
		for _, r := range raw {
			switch {
			case r.Movie != nil:
				all = append(all, Rating{MediaType: "movie", TMDBID: r.Movie.IDs.TMDB, Rating: r.Rating})
			case r.Show != nil:
				all = append(all, Rating{MediaType: "show", TMDBID: r.Show.IDs.TMDB, Rating: r.Rating})
			}
		}
	}
	return all, nil
}

func (c *Client) getWatchedPage(ctx context.Context, accessToken, path string) ([]WatchedItem, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, 0, ErrUnauthorized
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("trakt API error %d: %s", resp.StatusCode, string(body))
	}

	var raw []traktWatchedItem
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, 0, err
	}

	pageCount, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Page-Count"))
	items := make([]WatchedItem, 0, len(raw))
	for _, w := range raw {
		switch {
		case w.Movie != nil:
			items = append(items, WatchedItem{Title: w.Movie.Title, Year: w.Movie.Year, MediaType: "movie", TMDBID: w.Movie.IDs.TMDB, LastWatchedAt: w.LastWatchedAt, Plays: w.Plays})
		case w.Show != nil:
			items = append(items, WatchedItem{Title: w.Show.Title, Year: w.Show.Year, MediaType: "show", TMDBID: w.Show.IDs.TMDB, LastWatchedAt: w.LastWatchedAt, Plays: w.Plays, Episodes: w.episodes()})
		}
	}
	return items, pageCount, nil
}
