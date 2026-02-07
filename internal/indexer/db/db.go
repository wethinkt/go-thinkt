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
