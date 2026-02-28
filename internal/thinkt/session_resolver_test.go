package thinkt

import (
	"context"
	"io"
	"os"
	"testing"
)

type resolverMockStore struct {
	source        Source
	metas         map[string]*SessionMeta
	openResponses map[string]resolverOpenResponse
	getMetaCalls  []string
	openCalls     []string
}

type resolverOpenResponse struct {
	reader SessionReader
	err    error
}

func (m *resolverMockStore) Source() Source { return m.source }

func (m *resolverMockStore) Workspace() Workspace {
	return Workspace{ID: "test-workspace", Source: m.source}
}

func (m *resolverMockStore) ListProjects(ctx context.Context) ([]Project, error) {
	return nil, nil
}

func (m *resolverMockStore) GetProject(ctx context.Context, id string) (*Project, error) {
	return nil, nil
}

func (m *resolverMockStore) ListSessions(ctx context.Context, projectID string) ([]SessionMeta, error) {
	return nil, nil
}

func (m *resolverMockStore) GetSessionMeta(ctx context.Context, sessionID string) (*SessionMeta, error) {
	m.getMetaCalls = append(m.getMetaCalls, sessionID)
	meta := m.metas[sessionID]
	if meta == nil {
		return nil, nil
	}
	copy := *meta
	return &copy, nil
}

func (m *resolverMockStore) LoadSession(ctx context.Context, sessionID string) (*Session, error) {
	return nil, nil
}

func (m *resolverMockStore) OpenSession(ctx context.Context, sessionID string) (SessionReader, error) {
	m.openCalls = append(m.openCalls, sessionID)
	resp, ok := m.openResponses[sessionID]
	if !ok {
		return nil, nil
	}
	return resp.reader, resp.err
}
func (m *resolverMockStore) WatchConfig() WatchConfig { return DefaultWatchConfig() }

type staticSessionReader struct {
	meta SessionMeta
}

func (r *staticSessionReader) ReadNext() (*Entry, error) {
	return nil, io.EOF
}

func (r *staticSessionReader) Metadata() SessionMeta {
	return r.meta
}

func (r *staticSessionReader) Close() error {
	return nil
}

func TestResolveSessionByPath_CleansInputPathForLookup(t *testing.T) {
	reg := NewRegistry()
	path := "/tmp/project/session.jsonl"
	meta := &SessionMeta{ID: "sess", FullPath: path, Source: SourceClaude}

	store := &resolverMockStore{
		source: SourceClaude,
		metas:  map[string]*SessionMeta{path: meta},
	}
	reg.Register(store)

	_, gotMeta, err := reg.ResolveSessionByPath(context.Background(), "/tmp/project/./session.jsonl")
	if err != nil {
		t.Fatalf("ResolveSessionByPath error: %v", err)
	}
	if gotMeta == nil || gotMeta.FullPath != path {
		t.Fatalf("expected resolved meta with full path %q, got %#v", path, gotMeta)
	}
	if len(store.getMetaCalls) == 0 || store.getMetaCalls[0] != path {
		t.Fatalf("expected cleaned path lookup %q, calls: %v", path, store.getMetaCalls)
	}
}

func TestOpenLazySessionByPath_PrefersFullPathFirst(t *testing.T) {
	reg := NewRegistry()
	path := "/tmp/project/session.jsonl"
	id := "session-uuid"

	store := &resolverMockStore{
		source: SourceClaude,
		metas: map[string]*SessionMeta{
			path: {ID: id, FullPath: path, Source: SourceClaude},
		},
		openResponses: map[string]resolverOpenResponse{
			path: {
				reader: &staticSessionReader{
					meta: SessionMeta{FullPath: path, Source: SourceClaude},
				},
			},
			id: {
				err: os.ErrNotExist,
			},
		},
	}
	reg.Register(store)

	ls, err := reg.OpenLazySessionByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenLazySessionByPath error: %v", err)
	}
	defer ls.Close()

	if len(store.openCalls) != 1 {
		t.Fatalf("expected exactly one open call, got %v", store.openCalls)
	}
	if store.openCalls[0] != path {
		t.Fatalf("expected first open call on full path %q, got %q", path, store.openCalls[0])
	}
}

func TestOpenLazySessionByPath_FallsBackToID(t *testing.T) {
	reg := NewRegistry()
	path := "/tmp/project/session.jsonl"
	id := "session-uuid"

	store := &resolverMockStore{
		source: SourceCodex,
		metas: map[string]*SessionMeta{
			path: {ID: id, FullPath: path, Source: SourceCodex},
		},
		openResponses: map[string]resolverOpenResponse{
			path: {err: os.ErrNotExist},
			id: {
				reader: &staticSessionReader{
					meta: SessionMeta{ID: id, FullPath: path, Source: SourceCodex},
				},
			},
		},
	}
	reg.Register(store)

	ls, err := reg.OpenLazySessionByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenLazySessionByPath error: %v", err)
	}
	defer ls.Close()

	if len(store.openCalls) != 2 {
		t.Fatalf("expected two open calls, got %v", store.openCalls)
	}
	if store.openCalls[0] != path || store.openCalls[1] != id {
		t.Fatalf("expected open order [%q %q], got %v", path, id, store.openCalls)
	}
}
