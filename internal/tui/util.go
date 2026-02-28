package tui

import (
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

// IndexerAvailable checks if the thinkt-indexer binary is available.
func IndexerAvailable() bool {
	return config.FindIndexerBinary() != ""
}

// DefaultDBPath returns the default path to the DuckDB index file.
func DefaultDBPath() (string, error) {
	return db.DefaultPath()
}
