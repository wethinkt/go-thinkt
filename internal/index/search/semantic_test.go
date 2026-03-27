package search

import (
	"path/filepath"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
)

func TestApplyDiversityScoring(t *testing.T) {
	results := []SemanticResult{
		{SessionID: "s1", Distance: 0.1, ProjectName: "proj1", Source: "claude"},
		{SessionID: "s1", Distance: 0.15, ProjectName: "proj1", Source: "claude"},
		{SessionID: "s2", Distance: 0.2, ProjectName: "proj1", Source: "claude"},
		{SessionID: "s3", Distance: 0.3, ProjectName: "proj2", Source: "kimi"},
		{SessionID: "s1", Distance: 0.35, ProjectName: "proj1", Source: "claude"},
	}
	selected := applyDiversityScoring(results, 3)
	if len(selected) != 3 {
		t.Fatalf("expected 3 results, got %d", len(selected))
	}
	if selected[0].SessionID != "s1" || selected[0].Distance != 0.1 {
		t.Fatalf("first result should be s1/0.1, got %s/%f", selected[0].SessionID, selected[0].Distance)
	}
	sessions := map[string]bool{}
	for _, r := range selected {
		sessions[r.SessionID] = true
	}
	if len(sessions) < 2 {
		t.Fatal("diversity scoring should promote different sessions")
	}
}

func TestSessionSimilarity(t *testing.T) {
	a := SemanticResult{SessionID: "s1", ProjectName: "p1", Source: "claude"}
	b := SemanticResult{SessionID: "s1", ProjectName: "p1", Source: "claude"}
	if s := sessionSimilarity(a, b); s != 1.0 {
		t.Fatalf("same session should be 1.0, got %f", s)
	}
	b.SessionID = "s2"
	if s := sessionSimilarity(a, b); s != 0.5 {
		t.Fatalf("same project+source should be 0.5, got %f", s)
	}
	b.ProjectName = "p2"
	if s := sessionSimilarity(a, b); s != 0.3 {
		t.Fatalf("same source only should be 0.3, got %f", s)
	}
	b.Source = "kimi"
	if s := sessionSimilarity(a, b); s != 0.0 {
		t.Fatalf("completely different should be 0.0, got %f", s)
	}
}

func TestSemanticSearchIntegration(t *testing.T) {
	dir := t.TempDir()

	idb, err := indexdb.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer idb.Close()

	_, _ = idb.Exec(`INSERT INTO projects (id, path, name, source) VALUES ('p1', '/tmp', 'TestProject', 'claude')`)
	_, _ = idb.Exec(`INSERT INTO sessions (id, project_id, path, first_prompt, entry_count) VALUES ('s1', 'p1', '/tmp/s1.jsonl', 'hello world', 2)`)
	_, _ = idb.Exec(`INSERT INTO entries (session_id, uuid, role, word_count, line_number) VALUES ('s1', 'e1', 'user', 5, 1)`)
	_, _ = idb.Exec(`INSERT INTO entries (session_id, uuid, role, word_count, line_number) VALUES ('s1', 'e2', 'assistant', 10, 2)`)

	embDB, err := indexdb.OpenEmbeddings(filepath.Join(dir, "embeddings.db"), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer embDB.Close()

	vec1, _ := sqlite_vec.SerializeFloat32([]float32{1, 0, 0, 0})
	vec2, _ := sqlite_vec.SerializeFloat32([]float32{0, 1, 0, 0})

	_, _ = embDB.Exec("INSERT INTO vec_embeddings(embedding_id, embedding, session_id, tier, model) VALUES (?, ?, ?, ?, ?)", 1, vec1, "s1", "conversation", "test")
	_, _ = embDB.Exec("INSERT INTO embedding_meta(embedding_id, entry_uuid, chunk_index, text_hash) VALUES (?, ?, ?, ?)", 1, "e1", 0, "hash1")
	_, _ = embDB.Exec("INSERT INTO vec_embeddings(embedding_id, embedding, session_id, tier, model) VALUES (?, ?, ?, ?, ?)", 2, vec2, "s1", "conversation", "test")
	_, _ = embDB.Exec("INSERT INTO embedding_meta(embedding_id, entry_uuid, chunk_index, text_hash) VALUES (?, ?, ?, ?)", 2, "e2", 0, "hash2")

	svc := NewService(idb)
	results, err := svc.SemanticSearch(embDB, SemanticSearchOptions{
		QueryEmbedding: []float32{0.9, 0.1, 0, 0},
		Model:          "test",
		Dim:            4,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].EntryUUID != "e1" {
		t.Fatalf("expected e1 first (closest), got %s", results[0].EntryUUID)
	}
	if results[0].Role != "user" {
		t.Fatalf("expected role=user, got %s", results[0].Role)
	}
	if results[0].ProjectName != "TestProject" {
		t.Fatalf("expected project=TestProject, got %s", results[0].ProjectName)
	}
}

func TestSemanticSearchWithProjectFilter(t *testing.T) {
	dir := t.TempDir()

	idb, err := indexdb.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer idb.Close()

	_, _ = idb.Exec(`INSERT INTO projects (id, path, name, source) VALUES ('p1', '/tmp/a', 'Alpha', 'claude')`)
	_, _ = idb.Exec(`INSERT INTO projects (id, path, name, source) VALUES ('p2', '/tmp/b', 'Beta', 'kimi')`)
	_, _ = idb.Exec(`INSERT INTO sessions (id, project_id, path, entry_count) VALUES ('s1', 'p1', '/tmp/a/s1.jsonl', 1)`)
	_, _ = idb.Exec(`INSERT INTO sessions (id, project_id, path, entry_count) VALUES ('s2', 'p2', '/tmp/b/s2.jsonl', 1)`)
	_, _ = idb.Exec(`INSERT INTO entries (session_id, uuid, role) VALUES ('s1', 'e1', 'user')`)
	_, _ = idb.Exec(`INSERT INTO entries (session_id, uuid, role) VALUES ('s2', 'e2', 'user')`)

	embDB, err := indexdb.OpenEmbeddings(filepath.Join(dir, "embeddings.db"), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer embDB.Close()

	vec, _ := sqlite_vec.SerializeFloat32([]float32{1, 0, 0, 0})
	_, _ = embDB.Exec("INSERT INTO vec_embeddings(embedding_id, embedding, session_id, tier, model) VALUES (?, ?, ?, ?, ?)", 1, vec, "s1", "conversation", "test")
	_, _ = embDB.Exec("INSERT INTO embedding_meta(embedding_id, entry_uuid, chunk_index, text_hash) VALUES (?, ?, ?, ?)", 1, "e1", 0, "h1")
	_, _ = embDB.Exec("INSERT INTO vec_embeddings(embedding_id, embedding, session_id, tier, model) VALUES (?, ?, ?, ?, ?)", 2, vec, "s2", "conversation", "test")
	_, _ = embDB.Exec("INSERT INTO embedding_meta(embedding_id, entry_uuid, chunk_index, text_hash) VALUES (?, ?, ?, ?)", 2, "e2", 0, "h2")

	svc := NewService(idb)
	results, err := svc.SemanticSearch(embDB, SemanticSearchOptions{
		QueryEmbedding: []float32{1, 0, 0, 0},
		Model:          "test",
		Dim:            4,
		Limit:          10,
		FilterProject:  "Alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after project filter, got %d", len(results))
	}
	if results[0].SessionID != "s1" {
		t.Fatalf("expected s1, got %s", results[0].SessionID)
	}
}
