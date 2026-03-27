package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/config"
)

//go:embed schema/summaries.sql
var summariesSQL string

// DefaultSummariesPath returns the default path for the summaries database.
func DefaultSummariesPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dbs", "summaries.db"), nil
}

// OpenSummaries opens or creates a summaries database.
func OpenSummaries(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, config.DirPerms); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(summariesSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize summaries schema: %w", err)
	}

	return &DB{DB: db, path: path}, nil
}
