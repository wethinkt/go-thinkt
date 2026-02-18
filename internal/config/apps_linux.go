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
			ExecRun: []string{"x-terminal-emulator", "-e", "sh", "-c", "{}"},
			Enabled: checkCommandExists("x-terminal-emulator"),
		},
		{
			ID:      "konsole",
			Name:    "Konsole",
			Exec:    []string{"konsole", "{}"},
			ExecRun: []string{"konsole", "-e", "sh", "-c", "{}"},
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
		// Terminals
		{
			ID:      "ghostty",
			Name:    "Ghostty",
			Exec:    []string{"ghostty", "--working-directory={}"},
			ExecRun: []string{"ghostty", "-e", "sh", "-c", "{}"},
			Enabled: checkCommandExists("ghostty"),
		},
		{
			ID:      "kitty",
			Name:    "Kitty",
			Exec:    []string{"kitty", "--directory={}"},
			ExecRun: []string{"kitty", "sh", "-c", "{}"},
			Enabled: checkCommandExists("kitty"),
		},
		{
			ID:      "wezterm",
			Name:    "WezTerm",
			Exec:    []string{"wezterm", "start", "--cwd", "{}"},
			ExecRun: []string{"wezterm", "start", "--", "sh", "-c", "{}"},
			Enabled: checkCommandExists("wezterm"),
		},
		{
			ID:      "alacritty",
			Name:    "Alacritty",
			Exec:    []string{"alacritty", "--working-directory", "{}"},
			ExecRun: []string{"alacritty", "-e", "sh", "-c", "{}"},
			Enabled: checkCommandExists("alacritty"),
		},
	}
	return filterAvailable(append(apps, commonApps()...))
}
