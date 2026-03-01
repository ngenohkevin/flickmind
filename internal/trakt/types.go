package trakt

type WatchedItem struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	MediaType string `json:"media_type"` // "movie" or "show"
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	CreatedAt    int    `json:"created_at"`
	TokenType    string `json:"token_type"`
}

type traktHistoryItem struct {
	WatchedAt string          `json:"watched_at"`
	Type      string          `json:"type"`
	Movie     *traktMovie     `json:"movie,omitempty"`
	Show      *traktShow      `json:"show,omitempty"`
}

type traktMovie struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}

type traktShow struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}
