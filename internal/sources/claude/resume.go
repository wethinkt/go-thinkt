package claude

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume a Claude Code session.
// claude --resume requires running from the correct project directory,
// so we decode the project path from the session's FullPath.
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	// session.ProjectPath is the ~/.claude/projects/<encoded> dir path.
	// Decode the directory name to get the real filesystem project path.
	// e.g. "-Users-evan-wethinkt-go-thinkt" → "/Users/evan/wethinkt/go-thinkt"
	dir := session.ProjectPath
	if session.FullPath != "" {
		projectDir := filepath.Dir(session.FullPath)
		_, decoded := DecodeDirName(filepath.Base(projectDir))
		if decoded != "" {
			dir = decoded
		}
	}

	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"claude", "--resume", session.ID},
		Dir:     dir,
	}, nil
}
