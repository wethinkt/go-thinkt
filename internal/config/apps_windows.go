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
			ExecRun: []string{"wt", "cmd", "/c", "{}"},
			Enabled: checkCommandExists("wt"),
		},
		{
			ID:      "cmd",
			Name:    "Command Prompt",
			Exec:    []string{"cmd", "/c", "start", "cmd", "/k", "cd", "/d", "{}"},
			ExecRun: []string{"cmd", "/c", "start", "cmd", "/k", "{}"},
			Enabled: true,
		},
		{
			ID:      "powershell",
			Name:    "PowerShell",
			Exec:    []string{"pwsh", "{}"},
			ExecRun: []string{"pwsh", "-Command", "{}"},
			Enabled: checkCommandExists("pwsh"),
		},
	}
	return filterAvailable(append(apps, commonApps()...))
}
