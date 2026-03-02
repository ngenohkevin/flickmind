package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.themoviedb.org/3"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", c.apiKey)

	u := fmt.Sprintf("%s%s?%s", baseURL, path, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) SearchMovies(ctx context.Context, query string, year int) ([]tmdbMovieResult, error) {
	params := url.Values{"query": {query}}
	if year > 0 {
		params.Set("primary_release_year", fmt.Sprintf("%d", year))
	}

	data, err := c.get(ctx, "/search/movie", params)
	if err != nil {
		return nil, err
	}

	var resp tmdbSearchResponse[tmdbMovieResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (c *Client) SearchTV(ctx context.Context, query string, year int) ([]tmdbTVResult, error) {
	params := url.Values{"query": {query}}
	if year > 0 {
		params.Set("first_air_date_year", fmt.Sprintf("%d", year))
	}

	data, err := c.get(ctx, "/search/tv", params)
	if err != nil {
		return nil, err
	}

	var resp tmdbSearchResponse[tmdbTVResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (c *Client) DiscoverMovies(ctx context.Context, params url.Values) ([]tmdbMovieResult, error) {
	data, err := c.get(ctx, "/discover/movie", params)
	if err != nil {
		return nil, err
	}

	var resp tmdbSearchResponse[tmdbMovieResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (c *Client) GetExternalIDs(ctx context.Context, tmdbID int, mediaType string) (string, error) {
	path := fmt.Sprintf("/%s/%d/external_ids", mediaType, tmdbID)
	data, err := c.get(ctx, path, nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		IMDBId string `json:"imdb_id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.IMDBId, nil
}

func (c *Client) DiscoverTV(ctx context.Context, params url.Values) ([]tmdbTVResult, error) {
	data, err := c.get(ctx, "/discover/tv", params)
	if err != nil {
		return nil, err
	}

	var resp tmdbSearchResponse[tmdbTVResult]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}
