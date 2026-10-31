package gemini

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume a Gemini CLI session:
// `gemini --resume <sessionId>` run from the project directory, where the
// id is the one the session file records. A subagent resumes its parent.
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("gemini")
	if err != nil {
		return nil, fmt.Errorf("gemini CLI not found: %w", err)
	}
	id := session.ID
	if session.ParentSessionID != "" {
		id = session.ParentSessionID
	}
	headerID := s.headerID(id)
	if headerID == "" {
		return nil, fmt.Errorf("gemini session %s: no session id recorded", id)
	}
	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"gemini", "--resume", headerID},
		Dir:     session.ProjectPath,
	}, nil
}

// headerID returns the sessionId recorded in a session's file, loading the
// session's metadata when this store has not listed it yet.
func (s *Store) headerID(id string) string {
	s.headerMu.Lock()
	h, ok := s.headerIDs[id]
	s.headerMu.Unlock()
	if ok {
		return h
	}
	_, _ = s.GetSessionMeta(context.Background(), id)
	s.headerMu.Lock()
	defer s.headerMu.Unlock()
	return s.headerIDs[id]
}
