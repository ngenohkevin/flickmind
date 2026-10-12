package trakt

type WatchedItem struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	MediaType string `json:"media_type"` // "movie" or "show"
	TMDBID    int    `json:"tmdb_id,omitempty"`

	lastWatched string // RFC 3339; used only to order GetWatched results
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
	LastWatchedAt string      `json:"last_watched_at"`
	Movie         *traktMovie `json:"movie,omitempty"`
	Show          *traktShow  `json:"show,omitempty"`
}
