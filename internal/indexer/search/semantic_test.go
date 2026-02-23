package search_test

import (
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

func TestSemanticSearch_NoResults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	svc := search.NewService(d)
	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: make([]float32, 1024),
		Model:          "qwen3-embedding-0.6b",
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSemanticSearch_FindsSimilar(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert two embeddings: one similar to query, one not
	similar := make([]float32, 1024)
	similar[0] = 1.0 // pointing in one direction

	different := make([]float32, 1024)
	different[1] = 1.0 // pointing in orthogonal direction

	for _, tc := range []struct {
		id, sessID, entryUUID string
		emb                   []float32
	}{
		{"e1_0", "s1", "e1", similar},
		{"e2_0", "s2", "e2", different},
	} {
		_, err := d.Exec(`
			INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
			VALUES (?, ?, ?, 0, 'qwen3-embedding-0.6b', 1024, ?::FLOAT[1024], 'hash')`,
			tc.id, tc.sessID, tc.entryUUID, tc.emb)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Query with vector similar to "similar"
	query := make([]float32, 1024)
	query[0] = 1.0

	svc := search.NewService(d)
	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: query,
		Model:          "qwen3-embedding-0.6b",
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	// First result should be the similar one
	if results[0].SessionID != "s1" {
		t.Fatalf("expected s1 first, got %s", results[0].SessionID)
	}
}
