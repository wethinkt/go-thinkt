package config

// commonApps returns cross-platform app configurations for editors and terminals.
func commonApps() []AppConfig {
	return []AppConfig{
		// Editors
		{
			ID:      "vscode",
			Name:    "VS Code",
			Exec:    []string{"code", "{}"},
			Enabled: checkCommandExists("code"),
		},
		{
			ID:      "cursor",
			Name:    "Cursor",
			Exec:    []string{"cursor", "{}"},
			Enabled: checkCommandExists("cursor"),
		},
		{
			ID:      "zed",
			Name:    "Zed",
			Exec:    []string{"zed", "{}"},
			Enabled: checkCommandExists("zed"),
		},
		{
			ID:      "windsurf",
			Name:    "Windsurf",
			Exec:    []string{"windsurf", "{}"},
			Enabled: checkCommandExists("windsurf"),
		},
		{
			ID:      "sublime",
			Name:    "Sublime Text",
			Exec:    []string{"subl", "{}"},
			Enabled: checkCommandExists("subl"),
		},
		{
			ID:      "antigravity",
			Name:    "Antigravity",
			Exec:    []string{"agy", "{}"},
			Enabled: checkCommandExists("agy"),
		},
		{
			ID:      "opencode",
			Name:    "OpenCode",
			Exec:    []string{"opencode", "{}"},
			Enabled: checkCommandExists("opencode"),
		},
		{
			ID:      "nvim",
			Name:    "Neovim",
			Exec:    []string{"nvim", "{}"},
			Enabled: checkCommandExists("nvim"),
		},
		// Terminals
		{
			ID:      "ghostty",
			Name:    "Ghostty",
			Exec:    []string{"ghostty", "{}"},
			Enabled: checkCommandExists("ghostty"),
		},
		{
			ID:      "kitty",
			Name:    "Kitty",
			Exec:    []string{"kitty", "{}"},
			Enabled: checkCommandExists("kitty"),
		},
		{
			ID:      "wezterm",
			Name:    "WezTerm",
			Exec:    []string{"wezterm", "{}"},
			Enabled: checkCommandExists("wezterm"),
		},
		{
			ID:      "alacritty",
			Name:    "Alacritty",
			Exec:    []string{"alacritty", "{}"},
			Enabled: checkCommandExists("alacritty"),
		},
	}
}
