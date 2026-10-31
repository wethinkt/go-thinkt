package tui

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// The web selects a project by its id, which for Claude is the storage folder.
func TestWebProjectID(t *testing.T) {
	claude := thinkt.SessionMeta{Source: thinkt.SourceClaude, ProjectPath: "/real/proj", FullPath: "/h/projects/-real-proj/s.jsonl"}
	if got := webProjectID(claude); got != "/h/projects/-real-proj" {
		t.Fatalf("%q", got)
	}
	codex := thinkt.SessionMeta{Source: thinkt.SourceCodex, ProjectPath: "/real/proj", FullPath: "/h/sessions/s.jsonl"}
	if got := webProjectID(codex); got != "/real/proj" {
		t.Fatalf("%q", got)
	}
}
