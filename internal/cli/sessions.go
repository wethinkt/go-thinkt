// Package cli provides CLI output formatting utilities.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// SessionsFormatter handles session listing output.
type SessionsFormatter struct {
	w io.Writer
}

// NewSessionsFormatter creates a new sessions formatter.
func NewSessionsFormatter(w io.Writer) *SessionsFormatter {
	return &SessionsFormatter{w: w}
}

// SessionListOptions configures session list output.
type SessionListOptions struct {
	SortBy     string // "time" or "name"
	Descending bool
	Template   string // Custom Go template
}

// ResolveSession resolves a user-provided query (ID, suffix, or absolute path)
// into a known session from registered sources.
func ResolveSession(registry *thinkt.StoreRegistry, projectID, query string) (*thinkt.SessionMeta, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("session query is required")
	}

	ctx := context.Background()

	// Absolute paths must resolve to a known session in the registry.
	if filepath.IsAbs(query) {
		_, meta, err := registry.ResolveSessionByPath(ctx, query)
		if err == nil && meta != nil {
			return meta, nil
		}
		if err != nil && err != os.ErrNotExist {
			return nil, fmt.Errorf("resolve session path: %w", err)
		}
		return nil, fmt.Errorf("session not found in known sources: %s", query)
	}

	candidates, err := collectCandidateSessions(registry, projectID)
	if err != nil {
		return nil, err
	}

	matches := make([]thinkt.SessionMeta, 0, 4)
	for _, s := range candidates {
		if sessionMatchesQuery(s, query) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("session not found: %s", query)
	case 1:
		return &matches[0], nil
	default:
		var b strings.Builder
		b.WriteString("session query is ambiguous, matched multiple sessions:\n")
		max := len(matches)
		if max > 5 {
			max = 5
		}
		for i := 0; i < max; i++ {
			b.WriteString("  - ")
			b.WriteString(matches[i].FullPath)
			b.WriteByte('\n')
		}
		if len(matches) > max {
			b.WriteString(fmt.Sprintf("  ... and %d more", len(matches)-max))
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(b.String()))
	}
}

func collectCandidateSessions(registry *thinkt.StoreRegistry, projectID string) ([]thinkt.SessionMeta, error) {
	if projectID != "" {
		return ListSessionsForProject(registry, projectID)
	}

	ctx := context.Background()
	var all []thinkt.SessionMeta
	for _, store := range registry.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, p := range projects {
			sessions, err := store.ListSessions(ctx, p.ID)
			if err != nil {
				continue
			}
			all = append(all, sessions...)
		}
	}
	return all, nil
}

func sessionMatchesQuery(meta thinkt.SessionMeta, query string) bool {
	if meta.ID == query {
		return true
	}
	if filepath.Base(meta.FullPath) == query {
		return true
	}
	if strings.HasSuffix(meta.FullPath, query) {
		return true
	}
	if strings.HasSuffix(meta.FullPath, query+".jsonl") {
		return true
	}
	if strings.HasSuffix(meta.FullPath, query+".json") {
		return true
	}
	return false
}

// FormatList outputs sessions one per line (full path).
func (f *SessionsFormatter) FormatList(sessions []thinkt.SessionMeta) error {
	for _, s := range sessions {
		fmt.Fprintln(f.w, s.FullPath)
	}
	return nil
}

// SessionSummaryData is the template data for session summary.
type SessionSummaryData struct {
	Path      string
	SessionID string
	Summary   string
	Messages  int
	Created   time.Time
	Modified  time.Time
	Branch    string
	Source    string
}

const defaultSessionSummaryTemplate = `{{range .}}{{.Path}}
  ID:       {{.SessionID}}
  Source:   {{.Source}}
  Messages: {{.Messages}}
  Created:  {{.Created.Format "2006-01-02 15:04"}}
  Modified: {{.Modified.Format "2006-01-02 15:04"}}{{if .Branch}}
  Branch:   {{.Branch}}{{end}}{{if .Summary}}
  Summary:  {{.Summary}}{{end}}

{{end}}`

