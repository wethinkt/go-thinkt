package embedding_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestEndToEnd_EmbedAndSearch(t *testing.T) {
	embedder, err := embedding.NewEmbedder("")
	if err != nil {
		t.Skipf("yzma model not available: %v", err)
	}
	defer embedder.Close()

	dir := t.TempDir()

	indexDB, err := db.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer indexDB.Close()

	embDB, err := db.OpenEmbeddings(filepath.Join(dir, "embeddings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer embDB.Close()

	entries := []thinkt.Entry{
		{UUID: "e1", Role: thinkt.RoleUser, Text: "How do I fix the authentication timeout in the login flow?"},
		{UUID: "e2", Role: thinkt.RoleAssistant, Text: "The timeout is caused by a slow database query in the auth middleware."},
		{UUID: "e3", Role: thinkt.RoleUser, Text: "Can you help me set up a CI/CD pipeline with GitHub Actions?"},
	}

	var entryTexts []embedding.EntryText
	for _, e := range entries {
		text := embedding.ExtractText(e)
		if text == "" {
			continue
		}
		entryTexts = append(entryTexts, embedding.EntryText{
			UUID: e.UUID, SessionID: "s1", Text: text,
		})
	}

	requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)

	// Extract texts and embed with yzma
	texts := make([]string, len(requests))
	for i, r := range requests {
		texts[i] = r.Text
	}
	embedResult, err := embedder.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	// Set up session/project rows in the index DB for the join
	indexDB.Exec("INSERT INTO projects (id, path, name, source) VALUES ('p1', '/test', 'test-project', 'claude')")
	indexDB.Exec("INSERT INTO sessions (id, project_id, path, entry_count) VALUES ('s1', 'p1', '/test/s1.jsonl', 3)")
	for _, e := range entries {
		indexDB.Exec("INSERT INTO entries (uuid, session_id, role, word_count) VALUES (?, 's1', ?, ?)",
			e.UUID, string(e.Role), len(e.Text))
	}

	// Store embeddings in the embeddings DB
	for idx, m := range mapping {
		if idx >= len(embedResult.Vectors) {
			break
		}
		id := requests[idx].ID
		_, err := embDB.Exec(`
			INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[1024], ?)`,
			id, m.SessionID, m.EntryUUID, m.ChunkIndex, embedder.EmbedModelID(), embedder.Dim(), embedResult.Vectors[idx], m.TextHash)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Search for "auth timeout" — should find e1 and e2 before e3
	queryResult, err := embedder.Embed(context.Background(), []string{"authentication timeout problem"})
	if err != nil {
		t.Fatal(err)
	}

	svc := search.NewService(indexDB, embDB)
	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: queryResult.Vectors[0],
		Model:          embedding.ModelID,
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	t.Logf("Results:")
	for _, r := range results {
		t.Logf("  distance=%.4f entry=%s role=%s", r.Distance, r.EntryUUID, r.Role)
	}

	top := results[0].EntryUUID
	if top != "e1" && top != "e2" {
		t.Fatalf("expected e1 or e2 as top result, got %s", top)
	}
}
