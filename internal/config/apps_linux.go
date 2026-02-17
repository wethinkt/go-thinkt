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
		{
			ID:      "konsole",
			Name:    "Konsole",
			Exec:    []string{"konsole", "{}"},
			Enabled: checkCommandExists("konsole"),
		},
		{
			ID:      "nautilus",
			Name:    "Nautilus",
			Exec:    []string{"nautilus", "{}"},
			Enabled: checkCommandExists("nautilus"),
		},
		{
			ID:      "dolphin",
			Name:    "Dolphin",
			Exec:    []string{"dolphin", "{}"},
			Enabled: checkCommandExists("dolphin"),
		},
		{
			ID:      "thunar",
			Name:    "Thunar",
			Exec:    []string{"thunar", "{}"},
			Enabled: checkCommandExists("thunar"),
		},
		{
			ID:      "hx",
			Name:    "Helix",
			Exec:    []string{"hx", "{}"},
			Enabled: checkCommandExists("hx"),
		},
	}
	return filterAvailable(append(apps, commonApps()...))
}