// SessionSummaryTemplateHelp documents the template variables.
const SessionSummaryTemplateHelp = `Template variables:
  {{.Path}}       Full path to session file
  {{.SessionID}}  Session identifier
  {{.Source}}     Source type (kimi, claude)
  {{.Summary}}    First prompt summary (if available)
  {{.Messages}}   Number of messages
  {{.Created}}    Creation time (time.Time)
  {{.Modified}}   Last modified time (time.Time)
  {{.Branch}}     Git branch (if available)`

// FormatSummary outputs detailed session information.
func (f *SessionsFormatter) FormatSummary(sessions []thinkt.SessionMeta, customTmpl string, opts SessionListOptions) error {
	// Sort sessions
	sortSessions(sessions, opts.SortBy, opts.Descending)

	// Build template data
	data := make([]SessionSummaryData, len(sessions))
	for i, s := range sessions {
		data[i] = SessionSummaryData{
			Path:      s.FullPath,
			SessionID: s.ID,
			Summary:   s.FirstPrompt,
			Messages:  s.EntryCount,
			Created:   s.CreatedAt,
			Modified:  s.ModifiedAt,
			Branch:    s.GitBranch,
			Source:    string(s.Source),
		}
	}

	// Parse template
	tmplStr := defaultSessionSummaryTemplate
	if customTmpl != "" {
		tmplStr = customTmpl
	}

	tmpl, err := template.New("sessions").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	return tmpl.Execute(f.w, data)
}

func sortSessions(sessions []thinkt.SessionMeta, sortBy string, descending bool) {
	switch sortBy {
	case "name":
		sort.Slice(sessions, func(i, j int) bool {
			cmp := strings.Compare(
				strings.ToLower(sessions[i].ID),
				strings.ToLower(sessions[j].ID),
			)
			if descending {
				return cmp > 0
			}
			return cmp < 0
		})
	case "time", "":
		sort.Slice(sessions, func(i, j int) bool {
			if descending {
				return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
			}
			return sessions[i].ModifiedAt.Before(sessions[j].ModifiedAt)
		})
	}
}

// ListSessionsForProject lists sessions for a given project using the registry.
func ListSessionsForProject(registry *thinkt.StoreRegistry, projectID string) ([]thinkt.SessionMeta, error) {
	ctx := context.Background()

	// Find the project across all sources
	projects, err := registry.ListAllProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	var targetProject *thinkt.Project
	for _, p := range projects {
		if p.ID == projectID || p.Path == projectID {
			targetProject = &p
			break
		}
	}

	if targetProject == nil {
		return nil, fmt.Errorf("project not found: %s\n\nUse 'thinkt projects' to list available projects", projectID)
	}

	// Get the appropriate store
	store, ok := registry.Get(targetProject.Source)
	if !ok {
		return nil, fmt.Errorf("source not available: %s", targetProject.Source)
	}

	return store.ListSessions(ctx, targetProject.ID)
}

// SessionDeleter handles session deletion.
type SessionDeleter struct {
	registry *thinkt.StoreRegistry
	opts     SessionDeleteOptions
}

// SessionDeleteOptions configures session deletion.
type SessionDeleteOptions struct {
	Force   bool
	Stdout  io.Writer
	Project string // Project path to scope the session search
}

// NewSessionDeleter creates a new session deleter.
func NewSessionDeleter(registry *thinkt.StoreRegistry, opts SessionDeleteOptions) *SessionDeleter {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &SessionDeleter{registry: registry, opts: opts}
}

