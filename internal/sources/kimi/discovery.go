package kimi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Kimi Code.
type Discoverer struct{}

// NewDiscoverer creates a new Kimi discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Source returns the Kimi source type.
func (d *Discoverer) Source() thinkt.Source {
	return thinkt.SourceKimi
}

// Create creates a Kimi store if available.
func (d *Discoverer) Create() (thinkt.Store, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable checks if Kimi storage exists and has data.
func (d *Discoverer) IsAvailable() (bool, error) {
	store, err := d.Create()
	if err != nil || store == nil {
		return false, err
	}

	projects, err := store.ListProjects(context.TODO())
	if err != nil {
		return false, fmt.Errorf("list kimi projects: %w", err)
	}
	return len(projects) > 0, nil
}

// basePath returns the Kimi base directory.
// Uses THINKT_KIMI_HOME environment variable if set, otherwise ~/.kimi.
func (d *Discoverer) basePath() string {
	// Check THINKT_KIMI_HOME environment variable first
	if kimiHome := os.Getenv("THINKT_KIMI_HOME"); kimiHome != "" {
		if _, err := os.Stat(kimiHome); err == nil {
			return kimiHome
		}
		// If THINKT_KIMI_HOME is set but doesn't exist, still return it
		// so the caller can decide how to handle it
		return kimiHome
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	kimiDir := filepath.Join(home, ".kimi")
	if _, err := os.Stat(kimiDir); os.IsNotExist(err) {
		return ""
	}

	return kimiDir
}

// DefaultDir returns the Kimi base directory.
// Uses THINKT_KIMI_HOME environment variable if set, otherwise ~/.kimi.
func DefaultDir() (string, error) {
	// Check THINKT_KIMI_HOME environment variable first
	if kimiHome := os.Getenv("THINKT_KIMI_HOME"); kimiHome != "" {
		return kimiHome, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kimi"), nil
}

// IsSessionPath reports whether path looks like a Kimi session file.
func IsSessionPath(path string) bool {
	base := filepath.Base(path)
	if base == "context.jsonl" || (strings.HasPrefix(base, "context_sub_") && strings.HasSuffix(base, ".jsonl")) {
		return true
	}
	dir, _ := DefaultDir()
	if dir != "" && strings.HasPrefix(filepath.Clean(path), filepath.Clean(dir)) {
		return true
	}
	return false
}

// Factory returns a thinkt.StoreFactory for Kimi.
// This can be used with thinkt.Discovery.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
