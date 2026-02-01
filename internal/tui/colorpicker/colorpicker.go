// Package colorpicker provides an interactive terminal color picker component.
//
// The colorpicker can be embedded in other TUI applications. It supports three modes:
//   - Sliders: RGB sliders with keyboard controls
//   - Hex: Direct hex input
//   - Palette: Quick selection from curated color palettes
//
// Example usage in a bubbletea Update function:
//
//	case tea.KeyMsg:
//	    m.picker.HandleKey(msg.String())
//	    if m.picker.Confirmed {
//	        color := m.picker.Value()
//	    }
package colorpicker

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// Mode represents the current picker mode
type Mode int

const (
	ModeSliders Mode = iota // RGB slider mode
	ModeHex                 // Hex input mode
	ModePalette             // Palette selection mode
)

// Channel represents an RGB channel
type Channel int

const (
	ChannelR Channel = iota
	ChannelG
	ChannelB
)

// Model is the color picker model.
type Model struct {
	// Current color as RGB values
	R, G, B int

	// Original color (for reset)
	OrigR, OrigG, OrigB int

	// UI state
	Mode         Mode
	Channel      Channel // Selected RGB channel in slider mode
	HexInput     string  // Current hex input string
	HexCursor    int     // Cursor position in hex input
	PaletteIndex int     // Selected palette (0-4)
	ColorIndex   int     // Selected color in palette (0-15)

	// Dimensions
	Width  int
	Height int

	// Styling
	AccentColor string
	MutedColor  string

	// Result state
	Confirmed bool
	Cancelled bool
}

// New creates a new color picker with the given initial color.
func New(hexColor string) Model {
	r, g, b := HexToRGB(hexColor)
	return Model{
		R: r, G: g, B: b,
		OrigR: r, OrigG: g, OrigB: b,
		Mode:        ModeSliders,
		HexInput:    hexColor,
		AccentColor: "#7D56F4",
		MutedColor:  "#666666",
		Width:       40,
		Height:      20,
	}
}

// Value returns the current color as a hex string.
func (m Model) Value() string {
	return RGBToHex(m.R, m.G, m.B)
}

// Reset restores the picker to its original color.
func (m *Model) Reset() {
	m.R, m.G, m.B = m.OrigR, m.OrigG, m.OrigB
	m.HexInput = RGBToHex(m.R, m.G, m.B)
}

// SetColor sets the current color from a hex string.
func (m *Model) SetColor(hex string) {
	m.R, m.G, m.B = HexToRGB(hex)
	m.HexInput = hex
}

// SetOriginal sets the original color (used for reset).
func (m *Model) SetOriginal(hex string) {
	m.OrigR, m.OrigG, m.OrigB = HexToRGB(hex)
}

// HandleKey processes a key press and returns true if the key was handled.
func (m *Model) HandleKey(key string) bool {
	// Global keys
	switch key {
	case "enter":
		m.Confirmed = true
		return true
	case "esc":
		m.Cancelled = true
		return true
	case "r":
		// Reset to original
		m.Reset()
		return true
	case "tab":
		// Cycle through modes
		m.Mode = (m.Mode + 1) % 3
		if m.Mode == ModeHex {
			m.HexInput = RGBToHex(m.R, m.G, m.B)
			m.HexCursor = len(m.HexInput)
		}
		return true
	}

	// Mode-specific handling
	switch m.Mode {
	case ModeSliders:
		return m.handleSliderKey(key)
	case ModeHex:
		return m.handleHexKey(key)
	case ModePalette:
		return m.handlePaletteKey(key)
	}

	return false
}

func (m *Model) handleSliderKey(key string) bool {
	switch key {
	case "up", "k":
		if m.Channel > ChannelR {
			m.Channel--
		}
		return true
	case "down", "j":
		if m.Channel < ChannelB {
			m.Channel++
		}
		return true
	case "left", "h":
		m.adjustChannel(-13) // ~5%
		return true
	case "right", "l":
		m.adjustChannel(13)
		return true
	case "H": // Shift+h
		m.adjustChannel(-1)
		return true
	case "L": // Shift+l
		m.adjustChannel(1)
		return true
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Quick set: 0=0, 1=28, 2=57, ... 9=255
		n, _ := strconv.Atoi(key)
		val := n * 255 / 9
		if n == 0 {
			val = 0
		}
		m.setChannel(val)
		return true
	}
	return false
}

func (m *Model) adjustChannel(delta int) {
	val := m.getChannel() + delta
	m.setChannel(clamp(val, 0, 255))
	m.HexInput = RGBToHex(m.R, m.G, m.B)
}

func (m *Model) getChannel() int {
	switch m.Channel {
	case ChannelR:
		return m.R
	case ChannelG:
		return m.G
	case ChannelB:
		return m.B
	}
	return 0
}

func (m *Model) setChannel(val int) {
	switch m.Channel {
	case ChannelR:
		m.R = val
	case ChannelG:
		m.G = val
	case ChannelB:
		m.B = val
	}
	m.HexInput = RGBToHex(m.R, m.G, m.B)
}

