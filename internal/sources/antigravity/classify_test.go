package antigravity

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fixtureBytes reads a testdata fixture.
func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fixtureLines splits a testdata fixture into its lines.
func fixtureLines(t *testing.T, name string) [][]byte {
	t.Helper()
	return bytes.Split(bytes.TrimRight(fixtureBytes(t, name), "\n"), []byte("\n"))
}

// fixtureStep returns the classified head of one fixture step.
func fixtureStep(t *testing.T, name string, idx int) stepHead {
	t.Helper()
	h, _ := ClassifyLine(fixtureLines(t, name)[idx])
	return h
}

func TestClassifyLineEveryFixtureStep(t *testing.T) {
	want := []StepClass{ClassUserInput, ClassHarness, ClassModelTurn, ClassToolResult, ClassToolResult,
		ClassSystemMessage, ClassModelTurn, ClassAPIError, ClassModelTurn, ClassToolResult,
		ClassSystemMessage, ClassHarness, ClassCheckpoint, ClassUserInput, ClassModelTurn, ClassUnknown}
	lines := fixtureLines(t, "main.jsonl")
	if len(lines) != len(want) {
		t.Fatalf("got %d lines", len(lines))
	}
	for i, line := range lines {
		h, c := ClassifyLine(line)
		if c != want[i] || h.Index != i {
			t.Errorf("line %d: class %v index %d", i, c, h.Index)
		}
	}
}

func TestClassifyLineEveryResultType(t *testing.T) {
	for _, ty := range []string{"GENERIC", "VIEW_FILE", "RUN_COMMAND", "CODE_ACTION", "GREP_SEARCH",
		"LIST_DIRECTORY", "SEARCH_WEB", "READ_URL_CONTENT", "ASK_QUESTION"} {
		if _, c := ClassifyLine([]byte(`{"step_index":1,"type":"` + ty + `","content":"x"}`)); c != ClassToolResult {
			t.Errorf("%s: %v", ty, c)
		}
	}
}

func TestClassifyLineInvalidJSON(t *testing.T) {
	if _, c := ClassifyLine([]byte(`{"step_index":1,"ty`)); c != ClassInvalid {
		t.Fatalf("got %v", c)
	}
}

func TestClassifyLineDecodesHead(t *testing.T) {
	h := fixtureStep(t, "main.jsonl", 3)
	if h.Type != "VIEW_FILE" || h.Status != "DONE" || h.CreatedAt.IsZero() ||
		len(h.Media) != 1 || h.Media[0].URI != "/w/shot.png" || h.Media[0].MimeType != "image/png" {
		t.Fatalf("%+v", h)
	}
	if h := fixtureStep(t, "main.jsonl", 4); h.Error != "exit 1" || h.Status != "ERROR" {
		t.Fatalf("%+v", h)
	}
}

func TestSearchFilterKeepsConversationLines(t *testing.T) {
	f := NewSearchFilter()
	var kept []int
	for i, line := range fixtureLines(t, "main.jsonl") {
		if f(line) {
			kept = append(kept, i)
		}
	}
	if !slices.Equal(kept, []int{0, 2, 3, 4, 6, 8, 9, 13, 14}) {
		t.Fatalf("kept %v", kept)
	}
}

func TestSearchFilterKeepsSubagentTaskPrompt(t *testing.T) {
	f := NewSearchFilter()
	var kept []int
	for i, line := range fixtureLines(t, "child.jsonl") {
		if f(line) {
			kept = append(kept, i)
		}
	}
	if !slices.Equal(kept, []int{0, 1}) {
		t.Fatalf("kept %v", kept)
	}
}
