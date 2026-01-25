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

// SessionWindowMsg is sent when a session window finishes loading.
type SessionWindowMsg struct {
	Window      *claude.SessionWindow
	Path        string
	IsContinue  bool // True if this is a continuation (append), false for initial load
	Err         error
}
