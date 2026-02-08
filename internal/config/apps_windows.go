//go:build windows

package config

// DefaultApps returns Windows default app configurations.
func DefaultApps() []AppConfig {
	apps := []AppConfig{
		{
			ID:      "explorer",
			Name:    "Explorer",
			Exec:    []string{"explorer", "{}"},
			Enabled: true,
		},
		{
			ID:      "terminal",
			Name:    "Windows Terminal",
			Exec:    []string{"wt", "-d", "{}"},
			Enabled: checkCommandExists("wt"),
		},
		{
			ID:      "cmd",
			Name:    "Command Prompt",
			Exec:    []string{"cmd", "/c", "start", "cmd", "/k", "cd", "/d", "{}"},
			Enabled: true,
		},
	}
	return append(apps, editorApps()...)
}
