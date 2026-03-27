package summarize

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSummarizeThinkingBlock(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s", modelPath)
	}

	s, err := NewSummarizer("", modelPath)
	if err != nil {
		t.Skipf("NewSummarizer: %v", err)
	}
	defer s.Close()

	entry := thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: `I need to decide how to implement rate limiting for the API.
There are three main approaches:
1. Token bucket - simple, allows bursts, easy to implement with golang.org/x/time/rate
2. Sliding window - more precise, prevents edge-case bursts at window boundaries
3. Adaptive rate limiting - adjusts based on server load, but adds complexity

For our use case (protecting a REST API with ~100 req/min per key), token bucket
is the right choice. It's well-understood, the stdlib has good support, and the
slight imprecision at window edges doesn't matter for our scale.

I'll use golang.org/x/time/rate with a limit of 100 and burst of 10.`},
			{Type: "text", Text: "I'll implement rate limiting using token bucket."},
		},
	}

	thinkingText := ExtractThinkingText(entry)
	if thinkingText == "" {
		t.Fatal("expected thinking text")
	}

	result, err := s.Summarize(context.Background(), thinkingText)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if result.Category == "" {
		t.Error("expected non-empty category")
	}

	validCategories := map[string]bool{
		"idea": true, "discovery": true, "concern": true,
		"decision": true, "pattern": true, "rejected": true,
	}
	if !validCategories[result.Category] {
		t.Errorf("unexpected category %q", result.Category)
	}

	t.Logf("Summary: %s", result.Summary)
	t.Logf("Category: %s", result.Category)
	t.Logf("Entities: %v", result.Entities)
	t.Logf("Relevance: %.2f", result.Relevance)
}

func TestSummarizeSession(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s", modelPath)
	}

	s, err := NewSummarizer("", modelPath)
	if err != nil {
		t.Skipf("NewSummarizer: %v", err)
	}
	defer s.Close()

	entries := []thinkt.Entry{
		{Role: thinkt.RoleUser, Text: "Add rate limiting to the API endpoints"},
		{Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: strings.Repeat("Considering token bucket vs sliding window for rate limiting. ", 5)},
			{Type: "text", Text: "I'll implement token bucket rate limiting."},
		}},
		{Role: thinkt.RoleUser, Text: "Looks good, now add tests"},
		{Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "Added unit tests for the rate limiter middleware."},
		}},
	}

	sessionCtx := ExtractSessionContext(entries)
	if sessionCtx == "" {
		t.Fatal("expected session context")
	}

	result, err := s.SummarizeSession(context.Background(), sessionCtx)
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}

	if result.Summary == "" {
		t.Error("expected non-empty session summary")
	}

	t.Logf("Session summary: %s", result.Summary)
}