func (m *Model) handleHexKey(key string) bool {
	switch key {
	case "left":
		if m.HexCursor > 0 {
			m.HexCursor--
		}
		return true
	case "right":
		if m.HexCursor < len(m.HexInput) {
			m.HexCursor++
		}
		return true
	case "backspace":
		if m.HexCursor > 0 {
			m.HexInput = m.HexInput[:m.HexCursor-1] + m.HexInput[m.HexCursor:]
			m.HexCursor--
			m.tryParseHex()
		}
		return true
	case "delete":
		if m.HexCursor < len(m.HexInput) {
			m.HexInput = m.HexInput[:m.HexCursor] + m.HexInput[m.HexCursor+1:]
			m.tryParseHex()
		}
		return true
	default:
		// Try to insert character if it's valid hex
		if len(key) == 1 && isHexChar(key[0]) && len(m.HexInput) < 7 {
			m.HexInput = m.HexInput[:m.HexCursor] + key + m.HexInput[m.HexCursor:]
			m.HexCursor++
			m.tryParseHex()
			return true
		} else if key == "#" && m.HexCursor == 0 && !strings.HasPrefix(m.HexInput, "#") {
			m.HexInput = "#" + m.HexInput
			m.HexCursor++
			return true
		}
	}
	return false
}

func (m *Model) tryParseHex() {
	if len(m.HexInput) == 7 && m.HexInput[0] == '#' {
		r, g, b := HexToRGB(m.HexInput)
		m.R, m.G, m.B = r, g, b
	}
}

func (m *Model) handlePaletteKey(key string) bool {
	switch key {
	case "up", "k":
		if m.ColorIndex >= 4 {
			m.ColorIndex -= 4
		}
		return true
	case "down", "j":
		if m.ColorIndex < 12 {
			m.ColorIndex += 4
		}
		return true
	case "left", "h":
		if m.ColorIndex > 0 {
			m.ColorIndex--
		}
		return true
	case "right", "l":
		if m.ColorIndex < 15 {
			m.ColorIndex++
		}
		return true
	case "[", "p": // Previous palette
		if m.PaletteIndex > 0 {
			m.PaletteIndex--
		} else {
			m.PaletteIndex = len(Palettes) - 1
		}
		return true
	case "]", "n": // Next palette
		m.PaletteIndex = (m.PaletteIndex + 1) % len(Palettes)
		return true
	case " ": // Select color
		color := Palettes[m.PaletteIndex].Colors[m.ColorIndex]
		m.R, m.G, m.B = HexToRGB(color)
		m.HexInput = color
		return true
	}
	return false
}

// View renders the color picker.
func (m Model) View() string {
	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.AccentColor))
	b.WriteString(titleStyle.Render("Color Picker") + "\n\n")

	// Current color preview
	currentHex := RGBToHex(m.R, m.G, m.B)
	preview := lipgloss.NewStyle().
		Background(lipgloss.Color(currentHex)).
		Foreground(lipgloss.Color(ContrastColor(m.R, m.G, m.B))).
		Padding(0, 4).
		Render(currentHex)

	origHex := RGBToHex(m.OrigR, m.OrigG, m.OrigB)
	origPreview := lipgloss.NewStyle().
		Background(lipgloss.Color(origHex)).
		Foreground(lipgloss.Color(ContrastColorHex(origHex))).
		Padding(0, 2).
		Render("orig")

	b.WriteString("Current: " + preview + "  " + origPreview + "\n\n")

	// Mode tabs
	b.WriteString(m.renderModeTabs() + "\n\n")

	// Mode content
	switch m.Mode {
	case ModeSliders:
		b.WriteString(m.renderSliders())
	case ModeHex:
		b.WriteString(m.renderHexInput())
	case ModePalette:
		b.WriteString(m.renderPalette())
	}

	// Help
	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.MutedColor))
	help := m.getHelp()
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) renderModeTabs() string {
	tabs := []string{"Sliders", "Hex", "Palette"}
	var parts []string

	for i, tab := range tabs {
		style := lipgloss.NewStyle().Padding(0, 1)
		if Mode(i) == m.Mode {
			style = style.Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(lipgloss.Color(m.AccentColor))
		} else {
			style = style.Foreground(lipgloss.Color(m.MutedColor))
		}
		parts = append(parts, style.Render(tab))
	}

	return strings.Join(parts, " ")
}

