package server

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/tmdb"
	"github.com/ngenohkevin/flickmind/internal/trakt"
)

// Watch Again offers titles the user has already watched (Lucent asked for
// rewatches; every other row leaves watched titles out). Deterministic and
// AI-free: the rules below, then a rating-weighted shuffle that changes daily.
const (
	watchAgainMinAge     = 90 * 24 * time.Hour // long enough ago to feel fresh
	watchAgainMinRating  = 7.0
	watchAgainMinVotes   = 100
	watchAgainShowShare  = 0.8 // a show counts once this share of its episodes is watched
	watchAgainMaxLookups = 150 // TMDB detail calls per build
	watchAgainLookupConc = 8
)

// watchAgainExcluded mirrors prefs.alwaysExcluded: TV formats nobody rewatches.
var watchAgainExcluded = map[string]bool{"kids": true, "news": true, "reality": true, "soap": true, "talk": true}

// watchAgainCandidates applies the checks that need no TMDB call: the media
// type, watched at least 90 days ago, not rated 5 or lower on Trakt. At most
// watchAgainMaxLookups, newest first (the order GetWatched returns).
func watchAgainCandidates(watched []trakt.WatchedItem, ratings map[string]int, mediaType string, now time.Time) []trakt.WatchedItem {
	traktType := "movie"
	if mediaType == "series" {
		traktType = "show"
	}
	var out []trakt.WatchedItem
	for _, w := range watched {
		if w.MediaType != traktType || w.TMDBID == 0 {
			continue
		}
		last, err := time.Parse(time.RFC3339, w.LastWatchedAt)
		if err != nil || now.Sub(last) < watchAgainMinAge {
			continue
		}
		if r, ok := ratings[fmt.Sprintf("%s:%d", traktType, w.TMDBID)]; ok && r <= 5 {
			continue
		}
		out = append(out, w)
		if len(out) == watchAgainMaxLookups {
			break
		}
	}
	return out
}

// chooseWatchAgain picks up to want rewatches; details looks a title up on
// TMDB ("movie" or "tv"), nil when unknown. Same day, same order.
func chooseWatchAgain(watched []trakt.WatchedItem, ratings map[string]int, details func(mediaType string, id int) *tmdb.Details,
	mediaType string, now time.Time, want int) []tmdb.SearchResult {
	tmdbType := "movie"
	if mediaType == "series" {
		tmdbType = "tv"
	}
	day := now.UTC()
	seed := int64(day.Year()*10000+int(day.Month())*100+day.Day())*2 + int64(len(tmdbType))
	rng := rand.New(rand.NewSource(seed))

	type cand struct {
		r   tmdb.SearchResult
		key float64
	}
	var cands []cand
	for _, w := range watchAgainCandidates(watched, ratings, mediaType, now) {
		d := details(tmdbType, w.TMDBID)
		if d == nil || d.VoteAverage < watchAgainMinRating || d.VoteCount < watchAgainMinVotes || excludedGenre(d.GenreNames) {
			continue
		}
		if tmdbType == "tv" && (d.NumberOfEpisodes == 0 || float64(w.Plays) < watchAgainShowShare*float64(d.NumberOfEpisodes)) {
			continue
		}
		r := d.SearchResult
		if last, err := time.Parse(time.RFC3339, w.LastWatchedAt); err == nil {
			r.Reason = "You watched this in " + last.Format("January 2006") + "."
		}
		// Efraimidis–Spirakis: higher-rated titles tend to come first, but the order changes daily.
		cands = append(cands, cand{r, math.Pow(rng.Float64(), 1/d.VoteAverage)})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].key > cands[j].key })
	out := make([]tmdb.SearchResult, 0, min(want, len(cands)))
	for _, c := range cands {
		if len(out) == want {
			break
		}
		out = append(out, c.r)
	}
	return out
}

func excludedGenre(names []string) bool {
	for _, n := range names {
		if watchAgainExcluded[strings.ToLower(n)] {
			return true
		}
	}
	return false
}

// getWatchAgain builds the row: Trakt's watched list and ratings, TMDB details
// for the candidates (fetched side by side), then chooseWatchAgain.
func (s *Server) getWatchAgain(ctx context.Context, cfg *store.UserConfig, mediaType string) catalogEntry {
	if !cfg.TraktConnected || s.traktClient == nil {
		return catalogEntry{}
	}
	watched := s.fetchWatched(ctx, cfg)
	ratings := s.fetchRatings(ctx, cfg)
	now := time.Now()

	tmdbType := "movie"
	if mediaType == "series" {
		tmdbType = "tv"
	}
	found := map[int]*tmdb.Details{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, watchAgainLookupConc)
	for _, w := range watchAgainCandidates(watched, ratings, mediaType, now) {
		wg.Add(1)
		slots <- struct{}{}
		go func(id int) {
			defer wg.Done()
			defer func() { <-slots }()
			if d, err := s.tmdbClient.Details(ctx, tmdbType, id); err == nil {
				mu.Lock()
				found[id] = d
				mu.Unlock()
			}
		}(w.TMDBID)
	}
	wg.Wait()

	final := chooseWatchAgain(watched, ratings, func(_ string, id int) *tmdb.Details { return found[id] }, mediaType, now, cfg.MaxResults)
	entry := catalogEntry{Metas: toMetas(final)}
	for _, r := range final {
		entry.Keys = append(entry.Keys, seenKey(r))
	}
	return entry
}
