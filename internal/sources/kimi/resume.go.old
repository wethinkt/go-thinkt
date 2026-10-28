package kimi

import (
	"fmt"
	"os/exec"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResumeCommand returns the command to resume a Kimi Code session.
// Uses `kimi --session <id>` run from the correct project directory.
// Kimi stores ProjectPath as an MD5 hash, so we resolve it back to the
// real filesystem path via the kimi.json work directory mapping.
func (s *Store) ResumeCommand(session thinkt.SessionMeta) (*thinkt.ResumeInfo, error) {
	bin, err := exec.LookPath("kimi")
	if err != nil {
		return nil, fmt.Errorf("kimi CLI not found: %w", err)
	}

	// Resolve the hash-based ProjectPath to a real directory
	dir, err := s.resolveProjectDir(session.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve project directory: %w", err)
	}

	return &thinkt.ResumeInfo{
		Command: bin,
		Args:    []string{"kimi", "--session", session.ID},
		Dir:     dir,
	}, nil
}

// resolveProjectDir maps a project hash back to its real filesystem path.
func (s *Store) resolveProjectDir(hashOrPath string) (string, error) {
	// If it's already an absolute path, use it directly
	if len(hashOrPath) > 0 && hashOrPath[0] == '/' {
		return hashOrPath, nil
	}

	workDirs, err := s.loadWorkDirs()
	if err != nil {
		return "", err
	}

	for _, wd := range workDirs {
		if workDirHash(wd.Path) == hashOrPath {
			return wd.Path, nil
		}
	}

	return "", fmt.Errorf("no project found for hash %s", hashOrPath)
}
