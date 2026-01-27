// Package cli provides CLI output formatting utilities.
package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// DefaultSummaryTemplate is the default template for project summaries.
const DefaultSummaryTemplate = `{{range .}}{{.Path}}
  Sessions: {{.SessionCount}}
{{- if .Modified}}
  Modified: {{.Modified}}
{{- end}}
{{end}}`

// SummaryTemplateHelp documents the template variables available.
const SummaryTemplateHelp = `Template Variables
==================

Each project in the list has:
  .Path          string  - Full project path (or "~" for home)
  .DisplayName   string  - Short name (last path component)
  .SessionCount  int     - Number of sessions
  .Modified      string  - Last modified time (may be empty)
  .DirPath       string  - Path to Claude project directory

Example custom template:
  {{range .}}{{.DisplayName}}: {{.SessionCount}} sessions
  {{end}}`

// SummaryOptions configures summary output formatting.
type SummaryOptions struct {
	SortBy     string // "name" or "time"
	Descending bool   // sort order
}

// ProjectSummary holds template-friendly project data.
type ProjectSummary struct {
	Path         string
	DisplayName  string
	SessionCount int
	Modified     string
	DirPath      string
}

// ProjectsFormatter formats project listings for CLI output.
type ProjectsFormatter struct {
	w io.Writer
}

// NewProjectsFormatter creates a new projects formatter.
func NewProjectsFormatter(w io.Writer) *ProjectsFormatter {
	return &ProjectsFormatter{w: w}
}

// FormatLong writes project paths, one per line.
func (f *ProjectsFormatter) FormatLong(projects []claude.Project) error {
	for _, p := range projects {
		path := p.FullPath
		if path == "" {
			path = "~"
		}
		fmt.Fprintln(f.w, path)
	}
	return nil
}

// FormatTree writes projects in a tree view grouped by parent directory.
func (f *ProjectsFormatter) FormatTree(projects []claude.Project) error {
	groups := groupByParent(projects)
	parents := sortedKeys(groups)

	for i, parent := range parents {
		projs := groups[parent]

		// Tree characters
		isLast := i == len(parents)-1
		branchChar := "├── "
		if isLast {
			branchChar = "└── "
		}

		// Continuation character for child items
		childPrefix := "│   "
		if isLast {
			childPrefix = "    "
		}

		fmt.Fprintf(f.w, "%s%s/\n", branchChar, parent)

		// Print each project under this parent
		for j, p := range projs {
			itemChar := "├── "
			if j == len(projs)-1 {
				itemChar = "└── "
			}
			fmt.Fprintf(f.w, "%s%s%s (%d)\n", childPrefix, itemChar, p.DisplayName, p.SessionCount)
		}
	}

	return nil
}

// FormatSummary writes detailed project information using a template.
// If tmplStr is empty, uses DefaultSummaryTemplate.
func (f *ProjectsFormatter) FormatSummary(projects []claude.Project, tmplStr string, opts SummaryOptions) error {
	if tmplStr == "" {
		tmplStr = DefaultSummaryTemplate
	}

	tmpl, err := template.New("summary").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Sort projects
	sortProjects(projects, opts)

	summaries := make([]ProjectSummary, len(projects))
	for i, p := range projects {
		path := p.FullPath
		if path == "" {
			path = "~"
		}
		var modified string
		if !p.LastModified.IsZero() {
			modified = p.LastModified.Format("2006-01-02 15:04")
		}
		summaries[i] = ProjectSummary{
			Path:         path,
			DisplayName:  p.DisplayName,
			SessionCount: p.SessionCount,
			Modified:     modified,
			DirPath:      p.DirPath,
		}
	}

	return tmpl.Execute(f.w, summaries)
}

// sortProjects sorts projects based on options.
func sortProjects(projects []claude.Project, opts SummaryOptions) {
	switch opts.SortBy {
	case "name":
		sort.Slice(projects, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(projects[i].DisplayName), strings.ToLower(projects[j].DisplayName))
			if opts.Descending {
				return cmp > 0
			}
			return cmp < 0
		})
	case "time", "":
		sort.Slice(projects, func(i, j int) bool {
			if opts.Descending {
				return projects[i].LastModified.After(projects[j].LastModified)
			}
			return projects[i].LastModified.Before(projects[j].LastModified)
		})
	}
}

// groupByParent groups projects by their parent directory.
func groupByParent(projects []claude.Project) map[string][]claude.Project {
	groups := make(map[string][]claude.Project)

	for _, p := range projects {
		path := p.FullPath
		if path == "" {
			path = "~"
		}
		parent := filepath.Dir(path)
		if parent == "." {
			parent = "~"
		}
		groups[parent] = append(groups[parent], p)
	}

	return groups
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string][]claude.Project) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
