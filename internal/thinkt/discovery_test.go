package thinkt

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockFactory is a mock StoreFactory for testing.
type mockFactory struct {
	source       Source
	available    bool
	createErr    error
	availableErr error
	store        Store
}

func (m *mockFactory) Source() Source {
	return m.source
}

func (m *mockFactory) Create() (Store, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if !m.available {
		return nil, nil
	}
	return m.store, nil
}

func (m *mockFactory) IsAvailable() (bool, error) {
	if m.availableErr != nil {
		return false, m.availableErr
	}
	return m.available, nil
}

func TestDiscovery_Register(t *testing.T) {
	d := NewDiscovery()

	f1 := &mockFactory{source: SourceKimi}
	f2 := &mockFactory{source: SourceClaude}

	d.Register(f1)
	d.Register(f2)

	factories := d.Factories()
	if len(factories) != 2 {
		t.Errorf("expected 2 factories, got %d", len(factories))
	}
}

func TestDiscovery_Discover(t *testing.T) {
	kimiStore := &mockStore{source: SourceKimi, projects: []Project{{ID: "p1"}}}
	claudeStore := &mockStore{source: SourceClaude, projects: []Project{{ID: "p2"}}}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: kimiStore},
		&mockFactory{source: SourceClaude, available: true, store: claudeStore},
		&mockFactory{source: "unknown", available: false}, // Not available
	)

	ctx := context.Background()
	registry, err := d.Discover(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 stores (only the available ones with projects)
	all := registry.All()
	if len(all) != 2 {
		t.Errorf("expected 2 stores, got %d", len(all))
	}

	// Verify specific stores
	if _, ok := registry.Get(SourceKimi); !ok {
		t.Error("expected kimi store")
	}
	if _, ok := registry.Get(SourceClaude); !ok {
		t.Error("expected claude store")
	}
}

func TestDiscovery_Discover_WithErrors(t *testing.T) {
	kimiStore := &mockStore{source: SourceKimi, projects: []Project{{ID: "p1"}}}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: kimiStore},
		&mockFactory{source: SourceClaude, createErr: errors.New("failed")},
	)

	ctx := context.Background()
	registry, err := d.Discover(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have only 1 store (kimi), claude failed but shouldn't stop discovery
	all := registry.All()
	if len(all) != 1 {
		t.Errorf("expected 1 store, got %d", len(all))
	}
}

func TestDiscovery_DiscoverAvailable(t *testing.T) {
	kimiStore := &mockStore{source: SourceKimi, projects: []Project{{ID: "k1"}, {ID: "k2"}}}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: kimiStore},
		&mockFactory{source: SourceClaude, available: false},
	)

	ctx := context.Background()
	available, err := d.DiscoverAvailable(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(available) != 1 {
		t.Fatalf("expected 1 available source, got %d", len(available))
	}

	if available[0].Source != SourceKimi {
		t.Errorf("expected kimi, got %s", available[0].Source)
	}

	if available[0].ProjectCount != 2 {
		t.Errorf("expected 2 projects, got %d", available[0].ProjectCount)
	}

	if !available[0].Available {
		t.Error("expected Available=true")
	}
}

// detailedMockStore extends mockStore with per-project session data.
type detailedMockStore struct {
	source   Source
	projects []Project
	sessions map[string][]SessionMeta // projectID -> sessions
}

