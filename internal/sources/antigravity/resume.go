package antigravity

import (
	"fmt"
	"os/exec"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume an Antigravity conversation:
// `agy --conversation <id>` run from the conversation's workspace folder.
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("agy")
	if err != nil {
		return nil, fmt.Errorf("agy CLI not found: %w", err)
	}
	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"agy", "--conversation", session.ID},
		Dir:     session.ProjectPath,
	}, nil
}
