// Package theme provides theming support for the TUI.
package theme

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/config"
)

//go:embed themes/*.json
var embeddedThemes embed.FS

// Style defines colors and text attributes for a UI element.
type Style struct {
	Fg        string `json:"fg,omitempty"`
	Bg        string `json:"bg,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
}

// Theme defines all styles used in the TUI.
type Theme struct {
	// Metadata
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// UI chrome - accent colors for borders, highlights
	Accent         string `json:"accent,omitempty"`          // Primary accent (active elements)
	BorderActive   string `json:"border_active,omitempty"`   // Active/focused borders
	BorderInactive string `json:"border_inactive,omitempty"` // Inactive borders

	// Text styles (typically fg-only, on terminal default bg)
	TextPrimary   Style `json:"text_primary,omitempty"`
	TextSecondary Style `json:"text_secondary,omitempty"`
	TextMuted     Style `json:"text_muted,omitempty"`

	// Conversation blocks (fg + bg)
	UserBlock       Style `json:"user_block,omitempty"`
	AssistantBlock  Style `json:"assistant_block,omitempty"`
	ThinkingBlock   Style `json:"thinking_block,omitempty"`
	ToolCallBlock   Style `json:"tool_call_block,omitempty"`
	ToolResultBlock Style `json:"tool_result_block,omitempty"`

	// Labels (typically fg-only or fg + bold)
	UserLabel      Style `json:"user_label,omitempty"`
	AssistantLabel Style `json:"assistant_label,omitempty"`
	ThinkingLabel  Style `json:"thinking_label,omitempty"`
	ToolLabel      Style `json:"tool_label,omitempty"`
	OtherLabel     Style `json:"other_label,omitempty"`

	// Confirm dialog
	ConfirmPrompt     Style `json:"confirm_prompt,omitempty"`
	ConfirmSelected   Style `json:"confirm_selected,omitempty"`
	ConfirmUnselected Style `json:"confirm_unselected,omitempty"`
}

// ThemeMeta holds metadata about an available theme.
type ThemeMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`     // File path (empty for embedded)
	Embedded    bool   `json:"embedded"` // True if this is a built-in theme
}

// DefaultTheme returns the default dark theme (embedded fallback).
func DefaultTheme() Theme {
	theme, _ := LoadEmbedded("dark")
	return theme
}

// LoadEmbedded loads a theme from the embedded themes.
func LoadEmbedded(name string) (Theme, error) {
	data, err := embeddedThemes.ReadFile("themes/" + name + ".json")
	if err != nil {
		return Theme{}, err
	}

	var theme Theme
	if err := json.Unmarshal(data, &theme); err != nil {
		return Theme{}, err
	}

	return theme, nil
}

// ListEmbedded returns the names of all embedded themes.
func ListEmbedded() []string {
	entries, err := embeddedThemes.ReadDir("themes")
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			names = append(names, name)
		}
	}
	return names
}

// ThemesDir returns the path to the themes directory.
func ThemesDir() (string, error) {
	configDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "themes"), nil
}

// ListAvailable returns all available themes (embedded + user themes).
func ListAvailable() ([]ThemeMeta, error) {
	var themes []ThemeMeta

	// Add embedded themes
	for _, name := range ListEmbedded() {
		theme, err := LoadEmbedded(name)
		if err != nil {
			continue
		}
		themes = append(themes, ThemeMeta{
			Name:        name,
			Description: theme.Description,
			Embedded:    true,
		})
	}

	// Add user themes from ~/.thinkt/themes/
	themesDir, err := ThemesDir()
	if err == nil {
		entries, err := os.ReadDir(themesDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}

				name := strings.TrimSuffix(entry.Name(), ".json")
				path := filepath.Join(themesDir, entry.Name())

				// Try to load and get description
				description := "User theme"
				if data, err := os.ReadFile(path); err == nil {
					var t Theme
					if json.Unmarshal(data, &t) == nil && t.Description != "" {
						description = t.Description
					}
				}

				themes = append(themes, ThemeMeta{
					Name:        name,
					Description: description,
					Path:        path,
					Embedded:    false,
				})
			}
		}
	}

	return themes, nil
}

// LoadByName loads a theme by name, checking user themes first, then embedded.
func LoadByName(name string) (Theme, error) {
	// First, check user themes directory
	themesDir, err := ThemesDir()
	if err == nil {
		userPath := filepath.Join(themesDir, name+".json")
		if data, err := os.ReadFile(userPath); err == nil {
			theme := DefaultTheme() // Start with defaults for missing fields
			if err := json.Unmarshal(data, &theme); err == nil {
				theme.Name = name
				return theme, nil
			}
		}
	}

	// Fall back to embedded themes
	return LoadEmbedded(name)
}

// Load loads the currently configured theme.
// Falls back to embedded dark theme if anything fails.
func Load() (Theme, error) {
	cfg, err := config.Load()
	if err != nil {
		return DefaultTheme(), err
	}

	theme, err := LoadByName(cfg.Theme)
	if err != nil {
		return DefaultTheme(), err
	}

	return theme, nil
}

// Save writes a theme to the user themes directory.
func Save(name string, theme Theme) error {
	themesDir, err := ThemesDir()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		return err
	}

	theme.Name = name
	data, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(themesDir, name+".json")
	return os.WriteFile(path, data, 0644)
}

// SetActive sets the active theme in the config.
func SetActive(name string) error {
	// Verify the theme exists
	if _, err := LoadByName(name); err != nil {
		return err
	}

	cfg, _ := config.Load()
	cfg.Theme = name
	return config.Save(cfg)
}

// ActiveName returns the name of the currently active theme.
func ActiveName() string {
	cfg, _ := config.Load()
	return cfg.Theme
}

// EnsureUserThemesDir creates the themes directory and copies embedded themes if needed.
func EnsureUserThemesDir() error {
	themesDir, err := ThemesDir()
	if err != nil {
		return err
	}

	return os.MkdirAll(themesDir, 0755)
}

// current holds the loaded theme (initialized on first access).
var current *Theme

// Current returns the current theme, loading it if necessary.
func Current() Theme {
	if current == nil {
		theme, _ := Load()
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

// Helper methods for backward compatibility and convenience

// GetAccent returns the accent color, with fallback.
func (t Theme) GetAccent() string {
	if t.Accent != "" {
		return t.Accent
	}
	return "#7D56F4"
}

// GetBorderActive returns the active border color.
func (t Theme) GetBorderActive() string {
	if t.BorderActive != "" {
		return t.BorderActive
	}
	return t.GetAccent()
}

// GetBorderInactive returns the inactive border color.
func (t Theme) GetBorderInactive() string {
	if t.BorderInactive != "" {
		return t.BorderInactive
	}
	return "#444444"
}
