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
			ExecRun: []string{"osascript", "-e", "tell application \"Terminal\"\nactivate\ndo script \"{}\"\nend tell"},
			Enabled: true,
		},
		{
			ID:      "iterm",
			Name:    "iTerm",
			Exec:    []string{"open", "-a", "iTerm", "{}"},
			ExecRun: []string{"osascript", "-e", "tell application \"iTerm2\"\nactivate\ncreate window with default profile command \"{}\"\nend tell"},
			Enabled: checkAppExists("iTerm"),
		},
		{
			ID:      "xcode",
			Name:    "Xcode",
			Exec:    []string{"open", "-a", "Xcode", "{}"},
			Enabled: checkAppExists("Xcode"),
		},
		{
			ID:      "conductor",
			Name:    "Conductor",
			Exec:    []string{"open", "-a", "Conductor", "{}"},
			Enabled: checkAppExists("Conductor"),
		},
	}
	return filterAvailable(append(apps, commonApps()...))
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
