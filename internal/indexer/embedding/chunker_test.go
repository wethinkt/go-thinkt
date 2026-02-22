package embedding_test

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestChunkText_Short(t *testing.T) {
	chunks := embedding.ChunkText("hello world", 2000, 200)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello world" {
		t.Fatalf("unexpected chunk: %q", chunks[0])
	}
}

func TestChunkText_Long(t *testing.T) {
	// Create a 5000-char string
	text := ""
	for i := 0; i < 250; i++ {
		text += "twenty char string. "
	}
	chunks := embedding.ChunkText(text, 2000, 200)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks for 5000 chars, got %d", len(chunks))
	}
	// Verify overlap: end of chunk[0] should appear at start of chunk[1]
	overlap := chunks[0][len(chunks[0])-200:]
	if chunks[1][:200] != overlap {
		t.Fatal("expected 200-char overlap between chunks")
	}
}

func TestChunkText_Empty(t *testing.T) {
	chunks := embedding.ChunkText("", 2000, 200)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}
