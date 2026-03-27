package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/config"
)

//go:embed schema/embeddings.sql
var embeddingsSQL string

// DefaultEmbeddingsPath returns the default path for the embeddings database.
func DefaultEmbeddingsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dbs", "embeddings.db"), nil
}

// OpenEmbeddings opens or creates an embeddings database with the given vector dimension.
// The vec0 virtual table is created with float[dim] distance_metric=cosine.
func OpenEmbeddings(path string, dim int) (*DB, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("invalid embedding dimension: %d", dim)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, config.DirPerms); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Replace {DIM} placeholder with actual dimension.
	schema := strings.ReplaceAll(embeddingsSQL, "{DIM}", fmt.Sprintf("%d", dim))
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize embeddings schema: %w", err)
	}

	return &DB{DB: db, path: path}, nil
}
