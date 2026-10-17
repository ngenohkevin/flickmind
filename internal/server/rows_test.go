package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngenohkevin/flickmind/internal/store"
	"github.com/ngenohkevin/flickmind/internal/stremio"
)

func rowEntry(names ...string) catalogEntry {
	var e catalogEntry
	for i, n := range names {
		e.Metas = append(e.Metas, stremio.Meta{ID: n, Type: "movie", Name: n})
		e.Keys = append(e.Keys, "movie:"+string(rune('a'+i)))
	}
	return e
}

func getCatalog(t *testing.T, srv *Server, path string) ([]string, time.Duration) {
	t.Helper()
	start := time.Now()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	srv.router.ServeHTTP(w, req)
	took := time.Since(start)
	// Errorf, not Fatalf: some tests call this from goroutines
	if w.Code != 200 {
		t.Errorf("status %d", w.Code)
		return nil, took
	}
	var resp stremio.CatalogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Error(err)
		return nil, took
	}
	var names []string
	for _, m := range resp.Metas {
		names = append(names, m.Name)
	}
	return names, took
}

func userCfg(t *testing.T, ms *mockStore) *store.UserConfig {
	t.Helper()
	cfg, err := ms.GetUserConfig(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	store.NormalizeConfig(cfg)
	return cfg
}

func TestProviderChain_DeepSeekFirst(t *testing.T) {
	// DeepSeek's suggestions exist on TMDB 75-100% of the time against
	// Groq's 45-68%, and Groq's free tier runs out of tokens per minute.
	srv := newTestServer(newMockStore())
	chain := srv.buildProviderChain(&store.UserConfig{GroqKey: "g", DeepSeekKey: "d", GeminiKey: "m"})
	var names []string
	for _, p := range chain {
		names = append(names, p.Provider.Name())
	}
	if got := joinNames(names); got != "deepseek,groq,gemini" {
		t.Errorf("order = %s", got)
	}
}

func TestCatalog_ServesLastListWhileRebuilding(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)

	release := make(chan struct{})
	var builds atomic.Int32
	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		builds.Add(1)
		<-release
		return rowEntry("New Pick")
	}
	key := rowKey(userCfg(t, ms), "flickmind-ai-picks", "movie")
	srv.cache.Set(lastKey(key), rowEntry("Old Pick"), time.Hour)

	names, took := getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json")
	if len(names) != 1 || names[0] != "Old Pick" || took > time.Second {
		t.Fatalf("got %v in %v; want the last list at once", names, took)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		var fresh catalogEntry
		if srv.cache.Get(key, &fresh) && len(fresh.Metas) == 1 && fresh.Metas[0].Name == "New Pick" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the rebuild was not cached")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if names, _ := getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json"); names[0] != "New Pick" {
		t.Errorf("after the rebuild got %v", names)
	}
	if n := builds.Load(); n != 1 {
		t.Errorf("builds = %d, want 1", n)
	}
}

func TestCatalog_ColdRowWaitsForOneSharedBuild(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)

	var builds atomic.Int32
	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		builds.Add(1)
		time.Sleep(100 * time.Millisecond)
		return rowEntry("Pick")
	}
	done := make(chan []string, 3)
	for i := 0; i < 3; i++ {
		go func() {
			names, _ := getCatalog(t, srv, "/u1/catalog/movie/flickmind-hidden-gems.json")
			done <- names
		}()
	}
	for i := 0; i < 3; i++ {
		if names := <-done; len(names) != 1 || names[0] != "Pick" {
			t.Errorf("request %d got %v", i, names)
		}
	}
	if n := builds.Load(); n != 1 {
		t.Errorf("builds = %d, want 1 shared build", n)
	}
}

func TestCatalog_BuildOutlivesAClientThatGivesUp(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)

	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		time.Sleep(150 * time.Millisecond)
		return rowEntry("Pick")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "/u1/catalog/series/flickmind-ai-picks.json", nil)
	srv.router.ServeHTTP(httptest.NewRecorder(), req)

	key := rowKey(userCfg(t, ms), "flickmind-ai-picks", "series")
	deadline := time.Now().Add(2 * time.Second)
	for {
		var e catalogEntry
		if srv.cache.Get(key, &e) && len(e.Metas) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a cancelled request threw the build away")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCatalog_NewSettingsNeverServeTheOldList(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)
	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		return rowEntry("Built With New Settings")
	}
	old := userCfg(t, ms)
	srv.cache.Set(lastKey(rowKey(old, "flickmind-ai-picks", "movie")), rowEntry("Old Settings"), time.Hour)

	changed := *old
	changed.Genres = []string{"Horror"}
	_ = ms.SaveUserConfig(context.Background(), &changed)

	if names, _ := getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json"); names[0] != "Built With New Settings" {
		t.Errorf("got %v", names)
	}
}

