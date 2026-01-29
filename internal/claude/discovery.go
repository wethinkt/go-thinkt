package claude

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Claude Code.
type Discoverer struct{}

// NewDiscoverer creates a new Claude discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Source returns the Claude source type.
func (d *Discoverer) Source() thinkt.Source {
	return thinkt.SourceClaude
}

// Create creates a Claude store if available.
func (d *Discoverer) Create() (thinkt.Store, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable checks if Claude storage exists and has data.
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

// basePath returns the Claude base directory.
func (d *Discoverer) basePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	claudeDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		return ""
	}

	return claudeDir
}

// Factory returns a thinkt.StoreFactory for Claude.
// This can be used with thinkt.Discovery.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
