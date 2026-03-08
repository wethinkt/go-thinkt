package summarize

import (
	"encoding/json"
	"fmt"
	"strings"
)

const classifyPromptTemplate = `You are a thinking-block classifier. Given an AI assistant's internal reasoning, produce a JSON object with these fields:
- "summary": 1-3 sentence summary of the key insight or decision
- "category": exactly one of: idea, discovery, concern, decision, pattern, rejected
- "entities": array of key technologies, files, or concepts mentioned (max 5)
- "relevance": float 0.0-1.0 rating how interesting this is to a human developer

Categories:
- idea: new approach or feature considered
- discovery: finding about the codebase, API, or behavior
- concern: risk, issue, or warning identified
- decision: choice made between alternatives
- pattern: recurring theme or trend observed
- rejected: approach considered and explicitly discarded

Respond with ONLY the JSON object, no other text.

Thinking block:
%s

JSON:`

const sessionPromptTemplate = `Summarize this AI coding session in 2-3 sentences. Focus on what was accomplished, key decisions made, and any open issues. Be concrete and specific.

Session context:
%s

Summary:`

func buildClassifyPrompt(thinkingText string) string {
	if len(thinkingText) > 6000 {
		thinkingText = thinkingText[:6000] + "..."
	}
	return fmt.Sprintf(classifyPromptTemplate, thinkingText)
}

func buildSessionPrompt(sessionContext string) string {
	if len(sessionContext) > 8000 {
		sessionContext = sessionContext[:8000] + "..."
	}
	return fmt.Sprintf(sessionPromptTemplate, sessionContext)
}

// parseClassifyResponse extracts a SummaryResult from the model's JSON output.
func parseClassifyResponse(raw string) (*SummaryResult, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return &SummaryResult{
			Summary:   strings.TrimSpace(raw),
			Category:  "decision",
			Relevance: 0.5,
		}, nil
	}

	jsonStr := raw[start : end+1]

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
