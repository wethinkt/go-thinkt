package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSummaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summaries.db")

	db, err := OpenSummaries(path)
	if err != nil {
		t.Fatalf("OpenSummaries: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO summaries (session_id, entry_uuid, summary, category, entities, relevance, model)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"sess-1", "entry-1", "Test summary", "idea", `["foo"]`, 0.9, "test-model",
	)
	if err != nil {
		t.Fatalf("insert summary: %v", err)
	}

	var summary, category string
	err = db.QueryRow("SELECT summary, category FROM summaries WHERE session_id = ?", "sess-1").
		Scan(&summary, &category)
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	if summary != "Test summary" || category != "idea" {
		t.Fatalf("unexpected: summary=%q category=%q", summary, category)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestOpenSummariesPrunable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summaries.db")

	db, err := OpenSummaries(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove: %v", err)
	}

	db2, err := OpenSummaries(path)
	if err != nil {
		t.Fatalf("re-create after prune: %v", err)
	}
	db2.Close()
}
