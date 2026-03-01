//go:build cgo

package collect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a DuckDBStore in a temp directory for testing.
func newTestStore(t *testing.T) *DuckDBStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")
	store, err := NewDuckDBStore(dbPath, 100, time.Second)
	if err != nil {
		t.Fatalf("NewDuckDBStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestExportParquet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert test entries directly via the main db.
	now := time.Now().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		_, err := store.db.ExecContext(ctx, `
			INSERT INTO collected_entries (uuid, session_id, role, timestamp, model, text, input_tokens, output_tokens)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"uuid-"+string(rune('a'+i)), "sess-1", "assistant", ts, "claude-4", "hello", 10, 20,
		)
		if err != nil {
			t.Fatalf("insert entry %d: %v", i, err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "export")

	// Export all entries.
	if err := store.ExportParquet(ctx, outDir, ExportOptions{}); err != nil {
		t.Fatalf("ExportParquet: %v", err)
	}

	// Verify parquet file exists.
	parquetFile := filepath.Join(outDir, "entries.parquet")
	info, err := os.Stat(parquetFile)
	if err != nil {
		t.Fatalf("parquet file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("parquet file is empty")
	}

	// Verify readable via read_parquet on the export connection.
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM read_parquet('"+parquetFile+"')").Scan(&count)
	if err != nil {
		t.Fatalf("read_parquet: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 rows in parquet, got %d", count)
	}
}

func TestExportParquetWithDateFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		ts := base.Add(time.Duration(i) * 24 * time.Hour)
		_, err := store.db.ExecContext(ctx, `
			INSERT INTO collected_entries (uuid, session_id, role, timestamp, model, text)
			VALUES (?, ?, ?, ?, ?, ?)`,
			"uuid-"+string(rune('a'+i)), "sess-1", "assistant", ts, "claude-4", "entry",
		)
		if err != nil {
			t.Fatalf("insert entry %d: %v", i, err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "filtered")
	since := base.Add(3 * 24 * time.Hour)  // June 4
	until := base.Add(7 * 24 * time.Hour)  // June 8

	err := store.ExportParquet(ctx, outDir, ExportOptions{
		Since: &since,
		Until: &until,
	})
	if err != nil {
		t.Fatalf("ExportParquet with filter: %v", err)
	}

	parquetFile := filepath.Join(outDir, "entries.parquet")
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM read_parquet('"+parquetFile+"')").Scan(&count)
	if err != nil {
		t.Fatalf("read_parquet: %v", err)
	}
	// Days 3,4,5,6 = 4 entries (since >= June 4, until < June 8)
	if count != 4 {
		t.Errorf("expected 4 filtered rows, got %d", count)
	}
}
