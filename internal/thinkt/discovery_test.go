package thinkt

import (
	"context"
	"errors"
	"testing"
)

// mockFactory is a mock StoreFactory for testing.
type mockFactory struct {
	source      Source
	available   bool
	createErr   error
	availableErr error
	store       Store
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

func TestSourceDisplayName(t *testing.T) {
	tests := []struct {
		source   Source
		expected string
	}{
		{SourceKimi, "Kimi Code"},
		{SourceClaude, "Claude Code"},
		{"unknown", "unknown"},
	}
	
	for _, tt := range tests {
		got := sourceDisplayName(tt.source)
		if got != tt.expected {
			t.Errorf("sourceDisplayName(%q) = %q, want %q", tt.source, got, tt.expected)
		}
	}
}

func TestSourceDescription(t *testing.T) {
	tests := []struct {
		source   Source
		expected string
	}{
		{SourceKimi, "Kimi Code sessions (~/.kimi)"},
		{SourceClaude, "Claude Code sessions (~/.claude)"},
		{"unknown", "unknown sessions"},
	}
	
	for _, tt := range tests {
		got := sourceDescription(tt.source)
		if got != tt.expected {
			t.Errorf("sourceDescription(%q) = %q, want %q", tt.source, got, tt.expected)
		}
	}
}
