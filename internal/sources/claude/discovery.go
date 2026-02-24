package claude

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
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
// Uses THINKT_CLAUDE_HOME environment variable if set, otherwise ~/.claude.
func (d *Discoverer) basePath() string {
	// Check THINKT_CLAUDE_HOME environment variable first
	if claudeHome := os.Getenv("THINKT_CLAUDE_HOME"); claudeHome != "" {
		if _, err := os.Stat(claudeHome); err == nil {
			return claudeHome
		}
		// If THINKT_CLAUDE_HOME is set but doesn't exist, still return it
		// so the caller can decide how to handle it
		return claudeHome
	}

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

// IsSessionPath reports whether path looks like a Claude session file.
func IsSessionPath(path string) bool {
	if filepath.Ext(path) != ".jsonl" {
		return false
	}
	baseDir := (&Discoverer{}).basePath()
	if baseDir != "" && thinkt.IsPathWithinAny(path, []string{baseDir}) {
		return true
	}
	return false
}

// CreateTeamStore creates a TeamStore for Claude Code teams.
// This implements thinkt.TeamStoreFactory.
func (d *Discoverer) CreateTeamStore() (thinkt.TeamStore, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewTeamStore(basePath), nil
}

// Factory returns a thinkt.StoreFactory for Claude.
// The returned factory also implements thinkt.TeamStoreFactory.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