func TestCatalog_EmptyRebuildKeepsTheLastListWithoutRetryingEachTime(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)
	var builds atomic.Int32
	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		builds.Add(1)
		return catalogEntry{} // providers down
	}
	key := rowKey(userCfg(t, ms), "flickmind-ai-picks", "movie")
	srv.cache.Set(lastKey(key), rowEntry("Last Good"), time.Hour)

	getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json")
	deadline := time.Now().Add(2 * time.Second)
	for builds.Load() == 0 || !srv.cache.Get(key, &catalogEntry{}) {
		if time.Now().After(deadline) {
			t.Fatal("the last list was not held after an empty build")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if names, _ := getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json"); names[0] != "Last Good" {
		t.Errorf("got %v", names)
	}
	if n := builds.Load(); n != 1 {
		t.Errorf("builds = %d; an empty build should not be retried on the next request", n)
	}
}

func TestCatalog_RowsSkipEachOthersTitles(t *testing.T) {
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)
	cfg := userCfg(t, ms)

	srv.cache.Set(lastKey(rowKey(cfg, "flickmind-hidden-gems", "movie")), catalogEntry{Keys: []string{"movie:1"}}, time.Hour)
	srv.cache.Set(lastKey(rowKey(cfg, "flickmind-ai-picks", "series")), catalogEntry{Keys: []string{"tv:2"}}, time.Hour)
	srv.cache.Set(lastKey(rowKey(cfg, "flickmind-ai-picks", "movie")), catalogEntry{Keys: []string{"movie:3"}}, time.Hour)

	got := map[string]bool{}
	for _, k := range srv.otherRowKeys(cfg, "flickmind-ai-picks", "movie") {
		got[k] = true
	}
	if !got["movie:1"] || !got["tv:2"] || got["movie:3"] {
		t.Errorf("other rows' keys = %v; want the other rows' titles, not this row's own", got)
	}
}

func TestCatalog_AUsersRowsBuildOneAtATime(t *testing.T) {
	// Rows built side by side can't see each other's new titles, so two
	// rows refreshed together repeated 15 titles.
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)

	var running, peak atomic.Int32
	srv.rowBuilder = func(_ context.Context, _ *store.UserConfig, _, _ string) catalogEntry {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return rowEntry("Pick")
	}
	done := make(chan struct{})
	for _, path := range []string{
		"/u1/catalog/movie/flickmind-ai-picks.json",
		"/u1/catalog/movie/flickmind-hidden-gems.json",
		"/u1/catalog/series/flickmind-ai-picks.json",
	} {
		go func() {
			getCatalog(t, srv, path)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 3; i++ {
		<-done
	}
	if p := peak.Load(); p != 1 {
		t.Errorf("peak concurrent builds = %d, want 1", p)
	}
}

func TestCatalog_NextRowSeesThePreviousRowsNewList(t *testing.T) {
	// The build slot used to be released before the row was saved, so the
	// next row read the previous row's old list and they shared 4 titles.
	ms := newMockStore()
	_ = ms.CreateUser(context.Background(), "u1")
	srv := newTestServer(ms)
	cfg := userCfg(t, ms)

	seenByGems := make(chan []string, 1)
	srv.rowBuilder = func(_ context.Context, c *store.UserConfig, catalogID, mediaType string) catalogEntry {
		if catalogID == "flickmind-hidden-gems" {
			seenByGems <- srv.otherRowKeys(c, catalogID, mediaType)
			return rowEntry("Gem")
		}
		time.Sleep(30 * time.Millisecond)
		return catalogEntry{Metas: []stremio.Meta{{ID: "tt1", Name: "Pick"}}, Keys: []string{"movie:42"}}
	}
	go getCatalog(t, srv, "/u1/catalog/movie/flickmind-ai-picks.json")
	time.Sleep(10 * time.Millisecond) // AI Picks holds the slot first
	getCatalog(t, srv, "/u1/catalog/movie/flickmind-hidden-gems.json")

	keys := <-seenByGems
	found := false
	for _, k := range keys {
		found = found || k == "movie:42"
	}
	if !found {
		t.Errorf("Hidden Gems saw %v; want AI Picks' new list (movie:42)", keys)
	}
	_ = cfg
}
