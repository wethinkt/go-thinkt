package kimi

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
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
		return false, nil
	}
	return len(projects) > 0, nil
}

// basePath returns the Kimi base directory.
func (d *Discoverer) basePath() string {
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

// Factory returns a thinkt.StoreFactory for Kimi.
// This can be used with thinkt.Discovery.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
