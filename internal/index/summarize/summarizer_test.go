package summarize

import (
	"context"
	"os"
	"testing"
)

func TestNewSummarizer(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s (run EnsureModel first)", modelPath)
	}

	s, err := NewSummarizer("", modelPath)
	if err != nil {
		t.Fatalf("NewSummarizer: %v", err)
	}
	defer s.Close()

	if s.ModelID() != DefaultModelID {
		t.Errorf("ModelID() = %q, want %q", s.ModelID(), DefaultModelID)
	}
}

func TestGenerate(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s", modelPath)
	}

	s, err := NewSummarizer("", modelPath)
	if err != nil {
		t.Skipf("NewSummarizer: %v", err)
	}
	defer s.Close()

	output, err := s.Generate(context.Background(), "What is 2+2? Answer with just the number.")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if output == "" {
		t.Fatal("Generate returned empty output")
	}
	t.Logf("Output: %q", output)
}

func TestSummarize(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s", modelPath)
	}

	s, err := NewSummarizer("", modelPath)
	if err != nil {
		t.Skipf("NewSummarizer: %v", err)
	}
	defer s.Close()

	thinkingBlock := `I need to decide between using Redis for caching or an in-memory LRU cache.
Redis adds operational complexity and a network dependency, but supports sharing across instances.
For this single-server setup, an in-memory LRU is simpler and has lower latency.
I'll go with the LRU cache and revisit Redis if we add more servers.`

	result, err := s.Summarize(context.Background(), thinkingBlock)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if result.Summary == "" {
		t.Fatal("Summary is empty")
	}
	if result.Category == "" {
		t.Fatal("Category is empty")
	}
	t.Logf("Summary: %s", result.Summary)
	t.Logf("Category: %s", result.Category)
	t.Logf("Entities: %v", result.Entities)
	t.Logf("Relevance: %f", result.Relevance)
}
