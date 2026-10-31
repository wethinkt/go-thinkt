package claude

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume a Claude Code session.
// claude --resume requires running from the correct project directory,
// so we resolve the project path, preferring sessions-index.json's
// originalPath over DecodeDirName (which can mis-decode hyphens in paths).
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	dir := resolveProjectPath(session)

	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"claude", "--resume", session.ID},
		Dir:     dir,
	}, nil
}

// resolveProjectPath determines the real filesystem path for a session's project.
// It tries, in order:
//  1. sessions-index.json originalPath (ground truth written by Claude)
//  2. DecodeDirName from the encoded directory name
//  3. session.ProjectPath as-is
func resolveProjectPath(session thinkt.SessionMeta) string {
	if session.FullPath != "" {
		if path := projectPath(filepath.Dir(session.FullPath)); path != "" {
			return path
		}
	}
	return session.ProjectPath
}
