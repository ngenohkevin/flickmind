package stremio

type Manifest struct {
	ID          string          `json:"id"`
	Version     string          `json:"version"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Logo        string          `json:"logo,omitempty"`
	Resources   []string        `json:"resources"`
	Types       []string        `json:"types"`
	Catalogs    []CatalogDef    `json:"catalogs"`
	IDPrefixes  []string        `json:"idPrefixes,omitempty"`
	BehaviorHints *BehaviorHints `json:"behaviorHints,omitempty"`
}

type CatalogDef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BehaviorHints struct {
	Adult            bool   `json:"adult,omitempty"`
	Configurable     bool   `json:"configurable,omitempty"`
	ConfigurationURL string `json:"configurationURL,omitempty"`
}

type Meta struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Poster      string   `json:"poster,omitempty"`
	Background  string   `json:"background,omitempty"`
	Description string   `json:"description,omitempty"`
	Year        string   `json:"releaseInfo,omitempty"`
	IMDBRating  string   `json:"imdbRating,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Links       []Link   `json:"links,omitempty"`
}

type Link struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	URL      string `json:"url"`
}

type CatalogResponse struct {
	Metas []Meta `json:"metas"`
}
