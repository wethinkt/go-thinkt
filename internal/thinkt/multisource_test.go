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

func TestRegistry_FindProjectForPath(t *testing.T) {
	reg := NewRegistry()

	// Set up projects with various paths
	kimi := &mockStore{
		source: SourceKimi,
		projects: []Project{
			{ID: "p1", Path: "/Users/evan/projects/foo", Source: SourceKimi},
			{ID: "p2", Path: "/Users/evan/projects/foobar", Source: SourceKimi}, // Similar prefix, different project
			{ID: "wp1", Path: `C:\Users\evan\projects\foo`, Source: SourceKimi},
			{ID: "wp2", Path: `C:\Users\evan\projects\foobar`, Source: SourceKimi}, // Similar prefix, different project
		},
	}
	claude := &mockStore{
		source: SourceClaude,
		projects: []Project{
			{ID: "p3", Path: "/Users/evan/projects/foo/subproject", Source: SourceClaude}, // Nested in p1
			{ID: "p4", Path: "/Users/evan/other", Source: SourceClaude},
			{ID: "wp3", Path: `C:\Users\evan\projects\foo\subproject`, Source: SourceClaude}, // Nested in wp1
		},
	}

	reg.Register(kimi)
	reg.Register(claude)

	ctx := context.Background()

	tests := []struct {
		name     string
		path     string
		wantID   string
		wantNil  bool
	}{
		{
			name:   "exact match",
			path:   "/Users/evan/projects/foo",
			wantID: "p1",
		},
		{
			name:   "subdirectory match",
			path:   "/Users/evan/projects/foo/src/main.go",
			wantID: "p1",
		},
		{
			name:   "most specific match (nested project)",
			path:   "/Users/evan/projects/foo/subproject/file.go",
			wantID: "p3", // Should match the nested project, not the parent
		},
		{
			name:   "exact nested project",
			path:   "/Users/evan/projects/foo/subproject",
			wantID: "p3",
		},
		{
			name:    "no match",
			path:    "/Users/evan/unknown/path",
			wantNil: true,
		},
		{
			name:   "similar prefix but different project",
			path:   "/Users/evan/projects/foobar/file.go",
			wantID: "p2", // Should match foobar, not foo
		},
		{
			name:    "partial prefix should not match",
			path:    "/Users/evan/projects/foob", // Not a real subdir of foo
			wantNil: true,
		},
		{
			name:   "windows path exact match",
			path:   `C:\Users\evan\projects\foo`,
			wantID: "wp1",
		},
		{
			name:   "windows path subdirectory match",
			path:   `C:\Users\evan\projects\foo\src\main.go`,
			wantID: "wp1",
		},
		{
			name:   "windows most specific nested project",
			path:   `C:\Users\evan\projects\foo\subproject\file.go`,
			wantID: "wp3",
		},
		{
			name:   "windows similar prefix but different project",
			path:   `C:\Users\evan\projects\foobar\file.go`,
			wantID: "wp2",
		},
		{
			name:    "windows partial prefix should not match",
			path:    `C:\Users\evan\projects\foob`,
			wantNil: true,
		},
		{
			name:   "windows drive letter case-insensitive",
			path:   `c:\users\evan\projects\foo\README.md`,
			wantID: "wp1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reg.FindProjectForPath(ctx, tt.path)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got project %s", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected project %s, got nil", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Errorf("expected project %s, got %s", tt.wantID, got.ID)
			}
		})
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
