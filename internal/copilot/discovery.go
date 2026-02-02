package copilot

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Copilot CLI.
type Discoverer struct{}

// NewDiscoverer creates a new Copilot discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Source returns the Copilot source type.
func (d *Discoverer) Source() thinkt.Source {
	return thinkt.SourceCopilot
}

// Create creates a Copilot store if available.
func (d *Discoverer) Create() (thinkt.Store, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable checks if Copilot storage exists and has data.
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

// basePath returns the Copilot base directory.
// Uses THINKT_COPILOT_HOME environment variable if set, otherwise ~/.copilot.
func (d *Discoverer) basePath() string {
	if copilotHome := os.Getenv("THINKT_COPILOT_HOME"); copilotHome != "" {
		if _, err := os.Stat(copilotHome); err == nil {
			return copilotHome
		}
		return copilotHome
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	copilotDir := filepath.Join(home, ".copilot")
	if _, err := os.Stat(copilotDir); os.IsNotExist(err) {
		return ""
	}

	return copilotDir
}

// Factory returns a thinkt.StoreFactory for Copilot.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
