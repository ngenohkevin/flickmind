package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ngenohkevin/flickmind/internal/prefs"
	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// resultFilter enforces a catalog's rules on TMDB-enriched results. The AI is
// told the same rules, but it doesn't always follow them, so nothing reaches a
// catalog without passing here.
type resultFilter struct {
	rules    prefs.Rules
	types    map[string]bool // TMDB media types allowed: "movie", "tv"
	selected map[string]bool
	excluded map[string]bool
	langs    map[string]bool
	blocked  map[string][]int // normalized title -> years (watched, watchlist)
	blockIDs map[string]bool  // "movie:123" / "tv:123" TMDB keys of the same
	shown    map[string]bool  // keys the user's other rows already show
	seen     map[string]bool  // "movie:123" keys already kept
}

func newResultFilter(rules prefs.Rules, types map[string]bool, blocked []trakt.WatchedItem) *resultFilter {
	f := &resultFilter{
		rules:    rules,
		types:    types,
		selected: toSet(rules.Selected),
		excluded: toSet(rules.Excluded),
		langs:    toSet(rules.Languages),
		blocked:  map[string][]int{},
		blockIDs: map[string]bool{},
		shown:    map[string]bool{},
		seen:     map[string]bool{},
	}
	for _, item := range blocked {
		key := tmdb.NormTitle(item.Title)
		f.blocked[key] = append(f.blocked[key], item.Year)
		if item.TMDBID > 0 {
			mediaType := "movie"
			if item.MediaType == "show" {
				mediaType = "tv"
			}
			f.blockIDs[fmt.Sprintf("%s:%d", mediaType, item.TMDBID)] = true
		}
	}
	return f
}

// allowedTypes returns the TMDB media types a catalog may contain. A typed
// catalog row is limited to its type; the mixed anime/documentary rows follow
// the user's explicit Movies/TV Shows selections.
func allowedTypes(cfg *store.UserConfig, mediaType string) map[string]bool {
	switch mediaType {
	case "movie":
		return map[string]bool{"movie": true}
	case "series":
		return map[string]bool{"tv": true}
	}
	types := map[string]bool{}
	for _, ct := range cfg.ContentTypes {
		switch ct {
		case "movie":
			types["movie"] = true
		case "series":
			types["tv"] = true
		}
	}
	if len(types) == 0 {
		types["movie"], types["tv"] = true, true
	}
	return types
}

// hide drops titles the user's other rows already show (seenKey values), so
// the rows stop repeating each other.
func (f *resultFilter) hide(keys []string) {
	for _, k := range keys {
		f.shown[k] = true
	}
}

// apply returns the results that pass, in order, and a summary of why the
// rest were dropped.
func (f *resultFilter) apply(results []tmdb.SearchResult) ([]tmdb.SearchResult, string) {
	var kept []tmdb.SearchResult
	dropped := map[string]int{}
	for _, r := range results {
		if reason := f.reject(r); reason != "" {
			dropped[reason]++
			continue
		}
		f.seen[seenKey(r)] = true
		kept = append(kept, r)
	}
	return kept, summarize(dropped)
}

// reject returns why a result breaks the rules, or "" if it passes.
func (f *resultFilter) reject(r tmdb.SearchResult) string {
	if !f.types[r.MediaType] {
		return "type"
	}
	if f.seen[seenKey(r)] {
		return "duplicate"
	}
	if f.isBlocked(r) {
		return "watched"
	}
	if f.shown[seenKey(r)] {
		return "in another row"
	}

	if f.rules.YearFrom > 0 || f.rules.YearTo > 0 {
		if r.Year == 0 || (f.rules.YearFrom > 0 && r.Year < f.rules.YearFrom) || (f.rules.YearTo > 0 && r.Year > f.rules.YearTo) {
			return "year"
		}
	}
	if f.rules.MinRating > 0 && r.VoteAverage < f.rules.MinRating {
		return "rating"
	}
	if r.VoteCount >= prefs.QualityFloorMinVotes && r.VoteAverage < f.rules.QualityFloor {
		return "rating"
	}
	if f.rules.MaxVotes > 0 && r.VoteCount > f.rules.MaxVotes {
		return "too well-known"
	}
	if f.rules.MinVotes > 0 && r.VoteCount < f.rules.MinVotes {
		return "too few votes"
	}
	if len(f.langs) > 0 && !f.langs[r.OriginalLanguage] {
		return "language"
	}

	genres := toSet(tmdb.ConfigGenres(r.GenreIDs, r.MediaType))
	if f.rules.Required != "" && !genres[f.rules.Required] {
		return "genre"
	}
	for g := range genres {
		if f.excluded[g] {
			return "genre"
		}
	}
	if len(f.selected) > 0 && !overlaps(genres, f.selected) {
		return "genre"
	}
	return ""
}

// isBlocked reports whether the title is one the user already watched or
// saved. Trakt and TMDB can disagree on the year by one, so allow that.
func (f *resultFilter) isBlocked(r tmdb.SearchResult) bool {
	if f.blockIDs[seenKey(r)] {
		return true
	}
	for _, y := range f.blocked[tmdb.NormTitle(r.Title)] {
		if y == 0 || r.Year == 0 || (y-r.Year <= 1 && r.Year-y <= 1) {
			return true
		}
	}
	return false
}

func seenKey(r tmdb.SearchResult) string {
	return fmt.Sprintf("%s:%d", r.MediaType, r.ID)
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, s := range items {
		set[s] = true
	}
	return set
}

func overlaps(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func summarize(dropped map[string]int) string {
	if len(dropped) == 0 {
		return "none dropped"
	}
	reasons := make([]string, 0, len(dropped))
	for reason, n := range dropped {
		reasons = append(reasons, fmt.Sprintf("%s %d", reason, n))
	}
	sort.Strings(reasons)
	return "dropped: " + strings.Join(reasons, ", ")
}
