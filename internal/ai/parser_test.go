package ai

import (
	"testing"
)

func TestParseAIResponse_CleanJSONArray(t *testing.T) {
	input := `[{"title":"Inception","year":2010,"type":"movie","reason":"Mind-bending thriller"},{"title":"Breaking Bad","year":2008,"type":"series","reason":"Gripping drama"}]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 2 {
		t.Fatalf("expected 2 recs, got %d", len(recs))
	}
	if recs[0].Title != "Inception" {
		t.Errorf("expected Inception, got %s", recs[0].Title)
	}
	if recs[0].Year != 2010 {
		t.Errorf("expected year 2010, got %d", recs[0].Year)
	}
	if recs[0].Type != "movie" {
		t.Errorf("expected movie, got %s", recs[0].Type)
	}
	if recs[1].Type != "series" {
		t.Errorf("expected series, got %s", recs[1].Type)
	}
}

func TestParseAIResponse_MarkdownCodeBlocks(t *testing.T) {
	input := "```json\n[{\"title\":\"The Matrix\",\"year\":1999,\"type\":\"movie\",\"reason\":\"Sci-fi classic\"}]\n```"
	recs := ParseAIResponse(input, "test")

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Title != "The Matrix" {
		t.Errorf("expected The Matrix, got %s", recs[0].Title)
	}
}

func TestParseAIResponse_WrappedInObject(t *testing.T) {
	input := `{"recommendations":[{"title":"Parasite","year":2019,"type":"movie","reason":"Masterpiece"}]}`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Title != "Parasite" {
		t.Errorf("expected Parasite, got %s", recs[0].Title)
	}
}

func TestParseAIResponse_WrappedInObject_VariousKeys(t *testing.T) {
	keys := []string{"results", "movies", "shows", "data", "items", "suggestions"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			input := `{"` + key + `":[{"title":"Test Movie","year":2020,"type":"movie"}]}`
			recs := ParseAIResponse(input, "test")
			if len(recs) != 1 {
				t.Fatalf("expected 1 rec for key %s, got %d", key, len(recs))
			}
		})
	}
}

func TestParseAIResponse_MixedFieldNames(t *testing.T) {
	input := `[{"Title":"Movie A","Year":2020,"media_type":"movie"},{"name":"Show B","release_year":"2021","Type":"series"}]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 2 {
		t.Fatalf("expected 2 recs, got %d", len(recs))
	}
	if recs[0].Title != "Movie A" {
		t.Errorf("expected Movie A, got %s", recs[0].Title)
	}
	if recs[0].Year != 2020 {
		t.Errorf("expected 2020, got %d", recs[0].Year)
	}
	if recs[1].Title != "Show B" {
		t.Errorf("expected Show B, got %s", recs[1].Title)
	}
	if recs[1].Year != 2021 {
		t.Errorf("expected 2021, got %d", recs[1].Year)
	}
	if recs[1].Type != "series" {
		t.Errorf("expected series, got %s", recs[1].Type)
	}
}

func TestParseAIResponse_TrailingCommas(t *testing.T) {
	input := `[{"title":"Test","year":2020,"type":"movie",},]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Title != "Test" {
		t.Errorf("expected Test, got %s", recs[0].Title)
	}
}

func TestParseAIResponse_TVTypeVariants(t *testing.T) {
	tests := []struct {
		typeStr  string
		expected string
	}{
		{"tv", "series"},
		{"series", "series"},
		{"show", "series"},
		{"tvshow", "series"},
		{"tv_show", "series"},
		{"movie", "movie"},
		{"film", "movie"},
		{"anime", "movie"},
	}

	for _, tt := range tests {
		t.Run(tt.typeStr, func(t *testing.T) {
			input := `[{"title":"Test","year":2020,"type":"` + tt.typeStr + `"}]`
			recs := ParseAIResponse(input, "test")
			if len(recs) != 1 {
				t.Fatalf("expected 1 rec, got %d", len(recs))
			}
			if recs[0].Type != tt.expected {
				t.Errorf("type %q: expected %s, got %s", tt.typeStr, tt.expected, recs[0].Type)
			}
		})
	}
}

func TestParseAIResponse_EmptyInput(t *testing.T) {
	recs := ParseAIResponse("", "test")
	if recs != nil {
		t.Errorf("expected nil for empty input, got %v", recs)
	}
}

func TestParseAIResponse_InvalidJSON(t *testing.T) {
	recs := ParseAIResponse("not json at all", "test")
	if recs != nil {
		t.Errorf("expected nil for invalid input, got %v", recs)
	}
}

func TestParseAIResponse_EmptyArray(t *testing.T) {
	recs := ParseAIResponse("[]", "test")
	if len(recs) != 0 {
		t.Errorf("expected 0 recs for empty array, got %d", len(recs))
	}
}

func TestParseAIResponse_PartialItems_MissingTitle(t *testing.T) {
	input := `[{"title":"Valid","year":2020,"type":"movie"},{"year":2021,"type":"movie"},{"title":"Also Valid","year":2022,"type":"series"}]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 2 {
		t.Fatalf("expected 2 valid recs (item without title skipped), got %d", len(recs))
	}
	if recs[0].Title != "Valid" {
		t.Errorf("expected Valid, got %s", recs[0].Title)
	}
	if recs[1].Title != "Also Valid" {
		t.Errorf("expected Also Valid, got %s", recs[1].Title)
	}
}

func TestParseAIResponse_DefaultReason(t *testing.T) {
	input := `[{"title":"No Reason","year":2020,"type":"movie"}]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Reason != "Recommended based on your preferences" {
		t.Errorf("expected default reason, got %s", recs[0].Reason)
	}
}

func TestParseAIResponse_YearAsString(t *testing.T) {
	input := `[{"title":"Test","year":"2020","type":"movie"}]`
	recs := ParseAIResponse(input, "test")

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Year != 2020 {
		t.Errorf("expected 2020, got %d", recs[0].Year)
	}
}
