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
	base       string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		base:   baseURL,
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

	u := fmt.Sprintf("%s%s?%s", c.base, path, params.Encode())

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

// WithBaseURL points the client at another server (tests).
func (c *Client) WithBaseURL(u string) *Client {
	c.base = u
	return c
}

// Details returns one title by TMDB ID; mediaType is "movie" or "tv". Shows
// carry their episode count and the IMDb ID from external_ids.
func (c *Client) Details(ctx context.Context, mediaType string, id int) (*Details, error) {
	params := url.Values{}
	if mediaType == "tv" {
		params.Set("append_to_response", "external_ids")
	}
	data, err := c.get(ctx, fmt.Sprintf("/%s/%d", mediaType, id), params)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID               int     `json:"id"`
		Title            string  `json:"title"`
		Name             string  `json:"name"`
		ReleaseDate      string  `json:"release_date"`
		FirstAirDate     string  `json:"first_air_date"`
		PosterPath       string  `json:"poster_path"`
		BackdropPath     string  `json:"backdrop_path"`
		Overview         string  `json:"overview"`
		VoteAverage      float64 `json:"vote_average"`
		VoteCount        int     `json:"vote_count"`
		Popularity       float64 `json:"popularity"`
		OriginalLanguage string  `json:"original_language"`
		IMDBId           string  `json:"imdb_id"`
		NumberOfEpisodes int     `json:"number_of_episodes"`
		Genres           []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"genres"`
		ExternalIDs struct {
			IMDBId string `json:"imdb_id"`
		} `json:"external_ids"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	d := &Details{SearchResult: SearchResult{
		ID: raw.ID, Title: raw.Title, MediaType: mediaType, PosterPath: raw.PosterPath, BackdropPath: raw.BackdropPath,
		Overview: raw.Overview, VoteAverage: raw.VoteAverage, VoteCount: raw.VoteCount, Popularity: raw.Popularity,
		IMDBId: raw.IMDBId, OriginalLanguage: raw.OriginalLanguage, Year: extractYear(raw.ReleaseDate),
	}, NumberOfEpisodes: raw.NumberOfEpisodes}
	if mediaType == "tv" {
		d.Title, d.Year, d.IMDBId = raw.Name, extractYear(raw.FirstAirDate), raw.ExternalIDs.IMDBId
	}
	for _, g := range raw.Genres {
		d.GenreIDs = append(d.GenreIDs, g.ID)
		d.GenreNames = append(d.GenreNames, g.Name)
	}
	return d, nil
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

func (c *Client) GetMovieIMDBId(ctx context.Context, tmdbID int) (string, error) {
	path := fmt.Sprintf("/movie/%d", tmdbID)
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

// Recommendations returns TMDB's recommendations for a title (first page);
// mediaType is "movie" or "tv". Errors give an empty list.
func (c *Client) Recommendations(ctx context.Context, mediaType string, id int) []SearchResult {
	params := url.Values{"language": {"en-US"}}
	data, err := c.get(ctx, fmt.Sprintf("/%s/%d/recommendations", mediaType, id), params)
	if err != nil {
		return nil
	}
	var results []SearchResult
	if mediaType == "tv" {
		var resp tmdbSearchResponse[tmdbTVResult]
		if json.Unmarshal(data, &resp) == nil {
			for i := range resp.Results {
				results = append(results, *tvToResult(&resp.Results[i]))
			}
		}
		return results
	}
	var resp tmdbSearchResponse[tmdbMovieResult]
	if json.Unmarshal(data, &resp) == nil {
		for i := range resp.Results {
			results = append(results, *movieToResult(&resp.Results[i]))
		}
	}
	return results
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
