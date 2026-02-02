package gemini

import (
	"os"
	"path/filepath"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
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
	available, err := f.IsAvailable()
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, nil
	}
	return NewStore(""), nil // Use default path
}

// IsAvailable checks if the source directory exists and has data.
func (f *Discoverer) IsAvailable() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	
	// Check if ~/.gemini/tmp exists and has content
	baseDir := filepath.Join(home, ".gemini")
	tmpDir := filepath.Join(baseDir, "tmp")
	
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	
	return len(entries) > 0, nil
}