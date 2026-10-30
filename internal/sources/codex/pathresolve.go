package codex

import (
	"context"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for what
// IsSessionPath claims. BaseDir is the .codex directory above the file, or
// the Codex home when the file is under it.
func (d *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	base := layoutBaseDir(path)
	if base == "" && thinkt.WithinHome(path, d.basePath()) {
		base = d.basePath()
	}
	return thinkt.SessionPath{Source: thinkt.SourceCodex, BaseDir: base, SessionID: thinkt.HomeSpelling(path, base)}, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (d *Discoverer) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	base := sp.BaseDir
	if base == "" {
		base = layoutBaseDir(path)
	}
	reader, err := NewStore(base).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}

// layoutBaseDir finds the Codex base directory from a session path of the
// form .../.codex/sessions/YYYY/MM/DD/*.jsonl, or "".
func layoutBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(dir) == ".codex" || filepath.Base(dir) == "codex" {
			return dir
		}
		if filepath.Base(parent) == ".codex" {
			return parent
		}
		dir = parent
	}
	return ""
}
