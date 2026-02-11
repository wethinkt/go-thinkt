package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// IndexerAvailable checks if the thinkt-indexer binary is available.
func IndexerAvailable() bool {
	return findIndexerBinary() != ""
}

// findIndexerBinary attempts to locate the thinkt-indexer binary.
func findIndexerBinary() string {
	// 1. Check same directory as current executable
	if execPath, err := os.Executable(); err == nil {
		binDir := filepath.Dir(execPath)
		indexerPath := filepath.Join(binDir, "thinkt-indexer")
		if _, err := os.Stat(indexerPath); err == nil {
			return indexerPath
		}
	}

	// 2. Check system PATH
	if path, err := exec.LookPath("thinkt-indexer"); err == nil {
		return path
	}

	return ""
}

// DefaultDBPath returns the default path to the DuckDB index file.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".thinkt", "index.duckdb"), nil
}
