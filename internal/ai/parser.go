package ai

import (
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"
)

func ParseAIResponse(text, providerName string) []Recommendation {
	cleaned := strings.TrimSpace(text)

	// Step 1: Remove markdown code blocks
	if strings.HasPrefix(cleaned, "```") {
		re := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
		if m := re.FindStringSubmatch(cleaned); len(m) > 1 {
			cleaned = strings.TrimSpace(m[1])
		}
	}

	// Step 2: Extract JSON array from mixed content
	re := regexp.MustCompile(`(?s)\[.*\]`)
	if m := re.FindString(cleaned); m != "" {
		cleaned = m
	}

	// Step 3: Handle JSON wrapped in object
	if strings.HasPrefix(cleaned, "{") {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal([]byte(cleaned), &wrapper); err == nil {
			// Try known wrapper keys first
			for _, key := range []string{"recommendations", "results", "movies", "shows", "data", "items", "output", "suggestions", "titles", "list"} {
				if raw, ok := wrapper[key]; ok {
					var arr []json.RawMessage
					if json.Unmarshal(raw, &arr) == nil {
						cleaned = string(raw)
						break
					}
				}
			}
			// Fallback: find any array value
			if strings.HasPrefix(cleaned, "{") {
				for _, raw := range wrapper {
					var arr []json.RawMessage
					if json.Unmarshal(raw, &arr) == nil {
						cleaned = string(raw)
						break
					}
				}
			}
			// Fallback: collect all object values into an array
			// Handles {"title1": {obj}, "title2": {obj}, ...} format
			if strings.HasPrefix(cleaned, "{") {
				var objects []json.RawMessage
				for _, raw := range wrapper {
					trimmed := strings.TrimSpace(string(raw))
					if strings.HasPrefix(trimmed, "{") {
						objects = append(objects, raw)
					}
				}
				if len(objects) > 0 {
					arrJSON, _ := json.Marshal(objects)
					cleaned = string(arrJSON)
				}
			}
		}
	}

	// Step 4: Parse JSON
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		// Step 5: Try to fix common JSON issues
		fixed := tryFixJSON(cleaned)
		if err2 := json.Unmarshal([]byte(fixed), &parsed); err2 != nil {
			log.Printf("[%s] Failed to parse response: %v", providerName, err)
			if len(text) > 500 {
				log.Printf("[%s] Raw text: %s...", providerName, text[:500])
			}
			return nil
		}
	}

	// Step 6: Normalize each recommendation
	var recs []Recommendation
	for _, item := range parsed {
		if rec := normalizeRecommendation(item); rec != nil {
			recs = append(recs, *rec)
		}
	}

	if len(recs) == 0 && len(parsed) > 0 {
		log.Printf("[%s] Parsed %d items but none were valid", providerName, len(parsed))
	}

	return recs
}

func normalizeRecommendation(item map[string]interface{}) *Recommendation {
	title := firstString(item, "title", "name", "Title", "Name")
	if title == "" {
		return nil
	}

	year := 0
	for _, key := range []string{"year", "Year", "release_year"} {
		if v, ok := item[key]; ok {
			switch val := v.(type) {
			case float64:
				year = int(val)
			case string:
				if n, err := strconv.Atoi(val); err == nil {
					year = n
				}
			}
			if year > 0 {
				break
			}
		}
	}

	mediaType := "movie"
	rawType := strings.ToLower(firstString(item, "type", "Type", "media_type", "mediaType"))
	switch rawType {
	case "tv", "series", "show", "tvshow", "tv_show":
		mediaType = "series"
	case "anime", "animation":
		mediaType = "movie" // will be searched in both
	case "movie", "film":
		mediaType = "movie"
	}

	reason := firstString(item, "reason", "Reason", "description", "explanation")
	if reason == "" {
		reason = "Recommended based on your preferences"
	}

	// Strip parenthetical year from title (e.g., "The Witcher (2019)" → "The Witcher")
	// TMDB searches the title literally, so parentheses cause search failures
	cleanTitle := stripParenYear(title)

	return &Recommendation{
		Title:  strings.TrimSpace(cleanTitle),
		Year:   year,
		Type:   mediaType,
		Reason: strings.TrimSpace(reason),
	}
}

// stripParenYear removes trailing parenthetical year from title, e.g. "Title (2019)" → "Title"
var parenYearRe = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

func stripParenYear(title string) string {
	return parenYearRe.ReplaceAllString(title, "")
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func tryFixJSON(text string) string {
	fixed := text
	// Remove trailing commas before ] or }
	re := regexp.MustCompile(`,\s*([}\]])`)
	fixed = re.ReplaceAllString(fixed, "$1")
	// Remove control characters
	ctrlRe := regexp.MustCompile(`[\x00-\x1f\x7f]`)
	fixed = ctrlRe.ReplaceAllString(fixed, " ")

	// Handle truncated JSON (token limit cut off mid-response)
	// Find the last complete object and close the array
	if strings.HasPrefix(fixed, "[") && !strings.HasSuffix(strings.TrimSpace(fixed), "]") {
		// Find the last complete JSON object (last "}")
		lastBrace := strings.LastIndex(fixed, "}")
		if lastBrace > 0 {
			fixed = fixed[:lastBrace+1] + "]"
		}
	}

	return fixed
}
