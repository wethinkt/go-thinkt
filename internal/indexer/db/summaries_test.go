package db_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

func TestSummariesPathForModel(t *testing.T) {
	dir := "/tmp/summaries"
	got := db.SummariesPathForModel(dir, "claude-3.5-sonnet")
	want := filepath.Join(dir, "claude-3.5-sonnet.duckdb")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}

	// Unsafe characters are sanitized
	got = db.SummariesPathForModel(dir, "org/model:latest")
	want = filepath.Join(dir, "org_model_latest.duckdb")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestOpenSummaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.OpenSummaries(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert a row
	_, err = d.Exec(`
		INSERT INTO summaries (session_id, entry_uuid, summary, category, entities, relevance, model)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"sess1", "entry1", "Fixed a bug in the parser", "discovery", `["parser","bug"]`, 0.8, "claude-3.5-sonnet",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Query it back
	var sessionID, summary, model string
	var relevance float64
	err = d.QueryRow("SELECT session_id, summary, relevance, model FROM summaries WHERE session_id = ?", "sess1").
		Scan(&sessionID, &summary, &relevance, &model)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != "sess1" || summary != "Fixed a bug in the parser" || model != "claude-3.5-sonnet" {
		t.Fatalf("unexpected values: session_id=%s summary=%s model=%s", sessionID, summary, model)
	}
	if relevance != 0.8 {
		t.Fatalf("expected relevance 0.8, got %f", relevance)
	}
}

func TestOpenSummariesSessionSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.OpenSummaries(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert a session-level summary (sentinel entry_uuid)
	_, err = d.Exec(`
		INSERT INTO summaries (session_id, entry_uuid, summary, model)
		VALUES (?, ?, ?, ?)`,
		"sess1", "__session__", "Overall session summary", "claude-3.5-sonnet",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Insert an entry-level summary
	_, err = d.Exec(`
		INSERT INTO summaries (session_id, entry_uuid, summary, category, relevance, model)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"sess1", "entry1", "Discovered a pattern", "pattern", 0.9, "claude-3.5-sonnet",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Query excluding session-level summaries
	rows, err := d.Query("SELECT entry_uuid, summary FROM summaries WHERE session_id = ? AND entry_uuid != '__session__'", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var entryUUID, summary string
		if err := rows.Scan(&entryUUID, &summary); err != nil {
			t.Fatal(err)
		}
		if entryUUID != "entry1" {
			t.Fatalf("expected entry1, got %s", entryUUID)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 entry-level row, got %d", count)
	}

	// Verify session-level row exists
	var sessionSummary string
	err = d.QueryRow("SELECT summary FROM summaries WHERE session_id = ? AND entry_uuid = '__session__'", "sess1").
		Scan(&sessionSummary)
	if err != nil {
		t.Fatal(err)
	}
	if sessionSummary != "Overall session summary" {
		t.Fatalf("unexpected session summary: %s", sessionSummary)
	}
}

func TestDefaultSummariesDir(t *testing.T) {
	dir, err := db.DefaultSummariesDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.HasSuffix(dir, "summaries") {
		t.Fatalf("expected path ending in 'summaries', got %s", dir)
	}
}
