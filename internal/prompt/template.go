package prompt

import (
	"embed"
	"fmt"
	"io"
	"text/template"
	"time"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// DefaultTemplateHelp documents available template variables and functions.
const DefaultTemplateHelp = `Template Variables
==================

Top-level:
  .Prompts      []Prompt   - List of all extracted prompts
  .Count        int        - Total number of prompts
  .SessionID    string     - Session identifier (if available)
  .Model        string     - Model used (if available)
  .Branch       string     - Git branch (if available)
  .Version      string     - Agent version (if available)
  .CWD          string     - Working directory (if available)
  .StartTime    string     - Session start time
  .EndTime      string     - Session end time
  .Duration     string     - Session duration

Each Prompt in .Prompts:
  .Text         string     - The prompt text content
  .Timestamp    string     - ISO 8601 timestamp
  .UUID         string     - Unique identifier

Template Functions:
  formatTime .Timestamp "2006-01-02"  - Format timestamp
  truncate .Text 100                  - Truncate string
  lineCount .Text                     - Count lines
  wordCount .Text                     - Count words

Example custom template:
  {{range .Prompts}}
  ## {{formatTime .Timestamp "Jan 02, 2006"}}
  {{.Text}}
  {{end}}`

// TemplateData contains all variables available to templates.
type TemplateData struct {
	// Prompts is the list of extracted prompts
	Prompts []Prompt

	// Count is the total number of prompts
	Count int

	// Session metadata (optional, may be empty)
	SessionID string
	Model     string
	Branch    string
	Version   string
	CWD       string
	StartTime string
	EndTime   string
	Duration  string
}

// NewTemplateData creates template data from prompts.
func NewTemplateData(prompts []Prompt) *TemplateData {
	return &TemplateData{
		Prompts: prompts,
		Count:   len(prompts),
	}
}

// templateFuncs provides helper functions available in templates.
var templateFuncs = template.FuncMap{
	// formatTime formats a timestamp string using Go time format
	// Example: {{formatTime .Timestamp "2006-01-02"}}
	"formatTime": func(timestamp, layout string) string {
		t, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return timestamp // Return original if parse fails
		}
		return t.Format(layout)
	},

	// truncate limits string length
	// Example: {{truncate .Text 100}}
	"truncate": func(s string, maxLen int) string {
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen] + "..."
	},

	// lineCount returns number of lines in text
	// Example: {{lineCount .Text}}
	"lineCount": func(s string) int {
		if s == "" {
			return 0
		}
		count := 1
		for _, c := range s {
			if c == '\n' {
				count++
			}
		}
		return count
	},

	// wordCount returns approximate word count
	// Example: {{wordCount .Text}}
	"wordCount": func(s string) int {
		if s == "" {
			return 0
		}
		count := 0
		inWord := false
		for _, c := range s {
			if c == ' ' || c == '\n' || c == '\t' {
				inWord = false
			} else if !inWord {
				inWord = true
				count++
			}
		}
		return count
	},
}

// DefaultTemplate returns the embedded default markdown template.
func DefaultTemplate() (*template.Template, error) {
	return LoadEmbeddedTemplate("default.md.tmpl")
}

// LoadEmbeddedTemplate loads a template from the embedded filesystem.
func LoadEmbeddedTemplate(name string) (*template.Template, error) {
	content, err := templatesFS.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}

	return tmpl, nil
}

// LoadTemplateFile loads a template from an external file path.
func LoadTemplateFile(path string) (*template.Template, error) {
	tmpl, err := template.New("custom").Funcs(templateFuncs).ParseFiles(path)
	if err != nil {
		return nil, fmt.Errorf("parse template file %s: %w", path, err)
	}
	return tmpl, nil
}

// ExecuteTemplate executes a template with the given data.
func ExecuteTemplate(w io.Writer, tmpl *template.Template, data *TemplateData) error {
	return tmpl.Execute(w, data)
}

// ListEmbeddedTemplates returns names of all embedded templates.
func ListEmbeddedTemplates() ([]string, error) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
