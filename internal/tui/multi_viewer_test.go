package tui

import (
	"strings"
	"testing"
)

func TestRenderNoSessionContentIncludesErrorsAndHint(t *testing.T) {
	m := MultiViewerModel{
		width: 80,
		loadErrors: []string{
			"/tmp/session-a.jsonl: open failed",
			"/tmp/session-b.jsonl: parse failed",
		},
	}

	got := m.renderNoSessionContent()
	if !strings.Contains(got, "No sessions loaded successfully") {
		t.Fatalf("missing no-session header: %q", got)
	}
	if !strings.Contains(got, "Debug (load errors):") {
		t.Fatalf("missing debug header: %q", got)
	}
	if !strings.Contains(got, "THINKT_LOG_FILE") {
		t.Fatalf("missing log hint: %q", got)
	}
}

func TestHighlightLineMatches(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		query     string
		isCurrent bool
		wantSub   string // substring that must appear in result
		wantClean string // ANSI-stripped result must equal original stripped text
	}{
		{
			name:      "plain text match",
			line:      "hello world",
			query:     "world",
			isCurrent: false,
			wantSub:   "\033[7mworld\033[27m",
		},
		{
			name:      "case insensitive",
			line:      "Hello World",
			query:     "hello",
			isCurrent: false,
			wantSub:   "\033[7mHello\033[27m",
		},
		{
			name:      "current match uses bold",
			line:      "foo bar",
			query:     "bar",
			isCurrent: true,
			wantSub:   "\033[1;7mbar\033[27;22m",
		},
		{
			name:      "with ANSI codes",
			line:      "\033[1mhello\033[0m world",
			query:     "world",
			isCurrent: false,
			wantSub:   "\033[7mworld\033[27m",
		},
		{
			name:      "multiple matches",
			line:      "ab ab ab",
			query:     "ab",
			isCurrent: false,
			wantSub:   "\033[7mab\033[27m \033[7mab\033[27m \033[7mab\033[27m",
		},
		{
			name:  "no match returns unchanged",
			line:  "hello world",
			query: "xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightLineMatches(tt.line, tt.query, tt.isCurrent)
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("expected substring %q in result %q", tt.wantSub, got)
			}
			if tt.wantSub == "" && got != tt.line {
				t.Errorf("expected unchanged line %q, got %q", tt.line, got)
			}
		})
	}
}

func TestTruncateDebugLine(t *testing.T) {
	got := truncateDebugLine("abcdefghijklmnopqrstuvwxyz", 10)
	if got != "abcdefghijklm..." {
		t.Fatalf("unexpected truncation: %q", got)
	}

	short := truncateDebugLine("abc", 10)
	if short != "abc" {
		t.Fatalf("short string should be unchanged, got %q", short)
	}
}
