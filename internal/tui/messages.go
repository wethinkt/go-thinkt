package tui

import (
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// ProjectsLoadedMsg is sent when the project list finishes loading.
type ProjectsLoadedMsg struct {
	Projects []thinkt.Project
	Err      error
}

// SessionsLoadedMsg is sent when sessions for a project finish loading.
type SessionsLoadedMsg struct {
	Sessions []thinkt.SessionMeta
	Err      error
}

// SessionWindowMsg is sent when a session window finishes loading.
type SessionWindowMsg struct {
	Window      *claude.SessionWindow
	Path        string
	IsContinue  bool // True if this is a continuation (append), false for initial load
	Err         error
}

// LazySessionMsg is sent when a lazy session is opened.
type LazySessionMsg struct {
	Session *claude.LazySession
	Err     error
}

// LazyLoadedMsg is sent when more content is loaded from a lazy session.
type LazyLoadedMsg struct {
	Count int   // Number of new entries loaded
	Err   error
}

// ContentRenderedMsg is sent when content has been rendered asynchronously.
type ContentRenderedMsg struct {
	Rendered      string
	RenderedCount int
}
