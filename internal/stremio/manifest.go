package stremio

import "github.com/ngenohkevin/flickmind/internal/store"

func BuildManifest(baseURL, frontendURL, userID string, cfg *store.UserConfig) Manifest {
	// Determine which Stremio types to show based on user's content type preferences.
	// Only explicit "movie"/"series" selections control the catalog types.
	// "anime" and "documentary" are content focus areas — they affect AI prompts,
	// not which Stremio catalog types appear.
	showMovie := false
	showSeries := false
	hasExplicitType := false
	if cfg != nil && len(cfg.ContentTypes) > 0 {
		for _, ct := range cfg.ContentTypes {
			switch ct {
			case "movie":
				showMovie = true
				hasExplicitType = true
			case "series":
				showSeries = true
				hasExplicitType = true
			}
		}
		// If only anime/documentary selected (no explicit movie/series), default to both
		if !hasExplicitType {
			showMovie = true
			showSeries = true
		}
	} else {
		showMovie = true
		showSeries = true
	}

	var types []string
	if showMovie {
		types = append(types, "movie")
	}
	if showSeries {
		types = append(types, "series")
	}

	catalogIDs := []struct {
		id   string
		name string
	}{
		{"flickmind-ai-picks", "FlickMind: AI Picks for You"},
		{"flickmind-hidden-gems", "FlickMind: Hidden Gems"},
	}

	var catalogs []CatalogDef
	for _, cat := range catalogIDs {
		for _, t := range types {
			catalogs = append(catalogs, CatalogDef{Type: t, ID: cat.id, Name: cat.name})
		}
	}

	// Add dedicated focus catalogs (single row each, mixed movie+series)
	if cfg != nil {
		for _, ct := range cfg.ContentTypes {
			if ct == "anime" {
				catalogs = append(catalogs, CatalogDef{Type: "movie", ID: "flickmind-anime", Name: "FlickMind: Anime"})
			}
			if ct == "documentary" {
				catalogs = append(catalogs, CatalogDef{Type: "movie", ID: "flickmind-documentary", Name: "FlickMind: Documentaries"})
			}
		}
	}

	if cfg != nil && cfg.TraktConnected {
		for _, t := range types {
			catalogs = append(catalogs, CatalogDef{Type: t, ID: "flickmind-because-you-watched", Name: "FlickMind: Because You Watched"})
		}
	}

	return Manifest{
		ID:          "community.flickmind",
		Version:     "1.0.0",
		Name:        "FlickMind",
		Description: "AI-powered movie & TV recommendations. Personalized picks using Groq, DeepSeek, or Gemini — bring your own API key.",
		Logo:        frontendURL + "/apple-touch-icon.png",
		Resources:   []string{"catalog"},
		Types:       types,
		Catalogs:    catalogs,
		BehaviorHints: &BehaviorHints{
			Configurable:     true,
			ConfigurationURL: frontendURL + "/configure/" + userID,
		},
	}
}
