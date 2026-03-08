package summarize

// SummaryResult holds the output of a Summarize call.
type SummaryResult struct {
	Summary   string   // 2-3 sentence summary
	Category  string   // idea|discovery|concern|decision|pattern|rejected
	Entities  []string // key entities mentioned
	Relevance float64  // 0.0-1.0 relevance to a human reader
}

// SessionSummaryResult holds the output of a SummarizeSession call.
type SessionSummaryResult struct {
	Summary string // session-level summary
}
