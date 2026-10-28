package kimi

import (
	"fmt"
	"os/exec"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume a Kimi Code session:
// `kimi --session <id>` run from the session's project directory. A
// subagent id ("<session>/<agent>") resumes its parent session.
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("kimi")
	if err != nil {
		return nil, fmt.Errorf("kimi CLI not found: %w", err)
	}
	dir := session.ProjectPath
	if dir == "unknown" {
		dir = ""
	}
	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"kimi", "--session", rootSessionID(session.ID)},
		Dir:     dir,
	}, nil
}
