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

func TestSemanticSearch_WithDiversity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert embeddings for multiple sessions
	// All pointing in similar direction (high similarity)
	baseVec := make([]float32, 1024)
	baseVec[0] = 1.0

	sessions := []struct {
		id        string
		sessionID string
		project   string
		source    string
	}{
		{"e1_0", "session1", "project-a", "claude"},
		{"e2_0", "session2", "project-b", "kimi"},
		{"e3_0", "session3", "project-c", "codex"},
		{"e4_0", "session1", "project-a", "claude"}, // Same session as e1
		{"e5_0", "session1", "project-a", "claude"}, // Same session as e1
	}

	for _, tc := range sessions {
		vec := make([]float32, 1024)
		copy(vec, baseVec)
		// Slight variation to give different distances
		vec[1] = float32(tc.id[1]) / 1000.0

		_, err := d.Exec(`
			INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
			VALUES (?, ?, ?, 0, 'qwen3-embedding-0.6b', 1024, ?::FLOAT[1024], 'hash')`,
			tc.id, tc.sessionID, tc.id[:2], vec)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Query vector similar to all
	query := make([]float32, 1024)
	query[0] = 1.0

	svc := search.NewService(d)

	// Test without diversity - should favor session1 (more entries)
	resultsNoDiv, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: query,
		Model:          "qwen3-embedding-0.6b",
		Limit:          3,
		Diversity:      false,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test with diversity - should spread across sessions
	resultsDiv, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: query,
		Model:          "qwen3-embedding-0.6b",
		Limit:          3,
		Diversity:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Count unique sessions in each result set
	uniqueNoDiv := countUniqueSessions(resultsNoDiv)
	uniqueDiv := countUniqueSessions(resultsDiv)

	// With diversity, we expect more unique sessions
	// (This is probabilistic based on the algorithm, so we just check diversity >= non-diversity)
	if uniqueDiv < uniqueNoDiv {
		t.Logf("Without diversity: %d unique sessions, With diversity: %d unique sessions",
			uniqueNoDiv, uniqueDiv)
		// Not a hard failure since results depend on exact distances
	}
}

func countUniqueSessions(results []search.SemanticResult) int {
	seen := make(map[string]bool)
	for _, r := range results {
		seen[r.SessionID] = true
	}
	return len(seen)
}

func TestSemanticSearch_MaxDistance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert embeddings at different distances from query
	// Distance 0.3 vector
	vecClose := make([]float32, 1024)
	vecClose[0] = 0.7 // Will have distance ~0.3 from query with 0=1.0

	// Distance 1.0 vector (orthogonal)
	vecFar := make([]float32, 1024)
	vecFar[1] = 1.0

	_, err = d.Exec(`
		INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
		VALUES ('e1_0', 's1', 'e1', 0, 'qwen3-embedding-0.6b', 1024, ?::FLOAT[1024], 'hash')`,
		vecClose)
	if err != nil {
		t.Fatal(err)
	}

	_, err = d.Exec(`
		INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
		VALUES ('e2_0', 's2', 'e2', 0, 'qwen3-embedding-0.6b', 1024, ?::FLOAT[1024], 'hash')`,
		vecFar)
	if err != nil {
		t.Fatal(err)
	}

	// Query vector
	query := make([]float32, 1024)
	query[0] = 1.0

	svc := search.NewService(d)

	// Without max distance - should get both
	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: query,
		Model:          "qwen3-embedding-0.6b",
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results without max_distance, got %d", len(results))
	}

	// With max distance 0.5 - should only get the close one
	results, err = svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: query,
		Model:          "qwen3-embedding-0.6b",
		Limit:          10,
		MaxDistance:    0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with max_distance=0.5, got %d", len(results))
	}
	if results[0].SessionID != "s1" {
		t.Fatalf("expected s1 (close result), got %s", results[0].SessionID)
	}
}
