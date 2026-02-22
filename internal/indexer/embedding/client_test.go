package embedding_test

import (
	"context"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestClient_EmbedBatch_Integration(t *testing.T) {
	// Skip if thinkt-embed-apple not available
	client, err := embedding.NewClient()
	if err != nil {
		t.Skipf("thinkt-embed-apple not available: %v", err)
	}

	items := []embedding.EmbedRequest{
		{ID: "a", Text: "debugging the authentication timeout"},
		{ID: "b", Text: "refactoring the database pool"},
	}

	results, err := client.EmbedBatch(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Dim != 512 {
			t.Fatalf("expected dim=512, got %d", r.Dim)
		}
		if len(r.Embedding) != 512 {
			t.Fatalf("expected 512 floats, got %d", len(r.Embedding))
		}
	}
}

func TestClient_NotFound(t *testing.T) {
	_, err := embedding.NewClientWithBinary("nonexistent-binary-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
