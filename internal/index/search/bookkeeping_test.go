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
