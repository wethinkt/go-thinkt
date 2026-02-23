package thinkt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Default timeout for considering a session "active" based on file mtime.
const DefaultActiveWindow = 5 * time.Minute

// ActiveSession represents a session detected as currently active.
type ActiveSession struct {
	Source      Source    `json:"source"`
	ProjectPath string   `json:"project_path"`
	SessionPath string   `json:"session_path"`
	SessionID   string   `json:"session_id"`
	DetectedAt  time.Time `json:"detected_at"`
	Method      string   `json:"method"`       // "ide_lock", "process", "mtime"
	IDE         string   `json:"ide,omitempty"` // e.g. "Visual Studio Code" (ide_lock only)
	PID         int      `json:"pid,omitempty"` // process ID (ide_lock, process)
}

// ActiveSessionDetector detects which sessions are currently active on the
// local machine by combining multiple signals: process inspection, IDE lock
// files, and file mtime.
type ActiveSessionDetector struct {
	registry     *StoreRegistry
	activeWindow time.Duration
	claudeDir    string // override for testing; empty = default ~/.claude
}

// NewActiveSessionDetector creates a detector that uses the given registry.
func NewActiveSessionDetector(registry *StoreRegistry) *ActiveSessionDetector {
	return &ActiveSessionDetector{
		registry:     registry,
		activeWindow: DefaultActiveWindow,
	}
}

// SetActiveWindow overrides the mtime window for considering sessions active.
func (d *ActiveSessionDetector) SetActiveWindow(w time.Duration) {
	d.activeWindow = w
}

// SetClaudeDir overrides the Claude base directory (for testing).
func (d *ActiveSessionDetector) SetClaudeDir(dir string) {
	d.claudeDir = dir
}

// Detect returns all currently active sessions using multiple detection methods.
// Process inspection is tried first, then IDE lock files, then mtime heuristic.
func (d *ActiveSessionDetector) Detect(ctx context.Context) ([]ActiveSession, error) {
	now := time.Now()
	seen := make(map[string]bool) // sessionPath -> true, for dedup

	var result []ActiveSession

	// 1. Process inspection (ps + lsof/proc for cwd) — most reliable for
	//    detecting actually running CLI sessions (terminal + background).
	procSessions, err := d.detectProcess(ctx, now)
	if err == nil {
		for _, s := range procSessions {
			if !seen[s.SessionPath] {
				seen[s.SessionPath] = true
				result = append(result, s)
			}
		}
	}

	// 2. Claude IDE lock files — detects open VS Code workspaces.
	ideSessions, err := d.detectIDELock(ctx, now)
	if err == nil {
		for _, s := range ideSessions {
			if !seen[s.SessionPath] {
				seen[s.SessionPath] = true
				result = append(result, s)
			}
		}
	}

	// 3. File mtime heuristic across all sources
	mtimeSessions, err := d.detectMtime(ctx, now)
	if err == nil {
		for _, s := range mtimeSessions {
			if !seen[s.SessionPath] {
				seen[s.SessionPath] = true
				result = append(result, s)
			}
		}
	}

	// Sort by detected time descending (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].DetectedAt.After(result[j].DetectedAt)
	})

	return result, nil
}

// ideLockEntry represents a single JSON object in a Claude IDE lock file.
type ideLockEntry struct {
	PID              int      `json:"pid"`
	WorkspaceFolders []string `json:"workspaceFolders"`
	IDEName          string   `json:"ideName"`
}

// detectIDELock parses Claude IDE lock files (~/.claude/ide/*.lock),
// checks if the PID is alive, maps workspace folders to projects,
// and finds the most recent session per project.
func (d *ActiveSessionDetector) detectIDELock(_ context.Context, now time.Time) ([]ActiveSession, error) {
	claudeDir := d.claudeDir
	if claudeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		claudeDir = filepath.Join(home, ".claude")
	}

	ideDir := filepath.Join(claudeDir, "ide")
	lockFiles, err := filepath.Glob(filepath.Join(ideDir, "*.lock"))
	if err != nil || len(lockFiles) == 0 {
		return nil, err
	}

	var result []ActiveSession

	for _, lockFile := range lockFiles {
		entries, err := parseIDELockFile(lockFile)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !isProcessAlive(entry.PID) {
				continue
			}

			for _, wsFolder := range entry.WorkspaceFolders {
				// Find the most recent session file in this project
				sessionPath, sessionID := d.findMostRecentSession(claudeDir, wsFolder)
				if sessionPath == "" {
					continue
				}

				result = append(result, ActiveSession{
					Source:      SourceClaude,
					ProjectPath: wsFolder,
					SessionPath: sessionPath,
					SessionID:   sessionID,
					DetectedAt:  now,
					Method:      "ide_lock",
					IDE:         entry.IDEName,
					PID:         entry.PID,
				})
			}
		}
	}

	return result, nil
}

