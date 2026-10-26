package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanFileSkipsBookkeepingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"api-request-blob","hash":"h","message":{"role":"user","content":[{"type":"text","text":"needle in a prompt dump"}]}}
{"type":"user","uuid":"u1","message":{"role":"user","content":"needle in a real prompt"}}
{"type":"attachment","uuid":"at1","attachment":{"type":"total_tokens_reminder","text":"needle in a reminder"}}
{"role":"user","content":"needle in a kimi line"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher("needle", false, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan RawMatch, 16)
	scanFile(context.Background(), Candidate{Path: path, Source: "claude"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		if match.Role != "user" {
			t.Errorf("line %d: role %q", match.LineNum, match.Role)
		}
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 2 || lines[0] != 2 || lines[1] != 4 {
		t.Fatalf("want matches on lines 2 and 4 only, got %v", lines)
	}
}

// Secondary entries (reminders, hook output, notices, local command echoes)
// are hidden by default everywhere else; a search hit inside one is noise.
func TestScanFileSkipsSecondaryEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"user","uuid":"u1","isMeta":true,"message":{"role":"user","content":"<system-reminder>needle in a reminder</system-reminder>"}}
{"type":"attachment","uuid":"at1","attachment":{"type":"hook_success","hookName":"x","stdout":"needle in hook output"}}
{"type":"system","subtype":"informational","uuid":"s1","content":"needle in a notice"}
{"type":"user","uuid":"u2","message":{"role":"user","content":"<local-command-stdout>needle in a command echo</local-command-stdout>"}}
{"type":"assistant","uuid":"a1","message":{"id":"m","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"needle in a reply"}]}}
{"type":"system","subtype":"away_summary","uuid":"s2","content":"needle in an away summary"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher("needle", false, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan RawMatch, 16)
	scanFile(context.Background(), Candidate{Path: path, Source: "claude"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 2 || lines[0] != 5 || lines[1] != 6 {
		t.Fatalf("want matches on lines 5 and 6 only, got %v", lines)
	}
}
