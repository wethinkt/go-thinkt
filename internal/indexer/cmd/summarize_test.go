package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/summarize"
)

func TestSummarizeTagsCmdJSONGolden(t *testing.T) {
	origJSON := summarizeTagsJSON
	origSuggest := suggestTagsForInput
	t.Cleanup(func() {
		summarizeTagsJSON = origJSON
		suggestTagsForInput = origSuggest
		summarizeTagsCmd.SetOut(os.Stdout)
	})

	summarizeTagsJSON = true
	suggestTagsForInput = func(context.Context, string) (*summarize.TagSuggestionResult, error) {
		return &summarize.TagSuggestionResult{
			Tags:       []string{"duckdb", "sharing", "sync-gate"},
			Confidence: 0.82,
		}, nil
	}

	var out bytes.Buffer
	summarizeTagsCmd.SetOut(&out)

	if err := summarizeTagsCmd.RunE(summarizeTagsCmd, []string{"Fix the sync gate and suggest sharing tags"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	goldenPath := filepath.Join("testdata", "summarize_tags.golden.json")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if got := out.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}
