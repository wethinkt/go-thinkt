package claude

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestDiscoverer_Source(t *testing.T) {
	d := NewDiscoverer()
	if d.Source() != thinkt.SourceClaude {
		t.Errorf("expected SourceClaude, got %s", d.Source())
	}
}

func TestDiscoverer_Create_NotAvailable(t *testing.T) {
	// This test assumes ~/.claude may or may not exist
	d := NewDiscoverer()
	
	store, err := d.Create()
	// Should not error, but may return nil if not available
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	
	// If we got a store, verify it's the right type
	if store != nil {
		if store.Source() != thinkt.SourceClaude {
			t.Errorf("expected SourceClaude, got %s", store.Source())
		}
	}
}

func TestDiscoverer_IsAvailable(t *testing.T) {
	d := NewDiscoverer()
	
	// Just verify it doesn't panic
	available, err := d.IsAvailable()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	
	// Result depends on whether ~/.claude exists
	_ = available
}

func TestFactory(t *testing.T) {
	f := Factory()
	
	if f.Source() != thinkt.SourceClaude {
		t.Errorf("expected SourceClaude, got %s", f.Source())
	}
	
	// Verify it implements the interface
	var _ thinkt.StoreFactory = f
}