// Delete removes a session file after confirmation.
func (d *SessionDeleter) Delete(sessionPath string) error {
	// Find the session
	session, err := d.findSession(sessionPath)
	if err != nil {
		return err
	}

	// Show info and confirm
	if !d.opts.Force {
		fmt.Fprintf(d.opts.Stdout, "Session: %s\n", session.FullPath)
		fmt.Fprintf(d.opts.Stdout, "ID: %s\n", session.ID)
		fmt.Fprintf(d.opts.Stdout, "Source: %s\n", session.Source)
		fmt.Fprintf(d.opts.Stdout, "Messages: %d\n", session.EntryCount)
		if !session.ModifiedAt.IsZero() {
			fmt.Fprintf(d.opts.Stdout, "Modified: %s\n", session.ModifiedAt.Format("2006-01-02 15:04"))
		}
		if session.FirstPrompt != "" {
			summary := session.FirstPrompt
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			fmt.Fprintf(d.opts.Stdout, "Summary: %s\n", summary)
		}
		fmt.Fprintln(d.opts.Stdout)

		result, err := tui.Confirm(tui.ConfirmOptions{
			Prompt:      "Permanently delete this session?",
			Affirmative: "Delete",
			Negative:    "Cancel",
			Default:     false,
		})

		if err != nil || result != tui.ConfirmYes {
			fmt.Fprintf(d.opts.Stdout, "Cancelled.\n")
			return nil
		}
	}

	// Delete the session file
	if err := os.Remove(session.FullPath); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	fmt.Fprintf(d.opts.Stdout, "Deleted %s\n", session.FullPath)
	return nil
}

// findSession finds a session by path or ID within the project scope.
func (d *SessionDeleter) findSession(sessionPath string) (*thinkt.SessionMeta, error) {
	meta, err := ResolveSession(d.registry, d.opts.Project, sessionPath)
	if err != nil {
		if d.opts.Project != "" {
			return nil, fmt.Errorf("%w\n\nUse 'thinkt sessions list -p %s' to see available sessions", err, d.opts.Project)
		}
		return nil, fmt.Errorf("%w\n\nUse 'thinkt sessions list' or 'thinkt sessions resolve <query>' to find valid sessions", err)
	}
	return meta, nil
}

// SessionCopier handles session copying.
type SessionCopier struct {
	registry *thinkt.StoreRegistry
	opts     SessionCopyOptions
}

// SessionCopyOptions configures session copying.
type SessionCopyOptions struct {
	Stdout  io.Writer
	Project string // Project path to scope the session search
}

// NewSessionCopier creates a new session copier.
func NewSessionCopier(registry *thinkt.StoreRegistry, opts SessionCopyOptions) *SessionCopier {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &SessionCopier{registry: registry, opts: opts}
}

// Copy copies a session file to the target path.
func (c *SessionCopier) Copy(sessionPath, targetPath string) error {
	// Find the session
	session, err := c.findSession(sessionPath)
	if err != nil {
		return err
	}

	// Determine target file path
	targetFile := targetPath
	if info, err := os.Stat(targetPath); err == nil && info.IsDir() {
		// Target is a directory, use original filename
		targetFile = filepath.Join(targetPath, filepath.Base(session.FullPath))
	} else if filepath.Ext(targetPath) == "" {
		// Create target directory and use original filename
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return fmt.Errorf("create target directory: %w", err)
		}
		targetFile = filepath.Join(targetPath, filepath.Base(session.FullPath))
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	// Copy the file
	if err := copyFile(session.FullPath, targetFile); err != nil {
		return fmt.Errorf("copy session: %w", err)
	}

	fmt.Fprintf(c.opts.Stdout, "Copied %s to %s\n", session.FullPath, targetFile)
	return nil
}

// findSession finds a session by path or ID within the project scope.
func (c *SessionCopier) findSession(sessionPath string) (*thinkt.SessionMeta, error) {
	meta, err := ResolveSession(c.registry, c.opts.Project, sessionPath)
	if err != nil {
		if c.opts.Project != "" {
			return nil, fmt.Errorf("%w\n\nUse 'thinkt sessions list -p %s' to see available sessions", err, c.opts.Project)
		}
		return nil, fmt.Errorf("%w\n\nUse 'thinkt sessions list' or 'thinkt sessions resolve <query>' to find valid sessions", err)
	}
	return meta, nil
}
