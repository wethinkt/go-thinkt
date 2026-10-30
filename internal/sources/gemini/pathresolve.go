package gemini

import (
	"context"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for what
// IsSessionPath claims. The project id is the hash folder above chats/.
func (f *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	base := layoutBaseDir(path)
	if base == "" {
		base = f.basePath()
	}
	sp := thinkt.SessionPath{Source: thinkt.SourceGemini, BaseDir: base, SessionID: path}
	if chats := filepath.Dir(path); filepath.Base(chats) == "chats" {
		sp.ProjectID = filepath.Base(filepath.Dir(chats))
	}
	return sp, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (f *Discoverer) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	reader, err := NewStore(sp.BaseDir).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}

// layoutBaseDir finds the Gemini base directory from a session path of the
// form .../.gemini/tmp/{project-hash}/chats/{chat-id}.json, or "".
func layoutBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(parent) == ".gemini" || filepath.Base(parent) == "gemini" {
			return parent
		}
		dir = parent
	}
	return ""
}
