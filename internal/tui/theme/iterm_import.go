package theme

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/tui/colorpicker"
)

// ParseItermColors parses an iTerm2 .itermcolors plist XML file and returns
// a map of color names to RGB float64 triples (0.0–1.0).
func ParseItermColors(r io.Reader) (map[string][3]float64, error) {
	decoder := xml.NewDecoder(r)
	colors := make(map[string][3]float64)

	// Navigate to the top-level <dict> inside <plist>
	if err := seekElement(decoder, "dict"); err != nil {
		return nil, fmt.Errorf("plist: missing top-level dict: %w", err)
	}

	// Parse key/dict pairs inside the top-level dict
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local == "key" {
			name, err := readText(decoder)
			if err != nil {
				return nil, err
			}

			// Expect a <dict> with the color components
			if err := seekElement(decoder, "dict"); err != nil {
				continue // skip non-dict values
			}

			rgb, err := parseColorDict(decoder)
			if err != nil {
				return nil, fmt.Errorf("plist: color %q: %w", name, err)
			}

			colors[name] = rgb
		}
	}

	if len(colors) == 0 {
		return nil, fmt.Errorf("plist: no colors found")
	}

	return colors, nil
}

// parseColorDict reads key/value pairs from inside a color <dict>,
// extracting only the Red/Green/Blue Component real values.
func parseColorDict(decoder *xml.Decoder) ([3]float64, error) {
	var rgb [3]float64
	depth := 1 // we're inside the dict

	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return rgb, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "key" {
				key, err := readText(decoder)
				if err != nil {
					return rgb, err
				}

				// Read the next value element (could be <real>, <string>, <integer>, etc.)
				valTag, err := nextStartElement(decoder)
				if err != nil {
					continue
				}
				valStr, err := readText(decoder)
				if err != nil {
					return rgb, err
				}

				// Only care about <real> values for RGB components
				if valTag != "real" {
					continue
				}

				var val float64
				if _, err := fmt.Sscanf(valStr, "%f", &val); err != nil {
					continue
				}

				switch key {
				case "Red Component":
					rgb[0] = val
				case "Green Component":
					rgb[1] = val
				case "Blue Component":
					rgb[2] = val
				}
			}
		case xml.EndElement:
			if t.Name.Local == "dict" {
				depth--
			}
		}
	}

	return rgb, nil
}

// nextStartElement advances until the next StartElement and returns its local name.
func nextStartElement(decoder *xml.Decoder) (string, error) {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return "", err
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}

// seekElement advances the decoder to the next StartElement with the given name.
func seekElement(decoder *xml.Decoder, name string) error {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return err
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == name {
			return nil
		}
	}
}

// readText reads the character data inside the current element until its end tag.
func readText(decoder *xml.Decoder) (string, error) {
	var buf strings.Builder
	for {
		tok, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.Write(t)
		case xml.EndElement:
			return strings.TrimSpace(buf.String()), nil
		}
	}
}

// floatToHex converts 0.0–1.0 RGB floats to a hex color string.
func floatToHex(r, g, b float64) string {
	ri := int(math.Round(clampF(r, 0, 1) * 255))
	gi := int(math.Round(clampF(g, 0, 1) * 255))
	bi := int(math.Round(clampF(b, 0, 1) * 255))
	return colorpicker.RGBToHex(ri, gi, bi)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// blendColors linearly interpolates between two hex colors.
// t=0 returns a, t=1 returns b.
func blendColors(a, b string, t float64) string {
	ar, ag, ab := colorpicker.HexToRGB(a)
	br, bg, bb := colorpicker.HexToRGB(b)

	lerp := func(x, y int, t float64) int {
		return int(math.Round(float64(x)*(1-t) + float64(y)*t))
	}

	return colorpicker.RGBToHex(
		lerp(ar, br, t),
		lerp(ag, bg, t),
		lerp(ab, bb, t),
	)
}

// ImportIterm converts an iTerm2 .itermcolors file into a thinkt Theme.
func ImportIterm(r io.Reader, name string) (Theme, error) {
	colors, err := ParseItermColors(r)
	if err != nil {
		return Theme{}, err
	}

	get := func(key string) string {
		if c, ok := colors[key]; ok {
			return floatToHex(c[0], c[1], c[2])
		}
		return "#888888"
	}

	bg := get("Background Color")
	fg := get("Foreground Color")

	ansi := func(n int) string {
		return get(fmt.Sprintf("Ansi %d Color", n))
	}

	// ANSI color assignments:
	// 0=black 1=red 2=green 3=yellow 4=blue 5=magenta 6=cyan 7=white
	// 8-15 = bright variants

	accent := ansi(12)       // bright blue
	textSecondary := ansi(8) // bright black (gray)

	t := Theme{
		Name:        name,
		Description: fmt.Sprintf("Imported from %s iTerm2 color scheme", name),

		Accent:         accent,
		BorderActive:   accent,
		BorderInactive: ansi(8),

		TextPrimary:   Style{Fg: fg},
		TextSecondary: Style{Fg: textSecondary},
		TextMuted:     Style{Fg: blendColors(ansi(8), bg, 0.4)},

		UserLabel:      Style{Fg: ansi(4), Bold: true},
		UserBlock:      Style{Fg: fg, Bg: blendColors(bg, ansi(4), 0.15)},
		AssistantLabel: Style{Fg: ansi(2), Bold: true},
		AssistantBlock: Style{Fg: fg, Bg: blendColors(bg, ansi(2), 0.15)},
		ThinkingLabel:  Style{Fg: ansi(5), Bold: true},
		ThinkingBlock:  Style{Fg: blendColors(fg, ansi(5), 0.3), Bg: blendColors(bg, ansi(5), 0.12), Italic: true},
		ToolLabel:      Style{Fg: ansi(3), Bold: true},
		ToolCallBlock:  Style{Fg: blendColors(fg, ansi(3), 0.3), Bg: blendColors(bg, ansi(3), 0.12)},
		ToolResultBlock: Style{Fg: blendColors(fg, ansi(6), 0.3), Bg: blendColors(bg, ansi(6), 0.12)},

		ConfirmPrompt:     Style{Fg: fg},
		ConfirmSelected:   Style{Bg: accent, Fg: colorpicker.ContrastColorHex(accent)},
		ConfirmUnselected: Style{Fg: blendColors(ansi(8), bg, 0.4)},
	}

	return t, nil
}
