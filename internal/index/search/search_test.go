package search

import (
	"testing"
)

func TestNewMatcher(t *testing.T) {
	m, err := NewMatcher("hello", false, false)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	if !m.Match("Hello World") {
		t.Fatal("expected case-insensitive match")
	}
	if m.Match("goodbye") {
		t.Fatal("unexpected match")
	}
}

func TestNewMatcherCaseSensitive(t *testing.T) {
	m, err := NewMatcher("Hello", true, false)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	if !m.Match("Hello World") {
		t.Fatal("expected match")
	}
	if m.Match("hello world") {
		t.Fatal("unexpected case-insensitive match")
	}
}

func TestNewMatcherRegex(t *testing.T) {
	m, err := NewMatcher(`\d+`, false, true)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	if !m.Match("abc123") {
		t.Fatal("expected regex match")
	}
	if m.Match("abc") {
		t.Fatal("unexpected match")
	}
}

func TestFindIndex(t *testing.T) {
	m, _ := NewMatcher("world", false, false)
	start, end := m.FindIndex("hello world")
	if start != 6 || end != 11 {
		t.Fatalf("FindIndex = (%d, %d), want (6, 11)", start, end)
	}

	start, end = m.FindIndex("no match")
	if start != -1 || end != -1 {
		t.Fatalf("FindIndex = (%d, %d), want (-1, -1)", start, end)
	}
}

func TestExtractPreview(t *testing.T) {
	m, _ := NewMatcher("needle", false, false)

	// Short line — no truncation.
	preview, s, e := extractPreview("find the needle here", m)
	if preview != "find the needle here" {
		t.Fatalf("preview = %q", preview)
	}
	if s != 9 || e != 15 {
		t.Fatalf("match = (%d, %d), want (9, 15)", s, e)
	}

	// No match.
	preview, _, _ = extractPreview("no match here", m)
	if preview != "" {
		t.Fatalf("expected empty preview for no match, got %q", preview)
	}
}

func TestDefaultSearchOptions(t *testing.T) {
	opts := DefaultSearchOptions()
	if opts.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", opts.Limit)
	}
	if opts.LimitPerSession != 2 {
		t.Fatalf("LimitPerSession = %d, want 2", opts.LimitPerSession)
	}
}

func TestShortenPath(t *testing.T) {
	// Just verify it doesn't panic on non-home paths.
	result := ShortenPath("/tmp/test")
	if result != "/tmp/test" {
		t.Fatalf("ShortenPath = %q, want /tmp/test", result)
	}
}
