package db_test

import (
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

func TestEmbeddingsTableExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Verify embeddings table exists
	var count int
	err = d.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_name = 'embeddings'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected embeddings table to exist, got count=%d", count)
	}
}

func TestInsertAndQueryEmbedding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert a test embedding (1024 floats)
	embedding := make([]float32, 1024)
	for i := range embedding {
		embedding[i] = float32(i) / 1024.0
	}

	_, err = d.Exec(`
		INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[1024], ?)`,
		"entry1_0", "sess1", "entry1", 0, "qwen3-embedding-0.6b", 1024, embedding, "abc123",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Query it back
	var id, sessionID string
	err = d.QueryRow("SELECT id, session_id FROM embeddings WHERE id = ?", "entry1_0").Scan(&id, &sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if id != "entry1_0" || sessionID != "sess1" {
		t.Fatalf("unexpected values: id=%s session_id=%s", id, sessionID)
	}
}
