package db

import (
	"os"
	"path/filepath"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func TestOpenEmbeddings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.db")

	db, err := OpenEmbeddings(path, 768)
	if err != nil {
		t.Fatalf("OpenEmbeddings: %v", err)
	}
	defer db.Close()

	// Verify vec0 table exists by inserting and querying.
	// Use a non-zero vector; cosine distance of a zero vector is undefined (NULL).
	floats := make([]float32, 768)
	floats[0] = 1.0
	vec, err := sqlite_vec.SerializeFloat32(floats)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		"INSERT INTO vec_embeddings(embedding_id, embedding, session_id, tier, model) VALUES (?, ?, ?, ?, ?)",
		1, vec, "sess-1", "conversation", "test-model",
	)
	if err != nil {
		t.Fatalf("insert into vec_embeddings: %v", err)
	}

	// Verify embedding_meta table exists
	_, err = db.Exec(
		"INSERT INTO embedding_meta(embedding_id, entry_uuid, chunk_index, text_hash) VALUES (?, ?, ?, ?)",
		1, "entry-1", 0, "abc123",
	)
	if err != nil {
		t.Fatalf("insert into embedding_meta: %v", err)
	}

	// Verify KNN query works
	rows, err := db.Query(
		"SELECT embedding_id, distance FROM vec_embeddings WHERE embedding MATCH ? AND k = 1",
		vec,
	)
	if err != nil {
		t.Fatalf("KNN query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected 1 row from KNN query")
	}
	var id int64
	var dist float64
	if err := rows.Scan(&id, &dist); err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("expected embedding_id=1, got %d", id)
	}

	// Verify file exists on disk
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestOpenEmbeddingsPrunable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.db")

	db, err := OpenEmbeddings(path, 768)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Pruning: just delete the file
	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove: %v", err)
	}

	// Re-create from scratch
	db2, err := OpenEmbeddings(path, 768)
	if err != nil {
		t.Fatalf("re-create after prune: %v", err)
	}
	db2.Close()
}
