package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestLocaleSyntax ensures all TOML locale files are syntactically valid.
func TestLocaleSyntax(t *testing.T) {
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("finding project root: %v", err)
	}

	localeDir := filepath.Join(root, "internal", "i18n", "locales")
	entries, err := os.ReadDir(localeDir)
	if err != nil {
		t.Fatalf("reading locales dir: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			path := filepath.Join(localeDir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}

			var v map[string]interface{}
			if _, err := toml.Decode(string(data), &v); err != nil {
				t.Errorf("%s: invalid TOML syntax: %v", name, err)
			}
		})
	}
}
