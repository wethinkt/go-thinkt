package tui

import "github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"

// ProjectsLoadedMsg is sent when the project list finishes loading.
type ProjectsLoadedMsg struct {
	Projects []claude.Project
	Err      error
}

// SessionsLoadedMsg is sent when sessions for a project finish loading.
type SessionsLoadedMsg struct {
	Sessions []claude.SessionMeta
	Err      error
}

// SessionLoadedMsg is sent when a session finishes loading.
type SessionLoadedMsg struct {
	Session   *claude.Session
	IsPreview bool  // True if only a preview was loaded
	FileSize  int64 // File size in bytes
	Err       error
}
