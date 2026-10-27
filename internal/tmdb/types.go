package tmdb

type SearchResult struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	MediaType    string  `json:"media_type"` // "movie" or "tv"
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	Overview     string  `json:"overview"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	Year         int     `json:"year"`
	GenreIDs     []int   `json:"genre_ids"`
	Popularity   float64 `json:"popularity"`
	IMDBId       string  `json:"imdb_id,omitempty"`
	Reason       string  `json:"-"` // AI recommendation reason, not serialized
	// OriginalLanguage is TMDB's ISO 639-1 code for the title's original language.
	OriginalLanguage string `json:"original_language,omitempty"`
}

type tmdbMovieResult struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	OriginalLanguage string  `json:"original_language"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	Overview         string  `json:"overview"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
	ReleaseDate      string  `json:"release_date"`
	GenreIDs         []int   `json:"genre_ids"`
	Popularity       float64 `json:"popularity"`
}

type tmdbTVResult struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	OriginalName     string  `json:"original_name"`
	OriginalLanguage string  `json:"original_language"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	Overview         string  `json:"overview"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
	FirstAirDate     string  `json:"first_air_date"`
	GenreIDs         []int   `json:"genre_ids"`
	Popularity       float64 `json:"popularity"`
}

type tmdbSearchResponse[T any] struct {
	Results []T `json:"results"`
}

// Details is one title looked up by ID: the search fields plus its genre
// names and, for shows, how many episodes have aired.
type Details struct {
	SearchResult
	GenreNames       []string
	NumberOfEpisodes int
}