func (m *detailedMockStore) Source() Source                { return m.source }
func (m *detailedMockStore) Workspace() Workspace         { return Workspace{ID: "test-ws", Source: m.source, BasePath: "/test"} }
func (m *detailedMockStore) ListProjects(ctx context.Context) ([]Project, error) {
	return m.projects, nil
}
func (m *detailedMockStore) GetProject(ctx context.Context, id string) (*Project, error) {
	return nil, nil
}
func (m *detailedMockStore) ListSessions(ctx context.Context, projectID string, _ ...ListSessionsOption) ([]SessionMeta, error) {
	return m.sessions[projectID], nil
}
func (m *detailedMockStore) GetSessionMeta(ctx context.Context, sessionID string) (*SessionMeta, error) {
	return nil, nil
}
func (m *detailedMockStore) LoadSession(ctx context.Context, sessionID string) (*Session, error) {
	return nil, nil
}
func (m *detailedMockStore) OpenSession(ctx context.Context, sessionID string) (SessionReader, error) {
	return nil, nil
}
func (m *detailedMockStore) WatchConfig() WatchConfig { return DefaultWatchConfig() }

func TestDiscoverDetailed(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)

	store := &detailedMockStore{
		source: SourceKimi,
		projects: []Project{
			{ID: "p1", Name: "project-one"},
			{ID: "p2", Name: "project-two"},
		},
		sessions: map[string][]SessionMeta{
			"p1": {
				{ID: "s1", FileSize: 1000, CreatedAt: t1, ModifiedAt: t2},
				{ID: "s2", FileSize: 2000, CreatedAt: t3, ModifiedAt: t3},
			},
			"p2": {
				{ID: "s3", FileSize: 500, CreatedAt: t2, ModifiedAt: t2},
			},
		},
	}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: store},
	)

	var progressCalls []DetailedSourceInfo
	ctx := context.Background()
	result, err := d.DiscoverDetailed(ctx, func(info DetailedSourceInfo) {
		progressCalls = append(progressCalls, info)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 detailed source, got %d", len(result))
	}

	info := result[0]
	if info.Source != SourceKimi {
		t.Errorf("expected source kimi, got %s", info.Source)
	}
	if info.SessionCount != 3 {
		t.Errorf("expected 3 sessions, got %d", info.SessionCount)
	}
	if info.TotalSize != 3500 {
		t.Errorf("expected total size 3500, got %d", info.TotalSize)
	}
	if !info.FirstSession.Equal(t1) {
		t.Errorf("expected first session %v, got %v", t1, info.FirstSession)
	}
	if !info.LastSession.Equal(t2) {
		t.Errorf("expected last session %v, got %v", t2, info.LastSession)
	}
	if len(info.Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(info.Projects))
	}

	// Progress callback should have been called once (one source)
	if len(progressCalls) != 1 {
		t.Errorf("expected 1 progress call, got %d", len(progressCalls))
	}
	if progressCalls[0].Source != SourceKimi {
		t.Errorf("expected progress call for kimi, got %s", progressCalls[0].Source)
	}
}

func TestDiscoverDetailedSkipsUnavailable(t *testing.T) {
	store := &detailedMockStore{
		source:   SourceKimi,
		projects: []Project{{ID: "p1"}},
		sessions: map[string][]SessionMeta{
			"p1": {{ID: "s1", FileSize: 100, CreatedAt: time.Now(), ModifiedAt: time.Now()}},
		},
	}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: store},
		&mockFactory{source: SourceClaude, available: false},
		&mockFactory{source: SourceGemini, availableErr: errors.New("check failed")},
	)

	ctx := context.Background()
	result, err := d.DiscoverDetailed(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 detailed source, got %d", len(result))
	}
	if result[0].Source != SourceKimi {
		t.Errorf("expected kimi, got %s", result[0].Source)
	}
}

func TestDiscoverDetailedNilProgress(t *testing.T) {
	store := &detailedMockStore{
		source:   SourceKimi,
		projects: []Project{{ID: "p1"}},
		sessions: map[string][]SessionMeta{
			"p1": {{ID: "s1", FileSize: 100, CreatedAt: time.Now(), ModifiedAt: time.Now()}},
		},
	}

	d := NewDiscovery(
		&mockFactory{source: SourceKimi, available: true, store: store},
	)

	ctx := context.Background()
	// Should not panic with nil progress
	result, err := d.DiscoverDetailed(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}
