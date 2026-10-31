package gemini

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var sampleTokens = &Tokens{Input: 10259, Output: 257, Cached: 7000, Thoughts: 423, Tool: 5, Total: 10939}

var wantUsage = thinkt.TokenUsage{InputTokens: 10259, CacheReadInputTokens: 7000, OutputTokens: 680, ThinkingTokens: 423}

func TestUsageFromTokens(t *testing.T) {
	if u := usageFromTokens(sampleTokens); u == nil || *u != wantUsage {
		t.Fatalf("usage = %+v", u)
	}
	if usageFromTokens(nil) != nil {
		t.Fatal("nil tokens should give nil usage")
	}
}

func TestJSONSessionUsageKeepsThinking(t *testing.T) {
	sess := &Session{Messages: []Message{{ID: "g1", Type: "gemini", Content: "hi", Tokens: sampleTokens}}}
	got := convertSession(sess, &thinkt.SessionMeta{})
	if len(got.Entries) != 1 || got.Entries[0].Usage == nil || *got.Entries[0].Usage != wantUsage {
		t.Fatalf("entries = %+v", got.Entries)
	}
}
