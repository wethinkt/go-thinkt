package thinkt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveSessionByPath finds the store and session metadata for an absolute
// session file path across all registered sources.
func (r *StoreRegistry) ResolveSessionByPath(ctx context.Context, sessionPath string) (Store, *SessionMeta, error) {
	if sessionPath == "" {
		return nil, nil, fmt.Errorf("session path is required")
	}

	cleanPath := filepath.Clean(sessionPath)

	// Fast path: ask each store if it can resolve this path directly.
	for _, store := range r.All() {
		meta, err := store.GetSessionMeta(ctx, sessionPath)
		if err != nil || meta == nil {
			continue
		}
		if samePath(meta.FullPath, cleanPath) {
			return store, meta, nil
		}
	}

	// Fallback: scan project/session listings and match by FullPath.
	for _, store := range r.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, project := range projects {
			sessions, err := store.ListSessions(ctx, project.ID)
			if err != nil {
				continue
			}
			for i := range sessions {
				if samePath(sessions[i].FullPath, cleanPath) {
					meta := sessions[i]
					return store, &meta, nil
				}
			}
		}
	}

	return nil, nil, os.ErrNotExist
}

// OpenLazySessionByPath resolves the owning source store for a session file path,
// opens it via the store's SessionReader, and wraps it in a generic lazy session.
func (r *StoreRegistry) OpenLazySessionByPath(ctx context.Context, sessionPath string) (LazySession, error) {
	store, meta, err := r.ResolveSessionByPath(ctx, sessionPath)
	if err != nil {
		return nil, err
	}
	if store == nil || meta == nil {
		return nil, os.ErrNotExist
	}

	sessionID := meta.ID
	if sessionID == "" {
		sessionID = meta.FullPath
	}

	reader, err := store.OpenSession(ctx, sessionID)
	if (err != nil || reader == nil) && meta.FullPath != "" && meta.FullPath != sessionID {
		reader, err = store.OpenSession(ctx, meta.FullPath)
	}
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, os.ErrNotExist
	}

	ls, err := NewLazySession(reader)
	if err != nil {
		return nil, err
	}

	// Prefer metadata discovered from index/listing when the reader leaves fields empty.
	if ls.Meta.FullPath == "" {
		ls.Meta.FullPath = meta.FullPath
	}
	if ls.Meta.Source == "" {
		ls.Meta.Source = meta.Source
	}
	if ls.Meta.ID == "" {
		ls.Meta.ID = meta.ID
	}

	return ls, nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
