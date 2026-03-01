package stremio

import "github.com/ngenohkevin/flickmind/internal/store"

func BuildManifest(baseURL, frontendURL, userID string, cfg *store.UserConfig) Manifest {
	catalogs := []CatalogDef{
		{Type: "movie", ID: "flickmind-ai-picks", Name: "FlickMind: AI Picks for You"},
		{Type: "series", ID: "flickmind-ai-picks", Name: "FlickMind: AI Picks for You"},
		{Type: "movie", ID: "flickmind-hidden-gems", Name: "FlickMind: Hidden Gems"},
		{Type: "series", ID: "flickmind-hidden-gems", Name: "FlickMind: Hidden Gems"},
	}

	if cfg != nil && cfg.TraktConnected {
		catalogs = append(catalogs,
			CatalogDef{Type: "movie", ID: "flickmind-because-you-watched", Name: "FlickMind: Because You Watched"},
			CatalogDef{Type: "series", ID: "flickmind-because-you-watched", Name: "FlickMind: Because You Watched"},
		)
	}

	return Manifest{
		ID:          "community.flickmind",
		Version:     "1.0.0",
		Name:        "FlickMind",
		Description: "AI-powered movie & TV recommendations. Personalized picks using Groq, DeepSeek, or Gemini — bring your own API key.",
		Logo:        frontendURL + "/icon.svg",
		Resources:   []string{"catalog"},
		Types:       []string{"movie", "series"},
		Catalogs:    catalogs,
		BehaviorHints: &BehaviorHints{
			Configurable:     true,
			ConfigurationURL: frontendURL + "/configure/" + userID,
		},
	}
}
