// Package prefs turns a user's saved config into the concrete rules every
// catalog must honor. The AI prompt states these rules and the server
// enforces them against TMDB metadata, so both read from the same place.
package prefs

import (
	"strings"

	"github.com/ngenohkevin/flickmind/internal/store"
)

// Kind is the catalog family a set of rules is built for.
type Kind int

const (
	// Picks covers AI Picks, watchlist picks and Because You Watched.
	Picks Kind = iota
	Gems
	Anime
	Documentary
)

// GemsMaxVotes is the TMDB vote count above which a film is too well known
// to be a hidden gem. It matches the threshold stated in the gems prompt.
const GemsMaxVotes = 5000

// GemsMaxVotesSeries is the same for TV. Shows collect far fewer TMDB votes
// (Andor ~2.3k, Severance ~3k, The Shadow Line 84), so the film cap let
// mainstream shows into Hidden Gems.
const GemsMaxVotesSeries = 1000

// GemsMaxVotesFor returns the hidden-gems vote cap for a row's media type.
func GemsMaxVotesFor(mediaType string) int {
	if mediaType == "series" {
		return GemsMaxVotesSeries
	}
	return GemsMaxVotes
}

// QualityFloor drops poorly reviewed titles from every row, including when
// the user set no minimum rating (an AI pick rated 4.4 got through). A title
// with fewer than QualityFloorMinVotes votes has no reliable rating yet and
// is let through.
const (
	QualityFloor         = 6.0
	QualityFloorMinVotes = 20
)

// Hidden gems promise quality: below these, a title is just obscure.
const (
	GemsMinRating = 6.5
	GemsMinVotes  = 50
)

// Rules are the constraints a catalog's results must satisfy. Genre names are
// the lowercase config names ("sci-fi", "action", ...).
type Rules struct {
	// Selected genres: a title must carry at least one. Empty means any genre.
	Selected []string
	// Excluded genres: a title carrying any of these is dropped.
	Excluded []string
	// Required genre that every title must carry ("animation", "documentary").
	Required string
	// Languages lists accepted TMDB original_language codes. Empty means any.
	Languages []string
	YearFrom  int
	YearTo    int
	MinRating float64
	// QualityFloor drops titles rated below it once they have
	// QualityFloorMinVotes votes, whatever MinRating says.
	QualityFloor float64
	// MaxVotes drops titles with more TMDB votes than this. 0 means no cap.
	MaxVotes int
	// MinVotes drops titles with fewer TMDB votes (an unrated title can't be
	// a quality pick). 0 means no floor.
	MinVotes int
}

// selectable mirrors the genres offered on the config page.
var selectable = []string{
	"action", "adventure", "animation", "comedy", "crime", "documentary", "drama",
	"fantasy", "horror", "mystery", "romance", "sci-fi", "thriller", "war",
}

// neutral genres are tagged on so many titles that leaving them unselected
// can't mean "never show me this".
var neutral = map[string]bool{"drama": true}

// alwaysExcluded are TV formats nobody asks a recommendation engine for.
var alwaysExcluded = []string{"kids", "news", "reality", "soap", "talk"}

// ForMedia builds the rules for one catalog row; mediaType is "movie",
// "series", or "" for the mixed rows.
func ForMedia(cfg *store.UserConfig, kind Kind, mediaType string) Rules {
	r := For(cfg, kind)
	if kind == Gems {
		r.MaxVotes = GemsMaxVotesFor(mediaType)
	}
	return r
}

// For builds the rules for one catalog kind, with film thresholds.
func For(cfg *store.UserConfig, kind Kind) Rules {
	r := Rules{
		YearFrom:     cfg.YearFrom,
		YearTo:       cfg.YearTo,
		MinRating:    cfg.MinRating,
		QualityFloor: QualityFloor,
	}

	selected := map[string]bool{}
	for _, g := range cfg.Genres {
		selected[strings.ToLower(g)] = true
	}

	switch kind {
	case Anime:
		r.Required = "animation"
		// Anime is Japanese by definition, so the language preference doesn't apply.
		for _, g := range []string{"horror", "romance"} {
			if !selected[g] {
				r.Excluded = append(r.Excluded, g)
			}
		}
		return r
	case Documentary:
		r.Required = "documentary"
		r.Languages = Languages(cfg.Language)
		return r
	}

	r.Languages = Languages(cfg.Language)
	if kind == Gems {
		r.MaxVotes = GemsMaxVotes
		r.MinVotes = GemsMinVotes
		r.MinRating = max(r.MinRating, GemsMinRating)
	}

	// Anime and documentaries get their own rows when those content types
	// are enabled, so keep them out of the general rows.
	ownRow := map[string]bool{}
	for _, ct := range cfg.ContentTypes {
		switch ct {
		case "anime":
			ownRow["animation"] = true
		case "documentary":
			ownRow["documentary"] = true
		}
	}

	for _, g := range selectable {
		if selected[g] && !ownRow[g] {
			r.Selected = append(r.Selected, g)
		}
	}
	excluded := map[string]bool{}
	if len(selected) > 0 {
		for _, g := range selectable {
			if !selected[g] && !neutral[g] {
				excluded[g] = true
			}
		}
		if !selected["comedy"] && !selected["animation"] {
			excluded["family"] = true
		}
	}
	for g := range ownRow {
		excluded[g] = true
	}
	for _, g := range alwaysExcluded {
		excluded[g] = true
	}
	for _, g := range append(selectable, "family", "kids", "news", "reality", "soap", "talk") {
		if excluded[g] {
			r.Excluded = append(r.Excluded, g)
		}
	}
	return r
}

// Languages maps a config language code to accepted TMDB original_language
// codes. Empty means any language.
func Languages(code string) []string {
	switch code {
	case "":
		return nil
	case "zh":
		// TMDB tags Cantonese titles "cn".
		return []string{"zh", "cn"}
	default:
		return []string{code}
	}
}

var languageNames = map[string]string{
	"en": "English", "es": "Spanish", "fr": "French", "de": "German", "ja": "Japanese",
	"ko": "Korean", "zh": "Chinese", "hi": "Hindi", "it": "Italian", "pt": "Portuguese",
}

// LanguageName returns the display name for a config language code.
func LanguageName(code string) string {
	if n, ok := languageNames[code]; ok {
		return n
	}
	return strings.ToUpper(code)
}

var displayNames = map[string]string{
	"sci-fi": "Sci-Fi", "kids": "Kids", "news": "News", "reality": "Reality TV",
	"soap": "Soap Operas", "talk": "Talk Shows",
}

// DisplayGenre returns a human-readable genre name for prompts.
func DisplayGenre(g string) string {
	if n, ok := displayNames[g]; ok {
		return n
	}
	if g == "" {
		return g
	}
	return strings.ToUpper(g[:1]) + g[1:]
}
