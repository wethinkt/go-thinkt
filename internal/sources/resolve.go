package sources

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// resolveOrder is the order sources are asked whether a path is theirs. It
// mirrors the switch the TUI loader used before resolvers existed; the
// sources' path rules overlap, and the earlier source wins. It is not the
// order of AllFactories. Claude is not listed: it is only reached through
// the content sniff of a .jsonl file no source here claimed.
var resolveOrder = []thinkt.Source{
	thinkt.SourceKimi,
	thinkt.SourceAntigravity,
	thinkt.SourceCodex,
	thinkt.SourceCopilot,
	thinkt.SourceQwen,
	thinkt.SourceGemini,
}

// resolver returns the path resolver of source s, or nil.
func resolver(s thinkt.Source) thinkt.SessionPathResolver {
	r, _ := Factory(s).(thinkt.SessionPathResolver)
	return r
}

// ResolveSessionPath names the source a session file belongs to. Sources
// are asked in resolveOrder; a .jsonl file none of them claims is sniffed
// for Claude, Codex or Qwen records. ok is false when nothing claims path.
func ResolveSessionPath(path string) (thinkt.SessionPath, thinkt.SessionPathResolver, bool) {
	for _, s := range resolveOrder {
		r := resolver(s)
		if r == nil {
			continue
		}
		if sp, ok := r.ResolveSessionPath(path); ok {
			return sp, r, true
		}
	}
	if filepath.Ext(path) != ".jsonl" {
		return thinkt.SessionPath{}, nil, false
	}
	s, err := sniffJSONL(path)
	if err != nil || s == "" {
		return thinkt.SessionPath{}, nil, false
	}
	r := resolver(s)
	if r == nil {
		return thinkt.SessionPath{}, nil, false
	}
	if sp, ok := r.ResolveSessionPath(path); ok && sp.Source == s {
		return sp, r, true
	}
	return thinkt.SessionPath{Source: s, SessionID: path}, r, true
}

// OpenSessionPath opens the session file at path with the source that
// claims it. A file the user names opens even when its source is turned
// off.
func OpenSessionPath(ctx context.Context, path string) (thinkt.LazySession, error) {
	sp, r, ok := ResolveSessionPath(path)
	if !ok {
		return nil, fmt.Errorf("unsupported session format: %s", path)
	}
	ls, err := r.OpenSessionPath(ctx, sp, path)
	if err != nil {
		return nil, fmt.Errorf("open %s session: %w", sp.Source, err)
	}
	return ls, nil
}
