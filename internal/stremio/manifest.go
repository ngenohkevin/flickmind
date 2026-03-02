package stremio

import "github.com/ngenohkevin/flickmind/internal/store"

func BuildManifest(baseURL, frontendURL, userID string, cfg *store.UserConfig) Manifest {
	// Determine which Stremio types to show based on user's content type preferences.
	// "anime" maps to both movie and series in Stremio (no native anime type).
	showMovie := false
	showSeries := false
	if cfg != nil && len(cfg.ContentTypes) > 0 {
		for _, ct := range cfg.ContentTypes {
			switch ct {
			case "movie":
				showMovie = true
			case "series":
				showSeries = true
			case "anime", "documentary":
				showMovie = true
				showSeries = true
			}
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
