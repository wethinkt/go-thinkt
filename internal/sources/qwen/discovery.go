// Package qwen provides Qwen Code session storage implementation.
package qwen

import (
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Factory returns a factory function for creating Qwen stores.
func Factory() thinkt.StoreFactory {
	return &qwenFactory{}
}

// qwenFactory implements thinkt.StoreFactory for Qwen Code.
type qwenFactory struct{}

// Source returns the source type.
func (f *qwenFactory) Source() thinkt.Source {
	return thinkt.SourceQwen
}

// Create attempts to create a Qwen store.
func (f *qwenFactory) Create() (thinkt.Store, error) {
	baseDir := getBaseDir()
	return NewStore(baseDir), nil
}

// IsAvailable checks if Qwen data directory exists.
func (f *qwenFactory) IsAvailable() (bool, error) {
	baseDir := getBaseDir()
	_, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// getBaseDir returns the base directory for Qwen data.
// It checks THINKT_QWEN_HOME environment variable first, then defaults to ~/.qwen.
func getBaseDir() string {
	// Check environment variable
	if env := os.Getenv("THINKT_QWEN_HOME"); env != "" {
		return env
	}

	// Default to ~/.qwen
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".qwen")
}

// IsAvailable checks if Qwen data directory exists.
func IsAvailable() bool {
	baseDir := getBaseDir()
	_, err := os.Stat(baseDir)
	return err == nil
}