func (m Model) renderSliders() string {
	var b strings.Builder
	sliderWidth := 24

	channels := []struct {
		name string
		ch   Channel
		val  int
	}{
		{"R", ChannelR, m.R},
		{"G", ChannelG, m.G},
		{"B", ChannelB, m.B},
	}

	for _, c := range channels {
		// Indicator
		indicator := " "
		if m.Channel == c.ch {
			indicator = "▸"
		}

		// Slider bar
		filled := c.val * sliderWidth / 255
		empty := sliderWidth - filled

		// Color based on channel
		var fillHex string
		switch c.ch {
		case ChannelR:
			fillHex = fmt.Sprintf("#%02x0000", c.val)
		case ChannelG:
			fillHex = fmt.Sprintf("#00%02x00", c.val)
		case ChannelB:
			fillHex = fmt.Sprintf("#0000%02x", c.val)
		}

		filledStyle := lipgloss.NewStyle().Background(lipgloss.Color(fillHex))
		emptyStyle := lipgloss.NewStyle().Background(lipgloss.Color("#333333"))

		slider := filledStyle.Render(strings.Repeat(" ", filled)) +
			emptyStyle.Render(strings.Repeat(" ", empty))

		// Value
		valStyle := lipgloss.NewStyle()
		if m.Channel == c.ch {
			valStyle = valStyle.Bold(true).Foreground(lipgloss.Color(m.AccentColor))
		}

		b.WriteString(fmt.Sprintf("%s %s: %s %s\n",
			indicator,
			c.name,
			slider,
			valStyle.Render(fmt.Sprintf("%3d", c.val)),
		))
	}

	return b.String()
}

func (m Model) renderHexInput() string {
	var b strings.Builder

	b.WriteString("Enter hex color:\n\n")

	// Render input with cursor
	inputStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Padding(0, 1)

	// Build input string with cursor
	var inputStr string
	for i, ch := range m.HexInput {
		if i == m.HexCursor {
			inputStr += lipgloss.NewStyle().
				Background(lipgloss.Color(m.AccentColor)).
				Foreground(lipgloss.Color("#000000")).
				Render(string(ch))
		} else {
			inputStr += string(ch)
		}
	}
	if m.HexCursor >= len(m.HexInput) {
		inputStr += lipgloss.NewStyle().
			Background(lipgloss.Color(m.AccentColor)).
			Render(" ")
	}

	b.WriteString(inputStyle.Render(inputStr) + "\n\n")

	// Validation indicator
	if len(m.HexInput) == 7 && m.HexInput[0] == '#' {
		validStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
		b.WriteString(validStyle.Render("✓ Valid hex color"))
	} else {
		invalidStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
		b.WriteString(invalidStyle.Render("Format: #RRGGBB"))
	}

	return b.String()
}

func (m Model) renderPalette() string {
	var b strings.Builder

	palette := Palettes[m.PaletteIndex]

	// Palette name with navigation
	navStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.MutedColor))
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.AccentColor))

	b.WriteString(navStyle.Render("[p] ◀ ") +
		nameStyle.Render(palette.Name) +
		navStyle.Render(" ▶ [n]") + "\n\n")

	// Render 4x4 grid
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			idx := row*4 + col
			color := palette.Colors[idx]

			style := lipgloss.NewStyle().
				Background(lipgloss.Color(color)).
				Width(5).
				Align(lipgloss.Center)

			// Show selection indicator
			content := "  "
			if idx == m.ColorIndex {
				style = style.
					Foreground(lipgloss.Color(ContrastColorHex(color))).
					Bold(true)
				content = "▪▪"
			}

			b.WriteString(style.Render(content))
			if col < 3 {
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}

	// Show selected color info
	selectedColor := palette.Colors[m.ColorIndex]
	b.WriteString("\n")
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.MutedColor))
	b.WriteString(infoStyle.Render(fmt.Sprintf("Selected: %s (space to apply)", selectedColor)))

	return b.String()
}

func (m Model) getHelp() string {
	switch m.Mode {
	case ModeSliders:
		return "↑/↓: channel • h/l: ±5% • H/L: ±1 • 0-9: set • r: reset • tab: mode • enter: ok • esc: cancel"
	case ModeHex:
		return "type hex • ←/→: cursor • r: reset • tab: mode • enter: ok • esc: cancel"
	case ModePalette:
		return "↑/↓/←/→: select • p/n: palette • space: apply • r: reset • tab: mode • enter: ok • esc: cancel"
	}
	return ""
}

// Color conversion utilities - exported for reuse

// HexToRGB converts a hex color string to RGB values.
func HexToRGB(hex string) (int, int, int) {
	if len(hex) != 7 || hex[0] != '#' {
		return 128, 128, 128
	}
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return int(r), int(g), int(b)
}

// RGBToHex converts RGB values to a hex color string.
func RGBToHex(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// ContrastColor returns black or white depending on the luminance of the given RGB color.
func ContrastColor(r, g, b int) string {
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
	if luminance > 0.5 {
		return "#000000"
	}
	return "#ffffff"
}

// ContrastColorHex returns black or white depending on the luminance of the given hex color.
func ContrastColorHex(hex string) string {
	r, g, b := HexToRGB(hex)
	return ContrastColor(r, g, b)
}

// IsValidHex returns true if the string is a valid 7-character hex color.
func IsValidHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		if !isHexChar(s[i]) {
			return false
		}
	}
	return true
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
