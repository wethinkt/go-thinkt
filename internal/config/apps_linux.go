//go:build linux

package config

// DefaultApps returns Linux default app configurations.
func DefaultApps() []AppConfig {
	apps := []AppConfig{
		{
			ID:      "files",
			Name:    "File Manager",
			Exec:    []string{"xdg-open", "{}"},
			Enabled: checkCommandExists("xdg-open"),
		},
		{
			ID:      "terminal",
			Name:    "Terminal",
			Exec:    []string{"x-terminal-emulator", "{}"},
			Enabled: checkCommandExists("x-terminal-emulator"),
		},
	}
	return append(apps, editorApps()...)
}
