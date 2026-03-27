package summarize

import (
	"strings"
	"testing"
)

func TestBuildClassifyPrompt(t *testing.T) {
	prompt := buildClassifyPrompt("The user wants to refactor the database layer")
	if !strings.Contains(prompt, "The user wants to refactor the database layer") {
		t.Error("prompt should contain the input text")
	}
	if !strings.HasSuffix(prompt, "JSON:") {
		t.Error("prompt should end with JSON: marker")
	}
}

func TestBuildClassifyPromptTruncates(t *testing.T) {
	long := strings.Repeat("x", 7000)
	prompt := buildClassifyPrompt(long)
	if strings.Contains(prompt, strings.Repeat("x", 7000)) {
		t.Error("prompt should truncate long input")
	}
	if !strings.Contains(prompt, "...") {
		t.Error("truncated prompt should contain ellipsis")
	}
}

func TestParseClassifyResponse(t *testing.T) {
	raw := `{"summary":"Found a bug in auth","category":"discovery","entities":["auth.go","JWT"],"relevance":0.8}`
	result, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "Found a bug in auth" {
		t.Errorf("summary = %q, want %q", result.Summary, "Found a bug in auth")
	}
	if result.Category != "discovery" {
		t.Errorf("category = %q, want %q", result.Category, "discovery")
	}
	if len(result.Entities) != 2 {
		t.Errorf("entities len = %d, want 2", len(result.Entities))
	}
	if result.Relevance != 0.8 {
		t.Errorf("relevance = %f, want 0.8", result.Relevance)
	}
}

func TestParseClassifyResponseWithSurroundingText(t *testing.T) {
	raw := `Here is the JSON:
{"summary":"Decided to use PostgreSQL","category":"decision","entities":["PostgreSQL"],"relevance":0.9}
Hope that helps!`
	result, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "Decided to use PostgreSQL" {
		t.Errorf("summary = %q, want %q", result.Summary, "Decided to use PostgreSQL")
	}
	if result.Category != "decision" {
		t.Errorf("category = %q, want %q", result.Category, "decision")
	}
}

func TestParseClassifyResponseInvalidJSON(t *testing.T) {
	raw := "This is not JSON at all"
	result, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "This is not JSON at all" {
		t.Errorf("summary = %q, want raw text", result.Summary)
	}
	if result.Category != "decision" {
		t.Errorf("category = %q, want fallback 'decision'", result.Category)
	}
	if result.Relevance != 0.5 {
		t.Errorf("relevance = %f, want fallback 0.5", result.Relevance)
	}
}

func TestParseClassifyResponseInvalidCategory(t *testing.T) {
	raw := `{"summary":"test","category":"unknown_cat","entities":[],"relevance":0.5}`
	result, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Category != "decision" {
		t.Errorf("category = %q, want fallback 'decision'", result.Category)
	}
}

func TestParseClassifyResponseClampRelevance(t *testing.T) {
	// Test clamping high value
	raw := `{"summary":"test","category":"idea","entities":[],"relevance":5.0}`
	result, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Relevance != 1.0 {
		t.Errorf("relevance = %f, want 1.0 (clamped)", result.Relevance)
	}

	// Test clamping low value
	raw = `{"summary":"test","category":"idea","entities":[],"relevance":-2.0}`
	result, err = parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Relevance != 0.0 {
		t.Errorf("relevance = %f, want 0.0 (clamped)", result.Relevance)
	}
}

func TestBuildSessionPrompt(t *testing.T) {
	prompt := buildSessionPrompt("User asked to fix login bug")
	if !strings.Contains(prompt, "User asked to fix login bug") {
		t.Error("prompt should contain the input context")
	}
	if !strings.HasSuffix(prompt, "Summary:") {
		t.Error("prompt should end with Summary: marker")
	}
}

func TestBuildTagsPrompt(t *testing.T) {
	prompt := buildTagsPrompt("Need to fix sync gate races in the indexer server")
	if !strings.Contains(prompt, "Need to fix sync gate races in the indexer server") {
		t.Error("prompt should contain the input text")
	}
	if !strings.HasSuffix(prompt, "JSON:") {
		t.Error("prompt should end with JSON: marker")
	}
}

func TestParseTagsResponse(t *testing.T) {
	raw := `{"tags":["DuckDB","sync gate","internal/indexer/sync_gate.go","DuckDB"],"confidence":0.9}`
	result, err := parseTagsResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(result.Tags, ","), "duckdb,internal/indexer/sync_gate.go,sync-gate"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
	if result.Confidence != 0.9 {
		t.Fatalf("confidence = %f, want 0.9", result.Confidence)
	}
}

func TestParseTagsResponseWithSurroundingText(t *testing.T) {
	raw := `Suggested tags:
{"tags":["Semantic Search","Embeddings"],"confidence":0.7}
Thanks`
	result, err := parseTagsResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(result.Tags, ","), "embeddings,semantic-search"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
}

func TestParseTagsResponseInvalidJSON(t *testing.T) {
	raw := "duckdb, semantic search, indexer"
	result, err := parseTagsResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(result.Tags, ","), "duckdb,indexer,semantic-search"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
	if result.Confidence != 0.5 {
		t.Fatalf("confidence = %f, want 0.5", result.Confidence)
	}
}

func TestParseTagsResponseClampConfidence(t *testing.T) {
	raw := `{"tags":["duckdb"],"confidence":5.0}`
	result, err := parseTagsResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 1.0 {
		t.Fatalf("confidence = %f, want 1.0", result.Confidence)
	}
}
