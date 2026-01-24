package prompt

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
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
	format   Format
	writer   io.Writer
	template *template.Template
	data     *TemplateData
}

// FormatterOption configures a Formatter.
type FormatterOption func(*Formatter)

// WithTemplate sets a custom template for markdown output.
func WithTemplate(tmpl *template.Template) FormatterOption {
	return func(f *Formatter) {
		f.template = tmpl
	}
}

// WithTemplateData sets additional template data (session metadata).
func WithTemplateData(data *TemplateData) FormatterOption {
	return func(f *Formatter) {
		f.data = data
	}
}

// NewFormatter creates a formatter for the given format.
func NewFormatter(w io.Writer, format Format, opts ...FormatterOption) *Formatter {
	f := &Formatter{
		format: format,
		writer: w,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
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
	// Use custom template if provided, otherwise use default
	tmpl := f.template
	if tmpl == nil {
		var err error
		tmpl, err = DefaultTemplate()
		if err != nil {
			return fmt.Errorf("load default template: %w", err)
		}
	}

	// Build template data
	data := f.data
	if data == nil {
		data = NewTemplateData(prompts)
	} else {
		// Merge prompts into existing data
		data.Prompts = prompts
		data.Count = len(prompts)
	}

	return ExecuteTemplate(f.writer, tmpl, data)
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
