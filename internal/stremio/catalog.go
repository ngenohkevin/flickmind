package stremio

import (
	"fmt"

	"github.com/ngenohkevin/flickmind/internal/tmdb"
)

func TMDBResultToMeta(r tmdb.SearchResult, reason string) Meta {
	meta := Meta{
		ID:   fmt.Sprintf("tmdb:%d", r.ID),
		Name: r.Title,
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
