//go:build darwin

package config

import (
	"os"
	"path/filepath"
)

// DefaultApps returns macOS default app configurations.
func DefaultApps() []AppConfig {
	apps := []AppConfig{
		{
			ID:      "finder",
			Name:    "Finder",
			Exec:    []string{"open", "{}"},
			Enabled: true,
		},
		{
			ID:      "terminal",
			Name:    "Terminal",
			Exec:    []string{"open", "-a", "Terminal", "{}"},
			Enabled: true,
		},
		{
			ID:      "iterm",
			Name:    "iTerm",
			Exec:    []string{"open", "-a", "iTerm", "{}"},
			Enabled: checkAppExists("iTerm"),
		},
	}
	return append(apps, editorApps()...)
}

// checkAppExists checks if a macOS app exists in /Applications.
func checkAppExists(name string) bool {
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, "Applications", name+".app")); err == nil {
			return true
		}
	}
	if _, err := os.Stat("/Applications/" + name + ".app"); err == nil {
		return true
	}
	return false
}
