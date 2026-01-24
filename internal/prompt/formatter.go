package prompt

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Format specifies the output format for prompts.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
	FormatPlain    Format = "plain"
)

// ParseFormat parses a format string.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "markdown", "md":
		return FormatMarkdown, nil
	case "json":
		return FormatJSON, nil
	case "plain", "text", "txt":
		return FormatPlain, nil
	default:
		return "", fmt.Errorf("unknown format: %s (valid: markdown, json, plain)", s)
	}
}

// Formatter writes prompts in various formats.
type Formatter struct {
	format Format
	writer io.Writer
}

// NewFormatter creates a formatter for the given format.
func NewFormatter(w io.Writer, format Format) *Formatter {
	return &Formatter{
		format: format,
		writer: w,
	}
}

// Write writes prompts to the output.
func (f *Formatter) Write(prompts []Prompt) error {
	switch f.format {
	case FormatMarkdown:
		return f.writeMarkdown(prompts)
	case FormatJSON:
		return f.writeJSON(prompts)
	case FormatPlain:
		return f.writePlain(prompts)
	default:
		return fmt.Errorf("unsupported format: %s", f.format)
	}
}

func (f *Formatter) writeMarkdown(prompts []Prompt) error {
	for _, p := range prompts {
		// Match the hook format exactly
		timestamp := p.Timestamp
		if timestamp == "" {
			timestamp = "unknown"
		}
		_, err := fmt.Fprintf(f.writer, "\n---\n\n## %s\n\n%s\n", timestamp, p.Text)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *Formatter) writeJSON(prompts []Prompt) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(prompts)
}

func (f *Formatter) writePlain(prompts []Prompt) error {
	for _, p := range prompts {
		_, err := fmt.Fprintf(f.writer, "%s\n\n", p.Text)
		if err != nil {
			return err
		}
	}
	return nil
}
