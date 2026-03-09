package summarize

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

//go:embed prompts/classify.txt
var classifyPromptTemplate string

//go:embed prompts/tags.txt
var tagsPromptTemplate string

//go:embed prompts/session.txt
var sessionPromptTemplate string

func buildClassifyPrompt(thinkingText string) string {
	if len(thinkingText) > 6000 {
		thinkingText = thinkingText[:6000] + "..."
	}
	return fmt.Sprintf(strings.TrimRight(classifyPromptTemplate, "\n"), thinkingText)
}

func buildSessionPrompt(sessionContext string) string {
	if len(sessionContext) > 8000 {
		sessionContext = sessionContext[:8000] + "..."
	}
	return fmt.Sprintf(strings.TrimRight(sessionPromptTemplate, "\n"), sessionContext)
}

func buildTagsPrompt(thinkingText string) string {
	if len(thinkingText) > 6000 {
		thinkingText = thinkingText[:6000] + "..."
	}
	return fmt.Sprintf(strings.TrimRight(tagsPromptTemplate, "\n"), thinkingText)
}

// parseClassifyResponse extracts a SummaryResult from the model's JSON output.
func parseClassifyResponse(raw string) (*SummaryResult, error) {
	jsonStr, ok := extractJSONObject(raw)
	if !ok {
		return &SummaryResult{
			Summary:   strings.TrimSpace(raw),
			Category:  "decision",
			Relevance: 0.5,
		}, nil
	}

	var parsed struct {
		Summary   string   `json:"summary"`
		Category  string   `json:"category"`
		Entities  []string `json:"entities"`
		Relevance float64  `json:"relevance"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &SummaryResult{
			Summary:   strings.TrimSpace(raw),
			Category:  "decision",
			Relevance: 0.5,
		}, nil
	}

	validCategories := map[string]bool{
		"idea": true, "discovery": true, "concern": true,
		"decision": true, "pattern": true, "rejected": true,
	}
	if !validCategories[parsed.Category] {
		parsed.Category = "decision"
	}

	if parsed.Relevance < 0 {
		parsed.Relevance = 0
	}
	if parsed.Relevance > 1 {
		parsed.Relevance = 1
	}

	return &SummaryResult{
		Summary:   parsed.Summary,
		Category:  parsed.Category,
		Entities:  parsed.Entities,
		Relevance: parsed.Relevance,
	}, nil
}

// parseTagsResponse extracts tag suggestions from the model's JSON output.
func parseTagsResponse(raw string) (*TagSuggestionResult, error) {
	jsonStr, ok := extractJSONObject(raw)
	if !ok {
		return &TagSuggestionResult{
			Tags:       normalizeTags(splitRawTags(raw)),
			Confidence: 0.5,
		}, nil
	}

	var parsed struct {
		Tags       []string `json:"tags"`
		Confidence float64  `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &TagSuggestionResult{
			Tags:       normalizeTags(splitRawTags(raw)),
			Confidence: 0.5,
		}, nil
	}

	if parsed.Confidence < 0 {
		parsed.Confidence = 0
	}
	if parsed.Confidence > 1 {
		parsed.Confidence = 1
	}

	return &TagSuggestionResult{
		Tags:       normalizeTags(parsed.Tags),
		Confidence: parsed.Confidence,
	}, nil
}

func extractJSONObject(raw string) (string, bool) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return "", false
	}
	return raw[start : end+1], true
}

func splitRawTags(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '\t', ';':
			return true
		default:
			return false
		}
	})
	return fields
}

var nonTagChars = regexp.MustCompile(`[^a-z0-9._/\-]+`)

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		tag = strings.ReplaceAll(tag, " ", "-")
		tag = nonTagChars.ReplaceAllString(tag, "-")
		tag = strings.Trim(tag, "-./_")
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	slices.Sort(out)
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}
