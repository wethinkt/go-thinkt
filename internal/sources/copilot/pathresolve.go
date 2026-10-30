package copilot

import (
	"context"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for what
// IsSessionPath claims. BaseDir is the parent of the session-state folder
// above the file.
func (d *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	return thinkt.SessionPath{Source: thinkt.SourceCopilot, BaseDir: layoutBaseDir(path), SessionID: path}, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (d *Discoverer) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	reader, err := NewStore(sp.BaseDir).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}

// layoutBaseDir finds the Copilot base directory from a session path of the
// form .../session-state/{session-id}/events.jsonl, or "".
func layoutBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(dir) == "session-state" {
			return parent
		}
		dir = parent
	}
	return ""
}
