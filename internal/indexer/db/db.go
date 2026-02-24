package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

var (
	// ErrNoRows is returned when a query expects a row but none is found.
	ErrNoRows = sql.ErrNoRows
)

// DefaultPath returns the default filesystem path for the DuckDB index.
// It uses the standard ~/.thinkt directory.
func DefaultPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.duckdb"), nil
}

// DefaultEmbeddingsPath returns the default filesystem path for the embeddings database.
func DefaultEmbeddingsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "embeddings.duckdb"), nil
}

//go:embed schema/init.sql
var initSQL string

//go:embed schema/embeddings.sql
var embeddingsSQL string

// IndexSchema returns the schema SQL for the index database.
func IndexSchema() string { return initSQL }

// EmbeddingsSchema returns the schema SQL for the embeddings database.
func EmbeddingsSchema() string { return embeddingsSQL }

// DB wraps the DuckDB connection
type DB struct {
	*sql.DB
	path    string
	tempDir string // non-empty if this DB was opened via copy-on-read fallback
}

// Open initializes or opens a DuckDB index database at the given path.
func Open(path string) (*DB, error) {
	return openWithSchema(path, initSQL)
}

// OpenEmbeddings initializes or opens a DuckDB embeddings database at the given path.
func OpenEmbeddings(path string) (*DB, error) {
	return openWithSchema(path, embeddingsSQL)
}

// openWithSchema opens a DuckDB database, runs the given schema SQL, and hardens security.
func openWithSchema(path, schema string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Security hardening: Disable external access to prevent SQL injection attacks
	// from reading/writing arbitrary files or accessing network resources
	if _, err := db.Exec("SET enable_external_access=false"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set security settings: %w", err)
	}

	return &DB{
		DB:   db,
		path: path,
	}, nil
}

// Retry/copy-on-read constants for OpenReadOnly.
const (
	readOnlyRetries = 5                // direct-open retries before falling back to copy
	retryDelay      = 100 * time.Millisecond
	copyRetries     = 10               // how many times to try copy-then-validate
)

// OpenReadOnly opens a DuckDB database at the given path in read-only mode.
//
// Strategy when the database is locked:
//  1. Retry the direct read-only open a few times with a short delay. This
//     handles the common case where the watcher's brief open-on-demand lock
//     is released within milliseconds.
//  2. If the lock persists (e.g. a long-running sync), fall back to copying
//     the main database file and opening the copy. Because a checkpoint can
//     race with our copy, the copy is validated by opening it; if the copy
//     is corrupt the attempt is retried.
//
// See: https://github.com/duckdb/duckdb/discussions/14676
func OpenReadOnly(path string) (*DB, error) {
	// Phase 1: try direct open with retries.
	var lastErr error
	for attempt := range readOnlyRetries {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}
		d, err := tryOpenReadOnly(path)
		if err == nil {
			return d, nil
		}
		lastErr = err
		if !isLockError(err) {
			return nil, err
		}
	}

	// Phase 2: lock held for longer than retries cover — copy-on-read.
	tuilog.Log.Warn("db: locked after retries, falling back to copy-on-read", "retries", readOnlyRetries, "path", path)
	return openReadOnlyCopy(path, lastErr)
}

// tryOpenReadOnly attempts a single read-only open + security hardening.
func tryOpenReadOnly(path string) (*DB, error) {
	connStr := fmt.Sprintf("%s?access_mode=READ_ONLY", path)
	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("SET enable_external_access=false"); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{DB: db, path: path}, nil
}

// isLockError returns true if the error is a DuckDB lock contention error.
func isLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Conflicting lock") ||
		strings.Contains(msg, "same database file with a different configuration")
}

// openReadOnlyCopy copies the main database file to a temp directory, opens
// the copy read-only, and validates it by reading actual table data. If the
// copy was taken during a checkpoint and is corrupt, it retries after a delay.
func openReadOnlyCopy(path string, lastDirectErr error) (*DB, error) {
	for attempt := range copyRetries {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		tempDir, err := os.MkdirTemp("", "thinkt-db-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir for copy-on-read: %w", err)
		}

		// Copy only the main database file (not the WAL). Skipping the
		// WAL means we read data as of the last completed checkpoint,
		// which is acceptably stale.
		destPath := filepath.Join(tempDir, filepath.Base(path))
		if err := stableCopy(path, destPath); err != nil {
			os.RemoveAll(tempDir)
			tuilog.Log.Warn("db: copy-on-read attempt failed while copying", "attempt", attempt+1, "error", err)
			continue
		}

		connStr := fmt.Sprintf("%s?access_mode=READ_ONLY", destPath)
		db, err := sql.Open("duckdb", connStr)
		if err != nil {
			os.RemoveAll(tempDir)
			tuilog.Log.Warn("db: copy-on-read attempt failed while opening", "attempt", attempt+1, "error", err)
			continue
		}

		// Security hardening.
		if _, err := db.Exec("SET enable_external_access=false"); err != nil {
			db.Close()
			os.RemoveAll(tempDir)
			tuilog.Log.Warn("db: copy-on-read attempt failed during security hardening", "attempt", attempt+1, "error", err)
			continue
		}

		// Validate by reading actual table data to ensure data blocks
		// are intact (SELECT 1 would only test the header). A missing
		// table is acceptable — it means the schema hasn't been
		// checkpointed yet (stale but valid). Only IO/checksum errors
		// indicate a corrupt copy that should be retried.
		if err := validateCopy(db); err != nil {
			db.Close()
			os.RemoveAll(tempDir)
			tuilog.Log.Warn("db: copy-on-read attempt failed during validation", "attempt", attempt+1, "error", err)
			continue
		}

		return &DB{
			DB:      db,
			path:    path,
			tempDir: tempDir,
		}, nil
	}

	return nil, fmt.Errorf("database is locked and copy-on-read failed after %d attempts (last lock error: %v)", copyRetries, lastDirectErr)
}

// validateCopy runs queries against a copy to verify it is not corrupt.
// Returns nil if the copy is usable (even if tables don't exist yet due to
// the schema not being checkpointed — that's stale but valid).
func validateCopy(db *sql.DB) error {
	// Try reading actual table data to exercise data block reads.
	_, err := db.Exec("SELECT count(*) FROM projects")
	if err == nil {
		return nil
	}
	// "Table does not exist" means the schema hasn't been checkpointed yet.
	// The copy is stale but structurally valid.
	if strings.Contains(err.Error(), "does not exist") {
		return nil
	}
	// Any other error (IO errors, checksum mismatches) indicates corruption.
	return err
}

// errFileChangedDuringCopy is returned by stableCopy when the source file
// size changed during the copy, indicating a checkpoint was in progress.
var errFileChangedDuringCopy = fmt.Errorf("file changed during copy")

// stableCopy copies src to dst and verifies the source file size did not
// change during the copy. If it did, the copy may be inconsistent.
func stableCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Snapshot file size before copying.
	infoBefore, err := in.Stat()
	if err != nil {
		return err
	}
	sizeBefore := infoBefore.Size()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Check the source file size after the copy. If it changed, a
	// checkpoint was likely in progress and the copy is suspect.
	infoAfter, err := os.Stat(src)
	if err != nil {
		return err
	}
	if infoAfter.Size() != sizeBefore {
		return errFileChangedDuringCopy
	}

	return nil
}

// Close closes the database connection and cleans up any temp files.
func (d *DB) Close() error {
	err := d.DB.Close()
	if d.tempDir != "" {
		os.RemoveAll(d.tempDir)
	}
	return err
}

// GetPath returns the filesystem path to the database file
func (d *DB) GetPath() string {
	return d.path
}