// parseIDELockFile reads a lock file containing concatenated JSON objects.
func parseIDELockFile(path string) ([]ideLockEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var entries []ideLockEntry
	dec := json.NewDecoder(strings.NewReader(string(data)))
	for dec.More() {
		var entry ideLockEntry
		if err := dec.Decode(&entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// findMostRecentSession finds the most recently modified .jsonl session
// file for a given project workspace path within Claude's storage.
func (d *ActiveSessionDetector) findMostRecentSession(claudeDir, projectPath string) (string, string) {
	// Claude stores sessions in: ~/.claude/projects/<encoded-path>/<uuid>.jsonl
	// The encoded path uses - instead of /
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", ""
	}

	// Find the project directory that matches this workspace folder.
	// Claude encodes paths like: -Users-evan-wethinkt-go-thinkt
	normalizedProject := strings.ReplaceAll(projectPath, string(os.PathSeparator), "-")
	normalizedProject = strings.TrimPrefix(normalizedProject, "-")

	var bestPath string
	var bestTime time.Time
	var bestID string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		// Match: the directory name should correspond to the project path
		normalizedDir := strings.TrimPrefix(dirName, "-")
		if normalizedDir != normalizedProject {
			continue
		}

		sessDir := filepath.Join(projectsDir, dirName)
		sessEntries, err := os.ReadDir(sessDir)
		if err != nil {
			continue
		}

		for _, se := range sessEntries {
			if se.IsDir() || !strings.HasSuffix(se.Name(), ".jsonl") {
				continue
			}
			info, err := se.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(bestTime) {
				bestTime = info.ModTime()
				bestPath = filepath.Join(sessDir, se.Name())
				bestID = strings.TrimSuffix(se.Name(), ".jsonl")
			}
		}
	}

	return bestPath, bestID
}

// knownProcesses maps process command names to their Source type.
// These are the AI coding tool CLI processes we look for.
var knownProcesses = map[string]Source{
	"claude": SourceClaude,
	"kimi":   SourceKimi,
	"gemini": SourceGemini,
	"codex":  SourceCodex,
}

// detectMtime finds sessions with recent file modifications across all sources.
func (d *ActiveSessionDetector) detectMtime(ctx context.Context, now time.Time) ([]ActiveSession, error) {
	cutoff := now.Add(-d.activeWindow)
	var result []ActiveSession

	for _, store := range d.registry.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}

		for _, project := range projects {
			sessions, err := store.ListSessions(ctx, project.ID)
			if err != nil {
				continue
			}

			for _, sess := range sessions {
				modTime := sess.ModifiedAt
				if modTime.IsZero() {
					modTime = sess.CreatedAt
				}
				if modTime.After(cutoff) {
					result = append(result, ActiveSession{
						Source:      store.Source(),
						ProjectPath: project.Path,
						SessionPath: sess.FullPath,
						SessionID:   sess.ID,
						DetectedAt:  modTime,
						Method:      "mtime",
					})
				}
			}
		}
	}

	return result, nil
}

// FormatActiveSession returns a human-readable string for an active session.
func FormatActiveSession(s ActiveSession) string {
	parts := []string{fmt.Sprintf("[%s]", s.Source)}
	if s.IDE != "" {
		parts = append(parts, s.IDE)
	}
	parts = append(parts, s.ProjectPath)
	if s.SessionID != "" {
		id := s.SessionID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, id)
	}
	parts = append(parts, fmt.Sprintf("(%s)", s.Method))
	return strings.Join(parts, " ")
}
