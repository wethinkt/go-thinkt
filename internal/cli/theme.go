// Package cli provides CLI output formatting utilities.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// ThemeDisplay handles theme visualization in the terminal.
type ThemeDisplay struct {
	w     io.Writer
	theme theme.Theme
}

// NewThemeDisplay creates a new theme display formatter.
func NewThemeDisplay(w io.Writer, t theme.Theme) *ThemeDisplay {
	return &ThemeDisplay{w: w, theme: t}
}

// themeEntry represents a single theme color entry for display.
type themeEntry struct {
	Name       string
	Color      string
	Category   string
	SampleText string
	IsBg       bool // true if this is a background color (needs contrasting fg)
}

// Show displays the current theme with styled samples.
func (d *ThemeDisplay) Show() error {
	t := d.theme

	// Define all theme entries grouped by category
	entries := []themeEntry{
		// Accent colors
		{Name: "Accent", Color: t.GetAccent(), Category: "Accent", SampleText: "▌Active Border"},
		{Name: "BorderActive", Color: t.GetBorderActive(), Category: "Accent", SampleText: "▌Active Border"},
		{Name: "BorderInactive", Color: t.GetBorderInactive(), Category: "Accent", SampleText: "│ Inactive Border"},

		// Text colors
		{Name: "TextPrimary", Color: t.TextPrimary.Fg, Category: "Text", SampleText: "Primary Text"},
		{Name: "TextSecondary", Color: t.TextSecondary.Fg, Category: "Text", SampleText: "Secondary info text"},
		{Name: "TextMuted", Color: t.TextMuted.Fg, Category: "Text", SampleText: "Muted help text"},

		// Block backgrounds (with their foregrounds)
		{Name: "UserBlock", Color: t.UserBlock.Bg, Category: "Blocks", SampleText: " User message ", IsBg: true},
		{Name: "AssistantBlock", Color: t.AssistantBlock.Bg, Category: "Blocks", SampleText: " Assistant response ", IsBg: true},
		{Name: "ThinkingBlock", Color: t.ThinkingBlock.Bg, Category: "Blocks", SampleText: " Thinking... ", IsBg: true},
		{Name: "ToolCallBlock", Color: t.ToolCallBlock.Bg, Category: "Blocks", SampleText: " Tool: Read file ", IsBg: true},
		{Name: "ToolResultBlock", Color: t.ToolResultBlock.Bg, Category: "Blocks", SampleText: " Result: success ", IsBg: true},

		// Labels
		{Name: "UserLabel", Color: t.UserLabel.Fg, Category: "Labels", SampleText: "USER"},
		{Name: "AssistantLabel", Color: t.AssistantLabel.Fg, Category: "Labels", SampleText: "ASSISTANT"},
		{Name: "ThinkingLabel", Color: t.ThinkingLabel.Fg, Category: "Labels", SampleText: "THINKING"},
		{Name: "ToolLabel", Color: t.ToolLabel.Fg, Category: "Labels", SampleText: "TOOL"},

		// Confirm dialog
		{Name: "ConfirmPrompt", Color: t.ConfirmPrompt.Fg, Category: "Confirm", SampleText: "Delete this file?"},
		{Name: "ConfirmSelected", Color: t.ConfirmSelected.Bg, Category: "Confirm", SampleText: " Yes ", IsBg: true},
		{Name: "ConfirmUnselected", Color: t.ConfirmUnselected.Fg, Category: "Confirm", SampleText: " No "},
	}

	// Get block foreground colors for background entries
	blockFg := map[string]string{
		"UserBlock":       t.UserBlock.Fg,
		"AssistantBlock":  t.AssistantBlock.Fg,
		"ThinkingBlock":   t.ThinkingBlock.Fg,
		"ToolCallBlock":   t.ToolCallBlock.Fg,
		"ToolResultBlock": t.ToolResultBlock.Fg,
		"ConfirmSelected": t.ConfirmSelected.Fg,
	}

	// Print header with active theme info
	activeName := theme.ActiveName()
	themesDir, _ := theme.ThemesDir()

	fmt.Fprintf(d.w, "Active Theme: %s\n", activeName)
	if t.Description != "" {
		fmt.Fprintf(d.w, "Description:  %s\n", t.Description)
	}
	fmt.Fprintf(d.w, "Themes Dir:   %s\n\n", themesDir)

	// Group and display entries
	currentCategory := ""
	for _, entry := range entries {
		// Print category header
		if entry.Category != currentCategory {
			if currentCategory != "" {
				fmt.Fprintln(d.w)
			}
			categoryStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
			fmt.Fprintf(d.w, "%s\n", categoryStyle.Render(entry.Category))
			fmt.Fprintf(d.w, "%s\n", strings.Repeat("─", len(entry.Category)+2))
			currentCategory = entry.Category
		}

		// Create styled sample
		var sample string
		if entry.IsBg {
			fg := blockFg[entry.Name]
			if fg == "" {
				fg = "#ffffff"
			}
			style := lipgloss.NewStyle().
				Background(lipgloss.Color(entry.Color)).
				Foreground(lipgloss.Color(fg))
			sample = style.Render(entry.SampleText)
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(entry.Color))
			sample = style.Render(entry.SampleText)
		}

		// Print entry: name, hex value, and sample
		nameStyle := lipgloss.NewStyle().Width(20)
		colorStyle := lipgloss.NewStyle().Width(10).Foreground(lipgloss.Color(t.TextMuted.Fg))

		fmt.Fprintf(d.w, "  %s %s %s\n",
			nameStyle.Render(entry.Name),
			colorStyle.Render(entry.Color),
			sample,
		)
	}

	fmt.Fprintln(d.w)

	// Render conversation preview (same mock data as theme browser)
	d.renderPreview(t)

	return nil
}

