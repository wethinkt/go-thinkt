package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
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

// FormatList outputs sessions one per line (full path).
func (f *SessionsFormatter) FormatList(sessions []claude.SessionMeta) error {
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
}

const defaultSessionSummaryTemplate = `{{range .}}{{.Path}}
  ID:       {{.SessionID}}
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
  {{.Summary}}    First prompt summary (if available)
  {{.Messages}}   Number of messages
  {{.Created}}    Creation time (time.Time)
  {{.Modified}}   Last modified time (time.Time)
  {{.Branch}}     Git branch (if available)`

// FormatSummary outputs detailed session information.
func (f *SessionsFormatter) FormatSummary(sessions []claude.SessionMeta, customTmpl string, opts SessionListOptions) error {
	// Sort sessions
	sortSessions(sessions, opts.SortBy, opts.Descending)

	// Build template data
	data := make([]SessionSummaryData, len(sessions))
	for i, s := range sessions {
		data[i] = SessionSummaryData{
			Path:      s.FullPath,
			SessionID: s.SessionID,
			Summary:   s.Summary,
			Messages:  s.MessageCount,
			Created:   s.Created,
			Modified:  s.Modified,
			Branch:    s.GitBranch,
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

func sortSessions(sessions []claude.SessionMeta, sortBy string, descending bool) {
	switch sortBy {
	case "name":
		sort.Slice(sessions, func(i, j int) bool {
			cmp := strings.Compare(
				strings.ToLower(sessions[i].SessionID),
				strings.ToLower(sessions[j].SessionID),
			)
			if descending {
				return cmp > 0
			}
			return cmp < 0
		})
	case "time", "":
		sort.Slice(sessions, func(i, j int) bool {
			if descending {
				return sessions[i].Modified.After(sessions[j].Modified)
			}
			return sessions[i].Modified.Before(sessions[j].Modified)
		})
	}
}

// SessionDeleter handles session deletion.
type SessionDeleter struct {
	baseDir string
	opts    SessionDeleteOptions
}

// SessionDeleteOptions configures session deletion.
type SessionDeleteOptions struct {
	Force   bool
	Stdout  io.Writer
	Project string // Project path to scope the session search
}

// NewSessionDeleter creates a new session deleter.
func NewSessionDeleter(baseDir string, opts SessionDeleteOptions) *SessionDeleter {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &SessionDeleter{baseDir: baseDir, opts: opts}
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
		fmt.Fprintf(d.opts.Stdout, "ID: %s\n", session.SessionID)
		fmt.Fprintf(d.opts.Stdout, "Messages: %d\n", session.MessageCount)
		if !session.Modified.IsZero() {
			fmt.Fprintf(d.opts.Stdout, "Modified: %s\n", session.Modified.Format("2006-01-02 15:04"))
		}
		if session.Summary != "" {
			fmt.Fprintf(d.opts.Stdout, "Summary: %s\n", session.Summary)
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
func (d *SessionDeleter) findSession(sessionPath string) (*claude.SessionMeta, error) {
	// If it's an absolute path to a .jsonl file, use it directly
	if filepath.IsAbs(sessionPath) && strings.HasSuffix(sessionPath, ".jsonl") {
		if _, err := os.Stat(sessionPath); err == nil {
			// Load session metadata
			return d.loadSessionMeta(sessionPath)
		}
	}

	// Otherwise, search within the project
	if d.opts.Project == "" {
		return nil, fmt.Errorf("session not found: %s\n\nSpecify -p/--project to search within a project, or provide the full session path", sessionPath)
	}

	sessions, err := d.listProjectSessions()
	if err != nil {
		return nil, err
	}

	// Try to match by session ID or path suffix
	for _, s := range sessions {
		if s.SessionID == sessionPath || strings.HasSuffix(s.FullPath, sessionPath) {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionPath, d.opts.Project)
}

func (d *SessionDeleter) listProjectSessions() ([]claude.SessionMeta, error) {
	// Find project directory
	absPath, err := filepath.Abs(d.opts.Project)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	projects, err := claude.ListProjects(d.baseDir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for _, p := range projects {
		if p.FullPath == absPath {
			return claude.ListProjectSessions(p.DirPath)
		}
	}

	return nil, fmt.Errorf("project not found: %s", d.opts.Project)
}

func (d *SessionDeleter) loadSessionMeta(path string) (*claude.SessionMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat session: %w", err)
	}

	return &claude.SessionMeta{
		SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		FullPath:  path,
		Modified:  info.ModTime(),
	}, nil
}

// SessionCopier handles session copying.
type SessionCopier struct {
	baseDir string
	opts    SessionCopyOptions
}

// SessionCopyOptions configures session copying.
type SessionCopyOptions struct {
	Stdout  io.Writer
	Project string // Project path to scope the session search
}

// NewSessionCopier creates a new session copier.
func NewSessionCopier(baseDir string, opts SessionCopyOptions) *SessionCopier {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &SessionCopier{baseDir: baseDir, opts: opts}
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
	} else if !strings.HasSuffix(targetPath, ".jsonl") {
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
func (c *SessionCopier) findSession(sessionPath string) (*claude.SessionMeta, error) {
	// If it's an absolute path to a .jsonl file, use it directly
	if filepath.IsAbs(sessionPath) && strings.HasSuffix(sessionPath, ".jsonl") {
		if info, err := os.Stat(sessionPath); err == nil {
			return &claude.SessionMeta{
				SessionID: strings.TrimSuffix(filepath.Base(sessionPath), ".jsonl"),
				FullPath:  sessionPath,
				Modified:  info.ModTime(),
			}, nil
		}
	}

	// Otherwise, search within the project
	if c.opts.Project == "" {
		return nil, fmt.Errorf("session not found: %s\n\nSpecify -p/--project to search within a project, or provide the full session path", sessionPath)
	}

	sessions, err := c.listProjectSessions()
	if err != nil {
		return nil, err
	}

	// Try to match by session ID or path suffix
	for _, s := range sessions {
		if s.SessionID == sessionPath || strings.HasSuffix(s.FullPath, sessionPath) {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionPath, c.opts.Project)
}

func (c *SessionCopier) listProjectSessions() ([]claude.SessionMeta, error) {
	absPath, err := filepath.Abs(c.opts.Project)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	projects, err := claude.ListProjects(c.baseDir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for _, p := range projects {
		if p.FullPath == absPath {
			return claude.ListProjectSessions(p.DirPath)
		}
	}

	return nil, fmt.Errorf("project not found: %s", c.opts.Project)
}

// ListSessionsForProject lists sessions for a given project path.
func ListSessionsForProject(baseDir, projectPath string) ([]claude.SessionMeta, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	projects, err := claude.ListProjects(baseDir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for _, p := range projects {
		if p.FullPath == absPath {
			return claude.ListProjectSessions(p.DirPath)
		}
	}

	return nil, fmt.Errorf("project not found: %s\n\nUse 'thinkt projects' to list available projects", projectPath)
}
