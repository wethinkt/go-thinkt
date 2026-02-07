// Package cli provides CLI output formatting utilities.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// DefaultSummaryTemplate is the default template for project summaries.
const DefaultSummaryTemplate = `{{range .}}{{.Path}}
  Source: {{.Source}}
  Sessions: {{.SessionCount}}
{{- if .Modified}}
  Modified: {{.Modified}}
{{- end}}
{{- if .Sessions}}
  Session List:
  {{- range .Sessions}}
    - {{.Name}} ({{.EntryCount}} entries)
  {{- end}}
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
  .DirPath       string  - Path to project directory
  .Source        string  - Source type (kimi, claude)
  .Sessions      []SessionSummary - Session details (with --with-sessions flag)

Each SessionSummary has:
  .ID            string  - Session ID
  .Name          string  - First prompt or session ID
  .EntryCount    int     - Number of entries/messages
  .Modified      string  - Last modified time
  .GitBranch     string  - Git branch (if any)

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
	Source       string
	Sessions     []SessionSummary // Only populated when with-sessions flag is used
}

// SessionSummary holds template-friendly session data.
type SessionSummary struct {
	ID         string
	Name       string // First prompt or ID
	EntryCount int
	Modified   string
	GitBranch  string
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
func (f *ProjectsFormatter) FormatShort(projects []thinkt.Project) error {
	for _, p := range projects {
		path := p.Path
		if path == "" {
			path = "~"
		}
		fmt.Fprintln(f.w, path)
	}
	return nil
}

// FormatVerbose writes project paths with source and metadata in aligned columns.
func (f *ProjectsFormatter) FormatVerbose(projects []thinkt.Project) error {
	w := tabwriter.NewWriter(f.w, 0, 0, 2, ' ', 0)

	for _, p := range projects {
		path := p.Path
		if path == "" {
			path = "~"
		}

		// Format source with color-friendly indicators
		source := string(p.Source)

		// Format session count
		sessions := fmt.Sprintf("%d sessions", p.SessionCount)

		// Format last modified time
		var modified string
		if !p.LastModified.IsZero() {
			modified = p.LastModified.Format("2006-01-02 15:04")
		} else {
			modified = "-"
		}

		fmt.Fprintf(w, "%s\t[%s]\t%s\t%s\n", path, source, sessions, modified)
	}

	return w.Flush()
}

// FormatTree writes projects in a tree view grouped by source, then parent directory.
func (f *ProjectsFormatter) FormatTree(projects []thinkt.Project) error {
	// Group by source first
	bySource := groupBySource(projects)
	sources := sortedSourceKeys(bySource)

	for _, source := range sources {
		sourceProjs := bySource[source]

		// Source root line with base path (no tree characters, just the label)
		sourceLabel := fmt.Sprintf("%s (%s)", sourceDisplayName(source), sourceBasePath(sourceProjs))
		fmt.Fprintf(f.w, "%s\n", sourceLabel)

		// Group projects under this source by parent directory
		groups := groupByParent(sourceProjs)
		parents := sortedKeys(groups)

		for i, parent := range parents {
			projs := groups[parent]
			isLastParent := i == len(parents)-1

			// Parent directory branch (direct child of source/root)
			// Parents are at tree level 1, so they just use tree chars without prefix
			parentBranchChar := "├── "
			if isLastParent {
				parentBranchChar = "└── "
			}
			fmt.Fprintf(f.w, "%s%s/\n", parentBranchChar, parent)

			// Print each project under this parent
			// Projects are at tree level 2 - they show continuation from parent only
			// (source continuation is implicit since we're inside the source block)
			projPrefix := "│   "
			if isLastParent {
				projPrefix = "    "
			}

			for j, p := range projs {
				itemChar := "├── "
				if j == len(projs)-1 {
					itemChar = "└── "
				}
				fmt.Fprintf(f.w, "%s%s%s (%d)\n", projPrefix, itemChar, p.Name, p.SessionCount)
			}
		}
	}

	return nil
}

// FormatSummary writes detailed project information using a template.
// If tmplStr is empty, uses DefaultSummaryTemplate.
// If projectSessions is provided, session details are included in the summary.
func (f *ProjectsFormatter) FormatSummary(projects []thinkt.Project, projectSessions map[string][]thinkt.SessionMeta, tmplStr string, opts SummaryOptions) error {
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
		path := p.Path
		if path == "" {
			path = "~"
		}
		var modified string
		if !p.LastModified.IsZero() {
			modified = p.LastModified.Format("2006-01-02 15:04")
		}
		summary := ProjectSummary{
			Path:         path,
			DisplayName:  p.Name,
			SessionCount: p.SessionCount,
			Modified:     modified,
			DirPath:      p.ID,
			Source:       string(p.Source),
		}

		// Add session details if provided
		if sessions, ok := projectSessions[p.ID]; ok {
			summary.Sessions = make([]SessionSummary, len(sessions))
			for j, s := range sessions {
				name := s.FirstPrompt
				if name == "" {
					name = s.ID
				}
				// Truncate long first prompts
				if len(name) > 50 {
					name = name[:47] + "..."
				}
				var sessModified string
				if !s.ModifiedAt.IsZero() {
					sessModified = s.ModifiedAt.Format("2006-01-02 15:04")
				}
				summary.Sessions[j] = SessionSummary{
					ID:         s.ID,
					Name:       name,
					EntryCount: s.EntryCount,
					Modified:   sessModified,
					GitBranch:  s.GitBranch,
				}
			}
		}

		summaries[i] = summary
	}

	return tmpl.Execute(f.w, summaries)
}

// sortProjects sorts projects based on options.
func sortProjects(projects []thinkt.Project, opts SummaryOptions) {
	switch opts.SortBy {
	case "name":
		sort.Slice(projects, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(projects[i].Name), strings.ToLower(projects[j].Name))
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
func groupByParent(projects []thinkt.Project) map[string][]thinkt.Project {
	groups := make(map[string][]thinkt.Project)

	for _, p := range projects {
		path := p.Path
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
func sortedKeys(m map[string][]thinkt.Project) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// groupBySource groups projects by their source.
func groupBySource(projects []thinkt.Project) map[thinkt.Source][]thinkt.Project {
	groups := make(map[thinkt.Source][]thinkt.Project)
	for _, p := range projects {
		groups[p.Source] = append(groups[p.Source], p)
	}
	return groups
}

// sortedSourceKeys returns source keys sorted alphabetically.
func sortedSourceKeys(m map[thinkt.Source][]thinkt.Project) []thinkt.Source {
	keys := make([]thinkt.Source, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i]) < string(keys[j])
	})
	return keys
}

// sourceDisplayName returns a human-readable name for a source.
func sourceDisplayName(s thinkt.Source) string {
	switch s {
	case thinkt.SourceKimi:
		return "kimi"
	case thinkt.SourceClaude:
		return "claude"
	default:
		return string(s)
	}
}

// sourceBasePath extracts the base path from the first project in a source group.
func sourceBasePath(projects []thinkt.Project) string {
	if len(projects) == 0 {
		return ""
	}

	path := projects[0].SourceBasePath
	if path == "" {
		return ""
	}

	// Try to replace home dir with ~ for better display
	home, err := os.UserHomeDir()
	if err == nil {
		if strings.HasPrefix(path, home) {
			path = "~" + strings.TrimPrefix(path, home)
		}
	}

	return path
}
