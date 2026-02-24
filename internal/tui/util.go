package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// IndexerAvailable checks if the thinkt-indexer binary is available.
func IndexerAvailable() bool {
	return config.FindIndexerBinary() != ""
}

// DefaultDBPath returns the default path to the DuckDB index file.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".thinkt", "index.duckdb"), nil
}
