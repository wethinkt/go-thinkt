// Package theme provides theming support for the TUI.
package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Theme defines all colors used in the TUI.
type Theme struct {
	// Accent colors
	AccentPrimary  string `json:"accent_primary"`  // Primary accent (active borders, status bar bg, separators)
	BorderInactive string `json:"border_inactive"` // Inactive borders, viewer border

	// Text colors
	TextPrimary   string `json:"text_primary"`   // Main text (status bar, titles)
	TextSecondary string `json:"text_secondary"` // Secondary text (info, indicators)
	TextMuted     string `json:"text_muted"`     // Muted text (help text)

	// Block backgrounds
	UserBlockBg       string `json:"user_block_bg"`
	AssistantBlockBg  string `json:"assistant_block_bg"`
	ThinkingBlockBg   string `json:"thinking_block_bg"`
	ToolCallBlockBg   string `json:"tool_call_block_bg"`
	ToolResultBlockBg string `json:"tool_result_block_bg"`

	// Block foregrounds
	UserBlockFg       string `json:"user_block_fg"`
	AssistantBlockFg  string `json:"assistant_block_fg"`
	ThinkingBlockFg   string `json:"thinking_block_fg"`
	ToolCallBlockFg   string `json:"tool_call_block_fg"`
	ToolResultBlockFg string `json:"tool_result_block_fg"`

	// Label colors
	UserLabel      string `json:"user_label"`
	AssistantLabel string `json:"assistant_label"`
	ThinkingLabel  string `json:"thinking_label"`
	ToolLabel      string `json:"tool_label"`

	// Confirm dialog colors
	ConfirmPromptFg     string `json:"confirm_prompt_fg"`
	ConfirmSelectedFg   string `json:"confirm_selected_fg"`
	ConfirmSelectedBg   string `json:"confirm_selected_bg"`
	ConfirmUnselectedFg string `json:"confirm_unselected_fg"`
}

// DefaultTheme returns the default dark theme.
func DefaultTheme() Theme {
	return Theme{
		// Accent colors
		AccentPrimary:  "#7D56F4",
		BorderInactive: "#444444",

		// Text colors
		TextPrimary:   "#ffffff",
		TextSecondary: "#888888",
		TextMuted:     "#666666",

		// Block backgrounds
		UserBlockBg:       "#1a3a5c",
		AssistantBlockBg:  "#1a3c1a",
		ThinkingBlockBg:   "#3a1a3c",
		ToolCallBlockBg:   "#3c2a1a",
		ToolResultBlockBg: "#1a2a3c",

		// Block foregrounds
		UserBlockFg:       "#e0e0e0",
		AssistantBlockFg:  "#e0e0e0",
		ThinkingBlockFg:   "#c0a0c0",
		ToolCallBlockFg:   "#e0c080",
		ToolResultBlockFg: "#a0c0e0",

		// Label colors
		UserLabel:      "#5dade2",
		AssistantLabel: "#58d68d",
		ThinkingLabel:  "#af7ac5",
		ToolLabel:      "#f0b27a",

		// Confirm dialog colors
		ConfirmPromptFg:     "#ffffff",
		ConfirmSelectedFg:   "#000000",
		ConfirmSelectedBg:   "#ff87d7",
		ConfirmUnselectedFg: "#9e9e9e",
	}
}

// ThemeDir returns the path to the .thinkt directory.
func ThemeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".thinkt"), nil
}

// ThemePath returns the full path to the theme file.
func ThemePath() (string, error) {
	dir, err := ThemeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "thinkt-theme.json"), nil
}

// Load loads the theme from ~/.thinkt/thinkt-theme.json.
// If the file doesn't exist, it creates it with default values.
// If the file exists but has missing fields, the defaults are used for those fields.
func Load() (Theme, error) {
	themePath, err := ThemePath()
	if err != nil {
		return DefaultTheme(), err
	}

	// Start with defaults
	theme := DefaultTheme()

	// Check if file exists
	data, err := os.ReadFile(themePath)
	if os.IsNotExist(err) {
		// Create the theme file with defaults
		if err := Save(theme); err != nil {
			return theme, err
		}
		return theme, nil
	} else if err != nil {
		return theme, err
	}

	// Parse existing theme (defaults remain for missing fields)
	if err := json.Unmarshal(data, &theme); err != nil {
		return DefaultTheme(), err
	}

	return theme, nil
}

// Save writes the theme to ~/.thinkt/thinkt-theme.json.
func Save(theme Theme) error {
	themePath, err := ThemePath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(themePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write theme as formatted JSON
	data, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(themePath, data, 0644)
}

// current holds the loaded theme (initialized on first access).
var current *Theme

// Current returns the current theme, loading it if necessary.
func Current() Theme {
	if current == nil {
		theme, _ := Load() // Ignore errors, use defaults
		current = &theme
	}
	return *current
}

// Reload forces a reload of the theme from disk.
func Reload() (Theme, error) {
	theme, err := Load()
	if err != nil {
		return theme, err
	}
	current = &theme
	return theme, nil
}

