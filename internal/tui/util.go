package tui

import (
	"os"

	"github.com/wethinkt/go-thinkt/internal/index/db"
)

// IndexerAvailable checks if the search index database exists.
func IndexerAvailable() bool {
	p, err := db.DefaultPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// DefaultDBPath returns the default path to the SQLite index file.
func DefaultDBPath() (string, error) {
	return db.DefaultPath()
}
