package codex

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Codex CLI.
type Discoverer struct{}

// NewDiscoverer creates a new Codex source discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Factory returns a thinkt.StoreFactory for Codex CLI.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}

// Source returns the source type.
func (d *Discoverer) Source() thinkt.Source {
	return thinkt.SourceCodex
}

// Create creates a store if the source is available.
func (d *Discoverer) Create() (thinkt.Store, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable checks whether Codex session data exists.
func (d *Discoverer) IsAvailable() (bool, error) {
	store, err := d.Create()
	if err != nil || store == nil {
		return false, err
	}

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		return false, nil
	}
	return len(projects) > 0, nil
}

func (d *Discoverer) basePath() string {
	if codexHome := os.Getenv("THINKT_CODEX_HOME"); codexHome != "" {
		return codexHome
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	codexDir := filepath.Join(home, ".codex")
	if _, err := os.Stat(codexDir); os.IsNotExist(err) {
		return ""
	}
	return codexDir
}
