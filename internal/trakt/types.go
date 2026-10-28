package trakt

type WatchedItem struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	MediaType string `json:"media_type"` // "movie" or "show"
	TMDBID    int    `json:"tmdb_id,omitempty"`

	// LastWatchedAt (RFC 3339) and Plays come from /sync/watched; exported so
	// the cached list keeps them — Watch Again needs both.
	LastWatchedAt string `json:"last_watched_at,omitempty"`
	Plays         int    `json:"plays,omitempty"`
	// Episodes is how many of a show's episodes were watched, each counted
	// once (specials left out); Plays counts rewatches too.
	Episodes int `json:"episodes,omitempty"`
}

// Rating is one of the user's Trakt ratings (1–10).
type Rating struct {
	MediaType string // "movie" or "show"
	TMDBID    int
	Rating    int
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	CreatedAt    int    `json:"created_at"`
	TokenType    string `json:"token_type"`
}

type traktHistoryItem struct {
	WatchedAt string      `json:"watched_at"`
	Type      string      `json:"type"`
	Movie     *traktMovie `json:"movie,omitempty"`
	Show      *traktShow  `json:"show,omitempty"`
}

type traktWatchlistItem struct {
	Type  string      `json:"type"`
	Movie *traktMovie `json:"movie,omitempty"`
	Show  *traktShow  `json:"show,omitempty"`
}

type traktIDs struct {
	TMDB int `json:"tmdb"`
}

type traktMovie struct {
	Title string   `json:"title"`
	Year  int      `json:"year"`
	IDs   traktIDs `json:"ids"`
}

type traktShow struct {
	Title string   `json:"title"`
	Year  int      `json:"year"`
	IDs   traktIDs `json:"ids"`
}

// traktWatchedItem is one entry of /sync/watched/{movies,shows}.
type traktWatchedItem struct {
	LastWatchedAt string        `json:"last_watched_at"`
	Plays         int           `json:"plays"`
	Movie         *traktMovie   `json:"movie,omitempty"`
	Show          *traktShow    `json:"show,omitempty"`
	Seasons       []traktSeason `json:"seasons,omitempty"`
}

type traktSeason struct {
	Number   int `json:"number"`
	Episodes []struct {
		Number int `json:"number"`
	} `json:"episodes"`
}

// episodes counts the episodes watched, each once, leaving out specials
// (season 0), which TMDB's episode count leaves out too.
func (w traktWatchedItem) episodes() int {
	n := 0
	for _, s := range w.Seasons {
		if s.Number > 0 {
			n += len(s.Episodes)
		}
	}
	return n
}

// traktRatingItem is one entry of /sync/ratings/{movies,shows}.
type traktRatingItem struct {
	Rating int         `json:"rating"`
	Movie  *traktMovie `json:"movie,omitempty"`
	Show   *traktShow  `json:"show,omitempty"`
}
