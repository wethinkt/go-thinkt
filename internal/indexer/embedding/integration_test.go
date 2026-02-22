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
	client, err := embedding.NewClient()
	if err != nil {
		t.Skipf("thinkt-embed-apple not available: %v", err)
	}
	defer client.Close()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

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

	responses, err := client.EmbedBatch(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}

	respMap := make(map[string]embedding.EmbedResponse)
	for _, r := range responses {
		respMap[r.ID] = r
	}

	// Set up session/project rows for the join
	d.Exec("INSERT INTO projects (id, path, name, source) VALUES ('p1', '/test', 'test-project', 'claude')")
	d.Exec("INSERT INTO sessions (id, project_id, path, entry_count) VALUES ('s1', 'p1', '/test/s1.jsonl', 3)")
	for _, e := range entries {
		d.Exec("INSERT INTO entries (uuid, session_id, role, word_count) VALUES (?, 's1', ?, ?)",
			e.UUID, string(e.Role), len(e.Text))
	}

	for idx, m := range mapping {
		id := requests[idx].ID
		resp := respMap[id]
		_, err := d.Exec(`
			INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
			VALUES (?, ?, ?, ?, 'apple-nlcontextual-v1', ?, ?::FLOAT[512], ?)`,
			id, m.SessionID, m.EntryUUID, m.ChunkIndex, resp.Dim, resp.Embedding, m.TextHash)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Search for "auth timeout" — should find e1 and e2 before e3
	queryResp, err := client.EmbedBatch(context.Background(), []embedding.EmbedRequest{
		{ID: "q", Text: "authentication timeout problem"},
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := search.NewService(d)
	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: queryResp[0].Embedding,
		Model:          "apple-nlcontextual-v1",
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
