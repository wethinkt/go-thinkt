package gemini

import (
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Gemini.
type Discoverer struct{}

// NewDiscoverer creates a new Gemini factory.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Factory returns a thinkt.StoreFactory for Gemini.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}

// Source returns the source type.
func (f *Discoverer) Source() thinkt.Source {
	return thinkt.SourceGemini
}

// Create creates a store if the source is available.
func (f *Discoverer) Create() (thinkt.Store, error) {
	basePath := f.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable checks if the source directory exists and has data.
func (f *Discoverer) IsAvailable() (bool, error) {
	basePath := f.basePath()
	if basePath == "" {
		return false, nil
	}

	// Check if tmp dir exists and has content
	tmpDir := filepath.Join(basePath, "tmp")
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return len(entries) > 0, nil
}

// basePath returns the Gemini base directory.
// Uses THINKT_GEMINI_HOME environment variable if set, otherwise ~/.gemini.
func (f *Discoverer) basePath() string {
	if geminiHome := os.Getenv("THINKT_GEMINI_HOME"); geminiHome != "" {
		return geminiHome
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini")
}