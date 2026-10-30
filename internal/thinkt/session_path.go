package thinkt

import (
	"context"
	"path/filepath"
)

// SessionPath is what a source can tell about a session file from its path.
type SessionPath struct {
	Source    Source
	BaseDir   string // the source home the file sits in, from the path layout; empty if unknown
	SessionID string // the id GetSessionMeta and OpenSession accept
	ProjectID string // set only when it is the store's Project.ID
}

// SessionPathResolver is implemented by a StoreFactory whose source can
// recognise its own session files.
type SessionPathResolver interface {
	// ResolveSessionPath reports whether path is one of this source's
	// session files, by its path only.
	ResolveSessionPath(path string) (SessionPath, bool)
	// OpenSessionPath opens path as a lazy session, through a store rooted
	// at sp.BaseDir when it is set.
	OpenSessionPath(ctx context.Context, sp SessionPath, path string) (LazySession, error)
}

// WithinHome reports whether path is home or below it, also when either
// one reaches the other through a symlink. An empty home matches nothing.
func WithinHome(path, home string) bool {
	if home == "" {
		return false
	}
	if IsPathWithinAny(path, []string{home}) {
		return true
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return false
	}
	realPath := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		realPath = r
	} else if r, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		realPath = filepath.Join(r, filepath.Base(path))
	}
	return IsPathWithinAny(realPath, []string{realHome})
}
