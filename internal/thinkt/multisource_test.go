package thinkt

import (
	"context"
	"testing"
)

// mockStore is a simple mock implementation of Store for testing.
type mockStore struct {
	source   Source
	projects []Project
}

func (m *mockStore) Source() Source                                 { return m.source }
func (m *mockStore) Workspace() Workspace                           { return Workspace{ID: "test", Source: m.source} }
func (m *mockStore) ListProjects(ctx context.Context) ([]Project, error) { return m.projects, nil }
func (m *mockStore) GetProject(ctx context.Context, id string) (*Project, error) { return nil, nil }
func (m *mockStore) ListSessions(ctx context.Context, projectID string) ([]SessionMeta, error) { return nil, nil }
func (m *mockStore) GetSessionMeta(ctx context.Context, sessionID string) (*SessionMeta, error) { return nil, nil }
func (m *mockStore) LoadSession(ctx context.Context, sessionID string) (*Session, error) { return nil, nil }
func (m *mockStore) OpenSession(ctx context.Context, sessionID string) (SessionReader, error) { return nil, nil }

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	
	kimi := &mockStore{source: SourceKimi}
	claude := &mockStore{source: SourceClaude}
	
	reg.Register(kimi)
	reg.Register(claude)
	
	// Test Get
	if s, ok := reg.Get(SourceKimi); !ok || s.Source() != SourceKimi {
		t.Error("expected to get kimi store")
	}
	if s, ok := reg.Get(SourceClaude); !ok || s.Source() != SourceClaude {
		t.Error("expected to get claude store")
	}
	if _, ok := reg.Get("unknown"); ok {
		t.Error("expected not to find unknown source")
	}
}

func TestRegistry_All(t *testing.T) {
	reg := NewRegistry()
	
	kimi := &mockStore{source: SourceKimi}
	claude := &mockStore{source: SourceClaude}
	
	reg.Register(kimi)
	reg.Register(claude)
	
	all := reg.All()
	if len(all) != 2 {
		t.Errorf("expected 2 stores, got %d", len(all))
	}
}

func TestRegistry_Sources(t *testing.T) {
	reg := NewRegistry()
	
	kimi := &mockStore{source: SourceKimi}
	claude := &mockStore{source: SourceClaude}
	
	reg.Register(kimi)
	reg.Register(claude)
	
	sources := reg.Sources()
	if len(sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(sources))
	}
}

func TestRegistry_AvailableSources(t *testing.T) {
	reg := NewRegistry()
	
	// Store with projects
	kimi := &mockStore{
		source:   SourceKimi,
		projects: []Project{{ID: "p1"}},
	}
	// Store without projects
	claude := &mockStore{
		source:   SourceClaude,
		projects: []Project{},
	}
	
	reg.Register(kimi)
	reg.Register(claude)
	
	ctx := context.Background()
	available := reg.AvailableSources(ctx)
	
	if len(available) != 1 || available[0] != SourceKimi {
		t.Errorf("expected only kimi to be available, got %v", available)
	}
}

func TestRegistry_SourceStatus(t *testing.T) {
	reg := NewRegistry()
	
	kimi := &mockStore{
		source:   SourceKimi,
		projects: []Project{{ID: "p1"}, {ID: "p2"}},
	}
	
	reg.Register(kimi)
	
	ctx := context.Background()
	status := reg.SourceStatus(ctx)
	
	if len(status) != 1 {
		t.Fatalf("expected 1 status, got %d", len(status))
	}
	
	info := status[0]
	if info.Source != SourceKimi {
		t.Errorf("expected source kimi, got %s", info.Source)
	}
	if !info.Available {
		t.Error("expected kimi to be available")
	}
	if info.ProjectCount != 2 {
		t.Errorf("expected 2 projects, got %d", info.ProjectCount)
	}
}

func TestMultiStore(t *testing.T) {
	reg := NewRegistry()
	
	kimi := &mockStore{
		source:   SourceKimi,
		projects: []Project{{ID: "k1", Source: SourceKimi}},
	}
	claude := &mockStore{
		source:   SourceClaude,
		projects: []Project{{ID: "c1", Source: SourceClaude}},
	}
	
	reg.Register(kimi)
	reg.Register(claude)
	
	ms := NewMultiStore(reg)
	
	// Test AllSources
	sources := ms.AllSources()
	if len(sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(sources))
	}
	
	// Test GetStore
	if s, ok := ms.GetStore(SourceKimi); !ok || s.Source() != SourceKimi {
		t.Error("expected to get kimi store from MultiStore")
	}
	
	// Test ListAllProjects
	ctx := context.Background()
	projects, err := ms.ListAllProjects(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects from all sources, got %d", len(projects))
	}
}
