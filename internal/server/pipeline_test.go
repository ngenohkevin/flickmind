package server

import (
	"context"
	"testing"

	"github.com/ngenohkevin/flickmind/internal/ai"
)

func TestRoundInput_CarriesRowType(t *testing.T) {
	req := catalogRequest{kind: ai.KindPicks, mediaType: "series"}
	req.input.WatchHistory = []string{"Dark (2017)"}

	in := roundInput(req, 30, []string{"Heat (1995)"})
	if in.MediaType != "series" {
		t.Fatalf("MediaType = %q; the prompt must ask for the row's type", in.MediaType)
	}
	if in.Count != ai.OverRequest(30) || len(in.Exclude) != 1 || len(in.WatchHistory) != 1 {
		t.Errorf("unexpected input: %+v", in)
	}
}

type namedProvider string

func (n namedProvider) Name() string { return string(n) }
func (n namedProvider) GetRecommendations(context.Context, string, string) ([]ai.Recommendation, error) {
	return nil, nil
}

func TestDemoteProvider(t *testing.T) {
	chain := []ai.ProviderEntry{{Provider: namedProvider("groq")}, {Provider: namedProvider("deepseek")}, {Provider: namedProvider("gemini")}}
	got := demoteProvider(chain, "groq")
	var names []string
	for _, p := range got {
		names = append(names, p.Provider.Name())
	}
	if want := "deepseek,gemini,groq"; joinNames(names) != want {
		t.Errorf("order = %s, want %s", joinNames(names), want)
	}
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ","
		}
		out += n
	}
	return out
}
