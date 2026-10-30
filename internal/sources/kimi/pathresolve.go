package kimi

import (
	"context"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for a wire file
// anywhere on disk: .../sessions/<wd>/<session>/agents/<agent>/wire.jsonl.
// The wd_* folder is not a project id, so ProjectID stays empty.
func (d *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	base, _, _, ok := splitWirePath(path)
	if !ok {
		return thinkt.SessionPath{}, false
	}
	return thinkt.SessionPath{Source: thinkt.SourceKimi, BaseDir: base, SessionID: path}, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (d *Discoverer) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	base := sp.BaseDir
	if base == "" {
		base, _ = defaultDir()
	}
	reader, err := NewStore(base).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}
