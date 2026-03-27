package summarize

// SummaryResult holds the output of a Summarize call.
type SummaryResult struct {
	Summary   string   `json:"summary"`   // 2-3 sentence summary
	Category  string   `json:"category"`  // idea|discovery|concern|decision|pattern|rejected
	Entities  []string `json:"entities"`  // key entities mentioned
	Relevance float64  `json:"relevance"` // 0.0-1.0 relevance to a human reader
}

// TagSuggestionResult holds the output of a SuggestTags call.
type TagSuggestionResult struct {
	Tags       []string `json:"tags"`       // suggested tags, normalized for sharing/search UI
	Confidence float64  `json:"confidence"` // 0.0-1.0 confidence in the suggestions
}

// SessionSummaryResult holds the output of a SummarizeSession call.
type SessionSummaryResult struct {
	Summary string `json:"summary"` // session-level summary
}
