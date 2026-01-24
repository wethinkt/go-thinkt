// Package claude provides types and utilities for Claude Code sessions.
package claude

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session represents a Claude Code conversation session.
type Session struct {
	ID        string     // Session UUID
	Path      string     // Path to the JSONL trace file
	Branch    string     // Git branch (if available)
	Model     string     // Model used (e.g., "claude-opus-4-5-20251101")
	Version   string     // Claude Code version
	CWD       string     // Working directory
	StartTime time.Time  // First entry timestamp
	EndTime   time.Time  // Last entry timestamp
	Entries   []Entry    // All entries in the session
}

// Duration returns the session duration.
func (s *Session) Duration() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

// UserPrompts returns all user prompts (excluding tool results).
func (s *Session) UserPrompts() []Prompt {
	var prompts []Prompt
	for _, e := range s.Entries {
		if e.Type == EntryTypeUser {
			text := e.GetPromptText()
			if text != "" {
				prompts = append(prompts, Prompt{
					Text:      text,
					Timestamp: e.Timestamp,
					UUID:      e.UUID,
				})
			}
		}
	}
	return prompts
}

// TurnCount returns the number of user turns in the session.
func (s *Session) TurnCount() int {
	return len(s.UserPrompts())
}

// Prompt represents an extracted user prompt.
type Prompt struct {
	Text      string
	Timestamp string
	UUID      string
}

// ProjectsDir returns the default Claude Code projects directory.
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// FindSessions finds all Claude Code session trace files.
func FindSessions() ([]string, error) {
	projectsDir, err := ProjectsDir()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var traces []string
	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			traces = append(traces, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by modification time, newest first
	sort.Slice(traces, func(i, j int) bool {
		iInfo, _ := os.Stat(traces[i])
		jInfo, _ := os.Stat(traces[j])
		if iInfo == nil || jInfo == nil {
			return false
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	return traces, nil
}

// FindLatestSession finds the most recently modified session trace.
func FindLatestSession() (string, error) {
	sessions, err := FindSessions()
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", nil
	}
	return sessions[0], nil
}