// renderPreview renders a mock conversation using the theme's styles.
func (d *ThemeDisplay) renderPreview(t theme.Theme) {
	categoryStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	fmt.Fprintf(d.w, "%s\n", categoryStyle.Render("Preview"))
	fmt.Fprintf(d.w, "%s\n\n", strings.Repeat("─", len("Preview")+2))

	// Color swatches
	d.renderSwatches(t)
	fmt.Fprintln(d.w)

	width := 72

	apply := func(s lipgloss.Style, ts theme.Style) lipgloss.Style {
		if ts.Fg != "" {
			s = s.Foreground(lipgloss.Color(ts.Fg))
		}
		if ts.Bg != "" {
			s = s.Background(lipgloss.Color(ts.Bg))
		}
		if ts.Bold {
			s = s.Bold(true)
		}
		if ts.Italic {
			s = s.Italic(true)
		}
		if ts.Underline {
			s = s.Underline(true)
		}
		return s
	}

	userLabel := apply(lipgloss.NewStyle(), t.UserLabel)
	userBlock := apply(lipgloss.NewStyle(), t.UserBlock).Padding(0, 1).Width(width)
	assistantLabel := apply(lipgloss.NewStyle(), t.AssistantLabel)
	assistantBlock := apply(lipgloss.NewStyle(), t.AssistantBlock).Padding(0, 1).Width(width)
	thinkingLabel := apply(lipgloss.NewStyle(), t.ThinkingLabel)
	thinkingBlock := apply(lipgloss.NewStyle(), t.ThinkingBlock).Padding(0, 1).Width(width)
	toolLabel := apply(lipgloss.NewStyle(), t.ToolLabel)
	toolCallBlock := apply(lipgloss.NewStyle(), t.ToolCallBlock).Padding(0, 1).Width(width)
	toolResultBlock := apply(lipgloss.NewStyle(), t.ToolResultBlock).Padding(0, 1).Width(width)

	// Mock conversation (matches theme.MockEntries)
	fmt.Fprintln(d.w, userLabel.Render("User"))
	fmt.Fprintln(d.w, userBlock.Render("Can you help me understand how this code works?"))
	fmt.Fprintln(d.w)
	fmt.Fprintln(d.w, thinkingLabel.Render("Thinking"))
	fmt.Fprintln(d.w, thinkingBlock.Render("Let me analyze the code structure and identify the key components..."))
	fmt.Fprintln(d.w)
	fmt.Fprintln(d.w, assistantLabel.Render("Assistant"))
	fmt.Fprintln(d.w, assistantBlock.Render("I'll explain the code structure. Let me first read the main file to understand the architecture."))
	fmt.Fprintln(d.w)
	fmt.Fprintln(d.w, toolLabel.Render("Tool: Read"))
	fmt.Fprintln(d.w, toolCallBlock.Render("file_path: /src/main.go"))
	fmt.Fprintln(d.w)
	fmt.Fprintln(d.w, toolLabel.Render("Tool Result"))
	fmt.Fprintln(d.w, toolResultBlock.Render("package main\n\nfunc main() {\n    // ...\n}"))
	fmt.Fprintln(d.w)
	fmt.Fprintln(d.w, assistantLabel.Render("Assistant"))
	fmt.Fprintln(d.w, assistantBlock.Render("This is a simple Go application with a main function. The structure follows standard Go conventions."))
	fmt.Fprintln(d.w)
}

