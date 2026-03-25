package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// ErrNoRows is returned when a query expects a row but none is found.
var ErrNoRows = sql.ErrNoRows

//go:embed schema/init.sql
var initSQL string

// DB wraps a SQLite connection.
type DB struct {
	*sql.DB
	path string
}

// DefaultPath returns the default filesystem path for the SQLite index.
func DefaultPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dbs", "index.db"), nil
}

// Open initializes or opens a SQLite index database at the given path.
// Enables WAL mode for concurrent read access.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, config.DirPerms); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(initSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return &DB{DB: db, path: path}, nil
}

// OpenReadOnly opens a SQLite database in read-only mode.
// Returns an error if the database does not exist.
func OpenReadOnly(path string) (*DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("database not found: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?mode=ro&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}

	return &DB{DB: db, path: path}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.DB.Close()
}

// GetPath returns the filesystem path to the database file.
func (d *DB) GetPath() string {
	return d.path
}

// migrate runs necessary schema updates.
func migrate(db *sql.DB) error {
	var currentVersion int
	err := db.QueryRow(`SELECT COALESCE(max(version), 0) FROM migrations`).Scan(&currentVersion)
	if err != nil {
		return err
	}

	// Version 1: Initial schema (applied via init.sql).
	if currentVersion < 1 {
		if _, err := db.Exec(`INSERT INTO migrations (version) VALUES (1)`); err != nil {
			return err
		}
	}

	// Future migrations go here:
	// if currentVersion < 2 { ... }

	return nil
}
