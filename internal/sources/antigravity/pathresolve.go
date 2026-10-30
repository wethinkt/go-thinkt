package antigravity

import (
	"context"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for a transcript
// under the Antigravity home. Its project comes from the summaries
// database, so ProjectID stays empty.
func (d *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	return thinkt.SessionPath{Source: thinkt.SourceAntigravity, BaseDir: BaseDirForPath(path), SessionID: path}, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (d *Discoverer) OpenSessionPath(ctx context.Context, sp thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	base := sp.BaseDir
	if base == "" {
		base, _ = DefaultDir()
	}
	reader, err := NewStore(base).OpenSession(ctx, path)
	if err != nil {
		return nil, err
	}
	return thinkt.NewLazySession(reader)
}