// renderSwatches renders inline color swatches (same as theme browser).
func (d *ThemeDisplay) renderSwatches(t theme.Theme) {
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	var parts []string

	swatches := []struct {
		label string
		color string
	}{
		{"accent", t.GetAccent()},
		{"border", t.GetBorderActive()},
		{"text", t.TextPrimary.Fg},
		{"muted", t.TextMuted.Fg},
	}

	for _, s := range swatches {
		if s.color != "" {
			swatch := lipgloss.NewStyle().Background(lipgloss.Color(s.color)).Render("  ")
			parts = append(parts, mutedStyle.Render(s.label+" ")+swatch)
		}
	}

	fmt.Fprintf(d.w, "  %s\n", strings.Join(parts, "  "))
}

// ShowJSON displays the theme as JSON.
func (d *ThemeDisplay) ShowJSON() error {
	fmt.Fprintf(d.w, "{\n")
	t := d.theme

	fields := []struct {
		name  string
		value string
	}{
		{"name", t.Name},
		{"description", t.Description},
		{"accent", t.GetAccent()},
		{"border_active", t.GetBorderActive()},
		{"border_inactive", t.GetBorderInactive()},
		{"text_primary.fg", t.TextPrimary.Fg},
		{"text_secondary.fg", t.TextSecondary.Fg},
		{"text_muted.fg", t.TextMuted.Fg},
		{"user_block.fg", t.UserBlock.Fg},
		{"user_block.bg", t.UserBlock.Bg},
		{"assistant_block.fg", t.AssistantBlock.Fg},
		{"assistant_block.bg", t.AssistantBlock.Bg},
		{"thinking_block.fg", t.ThinkingBlock.Fg},
		{"thinking_block.bg", t.ThinkingBlock.Bg},
		{"tool_call_block.fg", t.ToolCallBlock.Fg},
		{"tool_call_block.bg", t.ToolCallBlock.Bg},
		{"tool_result_block.fg", t.ToolResultBlock.Fg},
		{"tool_result_block.bg", t.ToolResultBlock.Bg},
		{"user_label.fg", t.UserLabel.Fg},
		{"assistant_label.fg", t.AssistantLabel.Fg},
		{"thinking_label.fg", t.ThinkingLabel.Fg},
		{"tool_label.fg", t.ToolLabel.Fg},
		{"confirm_prompt.fg", t.ConfirmPrompt.Fg},
		{"confirm_selected.fg", t.ConfirmSelected.Fg},
		{"confirm_selected.bg", t.ConfirmSelected.Bg},
		{"confirm_unselected.fg", t.ConfirmUnselected.Fg},
	}

	for i, f := range fields {
		comma := ","
		if i == len(fields)-1 {
			comma = ""
		}
		fmt.Fprintf(d.w, "  %q: %q%s\n", f.name, f.value, comma)
	}

	fmt.Fprintf(d.w, "}\n")
	return nil
}

// ListThemes displays all available themes.
func ListThemes(w io.Writer) error {
	themes, err := theme.ListAvailable()
	if err != nil {
		return err
	}

	activeName := theme.ActiveName()

	fmt.Fprintln(w, "Available Themes:")
	fmt.Fprintln(w)

	for _, t := range themes {
		marker := "  "
		if t.Name == activeName {
			marker = "* "
		}

		source := "built-in"
		if !t.Embedded {
			source = "user"
		}

		fmt.Fprintf(w, "%s%-12s  %-10s  %s\n", marker, t.Name, "("+source+")", t.Description)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Active theme marked with *\n")
	fmt.Fprintf(w, "Use 'thinkt theme set <name>' to change theme\n")

	return nil
}

// ListThemesJSON outputs all available themes as JSON.
func ListThemesJSON(w io.Writer) error {
	themes, err := theme.ListAvailable()
	if err != nil {
		return err
	}

	activeName := theme.ActiveName()

	type themeJSON struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Active      bool   `json:"active"`
		Path        string `json:"path,omitempty"`
	}

	var out []themeJSON
	for _, t := range themes {
		source := "built-in"
		if !t.Embedded {
			source = "user"
		}
		out = append(out, themeJSON{
			Name:        t.Name,
			Description: t.Description,
			Source:      source,
			Active:      t.Name == activeName,
			Path:        t.Path,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(data))
	return nil
}
