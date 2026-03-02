package stremio

import (
	"fmt"

	"github.com/ngenohkevin/flickmind/internal/tmdb"
)

func TMDBResultToMeta(r tmdb.SearchResult, reason string) Meta {
	meta := Meta{
		Name: r.Title,
	}

	// Use IMDB ID when available, fall back to tmdb: prefix
	if r.IMDBId != "" {
		meta.ID = r.IMDBId
	} else {
		meta.ID = fmt.Sprintf("tmdb:%d", r.ID)
	}

	if r.MediaType == "tv" {
		meta.Type = "series"
	} else {
		meta.Type = "movie"
	}

	if r.PosterPath != "" {
		meta.Poster = "https://image.tmdb.org/t/p/w500" + r.PosterPath
	}
	if r.BackdropPath != "" {
		meta.Background = "https://image.tmdb.org/t/p/w1280" + r.BackdropPath
	}

	if r.Year > 0 {
		meta.Year = fmt.Sprintf("%d", r.Year)
	}
	if r.VoteAverage > 0 {
		meta.IMDBRating = fmt.Sprintf("%.1f", r.VoteAverage)
	}

	// Populate genre names from IDs
	if len(r.GenreIDs) > 0 {
		meta.Genres = tmdb.GenreIDsToNames(r.GenreIDs, r.MediaType)
	}

	// Add IMDB link
	if r.IMDBId != "" {
		meta.Links = append(meta.Links, Link{
			Name:     "IMDB",
			Category: "imdb",
			URL:      "https://www.imdb.com/title/" + r.IMDBId + "/",
		})
	}

	desc := reason
	if r.Overview != "" {
		if desc != "" {
			desc += "\n\n"
		}
		desc += r.Overview
	}
	meta.Description = desc

	return meta
}
