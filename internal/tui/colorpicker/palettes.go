package colorpicker

// Palette represents a named collection of 16 colors.
type Palette struct {
	Name   string
	Colors [16]string // 16 colors in a 4x4 grid
}

// Palettes contains pre-defined color palettes for quick selection.
// Each palette has 16 colors organized in a 4x4 grid.
var Palettes = []Palette{
	// Catppuccin Mocha - A soothing pastel theme
	// https://github.com/catppuccin/catppuccin
	{
		Name: "Catppuccin Mocha",
		Colors: [16]string{
			// Row 1: Base colors
			"#1e1e2e", "#313244", "#45475a", "#585b70",
			// Row 2: Surface & overlay
			"#6c7086", "#7f849c", "#9399b2", "#a6adc8",
			// Row 3: Accent colors (warm)
			"#f38ba8", "#fab387", "#f9e2af", "#a6e3a1",
			// Row 4: Accent colors (cool)
			"#94e2d5", "#89dceb", "#89b4fa", "#cba6f7",
		},
	},

	// Dracula - A dark theme with vibrant colors
	// https://draculatheme.com
	{
		Name: "Dracula",
		Colors: [16]string{
			// Row 1: Background shades
			"#282a36", "#44475a", "#6272a4", "#f8f8f2",
			// Row 2: Primary colors
			"#ff5555", "#ffb86c", "#f1fa8c", "#50fa7b",
			// Row 3: Secondary colors
			"#8be9fd", "#bd93f9", "#ff79c6", "#f8f8f2",
			// Row 4: Muted variants
			"#ff6e6e", "#ffc990", "#f4fb9c", "#6ffc94",
		},
	},

	// Nord - An arctic, north-bluish color palette
	// https://www.nordtheme.com
	{
		Name: "Nord",
		Colors: [16]string{
			// Row 1: Polar Night (dark backgrounds)
			"#2e3440", "#3b4252", "#434c5e", "#4c566a",
			// Row 2: Snow Storm (light foregrounds)
			"#d8dee9", "#e5e9f0", "#eceff4", "#8fbcbb",
			// Row 3: Frost (cool accents)
			"#88c0d0", "#81a1c1", "#5e81ac", "#bf616a",
			// Row 4: Aurora (warm accents)
			"#d08770", "#ebcb8b", "#a3be8c", "#b48ead",
		},
	},

	// Gruvbox Dark - Retro groove color scheme
	// https://github.com/morhetz/gruvbox
	{
		Name: "Gruvbox Dark",
		Colors: [16]string{
			// Row 1: Background shades
			"#1d2021", "#282828", "#3c3836", "#504945",
			// Row 2: Foreground shades
			"#665c54", "#928374", "#a89984", "#ebdbb2",
			// Row 3: Bright colors
			"#fb4934", "#fe8019", "#fabd2f", "#b8bb26",
			// Row 4: More bright colors
			"#8ec07c", "#83a598", "#d3869b", "#d65d0e",
		},
	},

	// Tokyo Night - A dark VS Code theme
	// https://github.com/enkia/tokyo-night-vscode-theme
	{
		Name: "Tokyo Night",
		Colors: [16]string{
			// Row 1: Background shades
			"#1a1b26", "#24283b", "#414868", "#565f89",
			// Row 2: Foreground shades
			"#a9b1d6", "#c0caf5", "#cfc9c2", "#787c99",
			// Row 3: Primary accents
			"#f7768e", "#ff9e64", "#e0af68", "#9ece6a",
			// Row 4: Secondary accents
			"#73daca", "#7dcfff", "#7aa2f7", "#bb9af7",
		},
	},
}

// GetPalette returns a palette by name, or nil if not found.
func GetPalette(name string) *Palette {
	for i := range Palettes {
		if Palettes[i].Name == name {
			return &Palettes[i]
		}
	}
	return nil
}

// PaletteNames returns the names of all available palettes.
func PaletteNames() []string {
	names := make([]string, len(Palettes))
	for i, p := range Palettes {
		names[i] = p.Name
	}
	return names
}
