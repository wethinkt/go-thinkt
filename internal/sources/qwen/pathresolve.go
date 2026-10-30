package qwen

import (
	"context"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for what
// IsSessionPath claims. BaseDir is the .qwen folder above projects/, or the
// Qwen home when the file is under it; the project id is the hash folder
// above chats/.
func (f *qwenFactory) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	base := layoutBaseDir(path)
	if base == "" && thinkt.WithinHome(path, getBaseDir()) {
		base = getBaseDir()
	}
	sp := thinkt.SessionPath{Source: thinkt.SourceQwen, BaseDir: base, SessionID: thinkt.HomeSpelling(path, base)}
	if chats := filepath.Dir(path); filepath.Base(chats) == "chats" {
		sp.ProjectID = filepath.Base(filepath.Dir(chats))
	}
	return sp, true
}

// OpenSessionPath implements thinkt.SessionPathResolver. An empty BaseDir
// keeps the store's default home.
func (f *qwenFactory) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	reader, err := NewStore(sp.BaseDir).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}

// layoutBaseDir returns the .qwen directory directly above a projects/
// folder on the path, or "".
func layoutBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if filepath.Base(dir) == "projects" && filepath.Base(parent) == ".qwen" {
			return parent
		}
		dir = parent
	}
}
