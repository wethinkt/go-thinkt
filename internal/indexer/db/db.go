package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/wethinkt/go-thinkt/internal/config"
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

//go:embed schema/init.sql
var initSQL string

// DB wraps the DuckDB connection
type DB struct {
	*sql.DB
	path string
}

// Open initializes or opens a DuckDB database at the given path
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// DuckDB connection string
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	// Initialize schema
	if _, err := db.Exec(initSQL); err != nil {
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

// OpenReadOnly opens a DuckDB database at the given path in read-only mode.
//
// IMPORTANT: DuckDB currently does NOT support concurrent READ_ONLY connections
// when a READ_WRITE connection is active (e.g., from the watch command).
// This function is provided for future compatibility and for cases where no
// write connection is active. If you get a lock error, stop the watch process
// before running read operations.
//
// See: https://github.com/duckdb/duckdb/discussions/14676
func OpenReadOnly(path string) (*DB, error) {
	// DuckDB connection string with read-only access mode
	connStr := fmt.Sprintf("%s?access_mode=READ_ONLY", path)
	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb (read-only): %w", err)
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

// Close closes the database connection
func (d *DB) Close() error {
	return d.DB.Close()
}

// GetPath returns the filesystem path to the database file
func (d *DB) GetPath() string {
	return d.path
}
