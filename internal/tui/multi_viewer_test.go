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
