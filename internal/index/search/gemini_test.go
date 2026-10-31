package search

import (
	"context"
	"path/filepath"
	"testing"
)

const geminiReplayFixture = "../../sources/gemini/testdata/replay.jsonl"

func geminiHits(t *testing.T, query string) []int {
	t.Helper()
	path, err := filepath.Abs(geminiReplayFixture)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher(query, true, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan RawMatch, 16)
	scanFile(context.Background(), Candidate{Path: path, Source: "gemini"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	return lines
}

// A rewound message, a $set line and an earlier copy of a repeated message
// are not in the session the CLI rebuilds; hits there do not count.
func TestSearchGeminiJSONLSkipsDeadLines(t *testing.T) {
	if got := geminiHits(t, "second"); len(got) != 0 {
		t.Errorf("rewound line hit: %v", got)
	}
	if got := geminiHits(t, "third"); len(got) != 1 || got[0] != 9 {
		t.Errorf("live line: %v", got)
	}
	if got := geminiHits(t, "Title"); len(got) != 0 {
		t.Errorf("$set line hit: %v", got)
	}
}

func TestSearchGeminiRepeatHitOnce(t *testing.T) {
	if got := geminiHits(t, "gemini-3-flash-preview"); len(got) != 1 || got[0] != 4 {
		t.Fatalf("want one hit on line 4, got %v", got)
	}
}
