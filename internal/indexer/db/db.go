package db

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
// It uses the standard ~/.thinkt/dbs directory.
func DefaultPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dbs", "indexer.duckdb"), nil
}

// DefaultEmbeddingsDir returns the directory that holds per-model embedding databases.
func DefaultEmbeddingsDir() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "embeddings"), nil
}

// EmbeddingsPathForModel returns the DB file path for a given model inside dir.
// Characters unsafe for filenames are replaced with '_'.
func EmbeddingsPathForModel(dir, modelID string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, modelID)
	return filepath.Join(dir, safe+".duckdb")
}

//go:embed schema/init.sql
var initSQL string

//go:embed schema/embeddings.sql
var embeddingsSQL string

// IndexSchema returns the schema SQL for the index database.
func IndexSchema() string { return initSQL }

// EmbeddingsSchemaForDim returns the embeddings schema SQL with the
// given embedding dimension substituted for the {DIM} placeholder.
func EmbeddingsSchemaForDim(dim int) string {
	return strings.ReplaceAll(embeddingsSQL, "{DIM}", strconv.Itoa(dim))
}

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
// dim specifies the embedding dimension for the schema (e.g. 768 for nomic, 1024 for qwen3).
func OpenEmbeddings(path string, dim int) (*DB, error) {
	return openWithSchema(path, EmbeddingsSchemaForDim(dim))
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

	// Apply migrations.
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
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

// migrate runs necessary schema updates.
func migrate(db *sql.DB) error {
	// Ensure migrations table exists (already in schema, but defensive)
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
	if err != nil {
		return err
	}

	var currentVersion int
	err = db.QueryRow(`SELECT COALESCE(max(version), 0) FROM migrations`).Scan(&currentVersion)
	if err != nil {
		return err
	}

	// Version 1: Initial schema with comments (applied via init.sql/embeddings.sql)
	// We just record it if not already present.
	if currentVersion < 1 {
		if _, err := db.Exec(`INSERT INTO migrations (version) VALUES (1)`); err != nil {
			return err
		}
		currentVersion = 1
	}

	// Future migrations go here:
	// if currentVersion < 2 { ... }

	return nil
}

// Retry/copy-on-read constants for OpenReadOnly.
const (
	readOnlyRetries = 5 // direct-open retries before falling back to copy
	retryDelay      = 100 * time.Millisecond
	copyRetries     = 10 // how many times to try snapshot-then-validate
)

// OpenReadOnly opens a DuckDB database at the given path in read-only mode.
//
// Strategy when the database is locked:
//  1. Retry the direct read-only open a few times with a short delay. This
//     handles the common case where the watcher's brief open-on-demand lock
//     is released within milliseconds.
//  2. If the lock persists (e.g. a long-running sync), fall back to taking a
//     filesystem snapshot of the main database file and opening the snapshot.
//     If the filesystem cannot provide a safe snapshot, fail closed rather
//     than risk a torn copy of a live DuckDB file.
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

// openReadOnlyCopy takes a filesystem-level snapshot of the main database file,
// opens the snapshot read-only, and validates it by reading actual table data.
// If the filesystem cannot provide a safe snapshot, it fails closed rather than
// risking a torn byte copy of a live DuckDB file.
func openReadOnlyCopy(path string, lastDirectErr error) (*DB, error) {
	for attempt := range copyRetries {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		tempDir, err := makeCopyTempDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir for copy-on-read: %w", err)
		}

		// Snapshot only the main database file (not the WAL). Skipping the
		// WAL means we read data as of the last completed checkpoint, which
		// is acceptably stale.
		destPath := filepath.Join(tempDir, filepath.Base(path))
		if err := snapshotCopy(path, destPath); err != nil {
			os.RemoveAll(tempDir)
			if errors.Is(err, errSnapshotCopyUnavailable) {
				return nil, fmt.Errorf("database is locked and safe copy-on-read is unavailable on this filesystem: %w", lastDirectErr)
			}
			tuilog.Log.Warn("db: copy-on-read attempt failed while snapshotting", "attempt", attempt+1, "error", err)
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

		// Validate by reading actual table data to ensure data blocks are
		// intact. A missing table is acceptable — it means the schema
		// hasn't been checkpointed yet (stale but valid). Only IO/checksum
		// errors indicate a corrupt copy that should be retried.
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
	checks := []func(*sql.DB) error{
		func(db *sql.DB) error {
			var rows, idBytes, nameBytes int64
			return db.QueryRow("SELECT count(*), coalesce(sum(length(id)), 0), coalesce(sum(length(name)), 0) FROM projects").Scan(&rows, &idBytes, &nameBytes)
		},
		func(db *sql.DB) error {
			var rows, idBytes, pathBytes int64
			return db.QueryRow("SELECT count(*), coalesce(sum(length(id)), 0), coalesce(sum(length(path)), 0) FROM sessions").Scan(&rows, &idBytes, &pathBytes)
		},
		func(db *sql.DB) error {
			var rows, sessionBytes, uuidBytes int64
			return db.QueryRow("SELECT count(*), coalesce(sum(length(session_id)), 0), coalesce(sum(length(uuid)), 0) FROM entries").Scan(&rows, &sessionBytes, &uuidBytes)
		},
		func(db *sql.DB) error {
			var rows, fileBytes int64
			return db.QueryRow("SELECT count(*), coalesce(sum(length(file_path)), 0) FROM sync_state").Scan(&rows, &fileBytes)
		},
		func(db *sql.DB) error {
			var rows, idBytes, hashBytes int64
			return db.QueryRow("SELECT count(*), coalesce(sum(length(id)), 0), coalesce(sum(length(text_hash)), 0) FROM embeddings").Scan(&rows, &idBytes, &hashBytes)
		},
	}

	for _, check := range checks {
		err := check(db)
		if err == nil {
			return nil
		}
		// "Table does not exist" means the schema hasn't been checkpointed yet.
		// The copy is stale but structurally valid.
		if strings.Contains(err.Error(), "does not exist") {
			continue
		}
		return err
	}

	var rows int64
	err := db.QueryRow("SELECT count(*) FROM migrations").Scan(&rows)
	if err == nil || strings.Contains(err.Error(), "does not exist") {
		return nil
	}
	return err
}

func makeCopyTempDir(path string) (string, error) {
	parent := filepath.Dir(path)
	tempDir, err := os.MkdirTemp(parent, ".thinkt-db-*")
	if err == nil {
		return tempDir, nil
	}
	return os.MkdirTemp("", "thinkt-db-*")
}

func snapshotCopy(src, dst string) error {
	if err := trySnapshotCopy(src, dst); err != nil {
		if errors.Is(err, errSnapshotCopyUnavailable) {
			return err
		}
		return err
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
