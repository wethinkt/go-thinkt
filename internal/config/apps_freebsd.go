//go:build freebsd

package config

// DefaultApps returns FreeBSD default app configurations.
func DefaultApps() []AppConfig {
	apps := []AppConfig{
		{
			ID:      "files",
			Name:    "File Manager",
			Exec:    []string{"xdg-open", "{}"},
			Enabled: checkCommandExists("xdg-open"),
		},
	}
	return append(apps, editorApps()...)
}
