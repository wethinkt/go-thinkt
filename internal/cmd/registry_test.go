package cmd

import (
	"slices"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
)

func TestConfiguredEnabledSources_UnconstrainedWhenUnset(t *testing.T) {
	allowed, constrained := configuredEnabledSources(config.Config{})
	if constrained {
		t.Fatal("expected unconstrained sources when config has no source settings")
	}
	if allowed != nil {
		t.Fatalf("expected nil allowed list when unconstrained, got %v", allowed)
	}
}

func TestConfiguredEnabledSources_UsesSourcesMap(t *testing.T) {
	cfg := config.Config{
		Sources: map[string]config.SourceConfig{
			"claude": {Enabled: true},
			"kimi":   {Enabled: false},
		},
	}

	allowed, constrained := configuredEnabledSources(cfg)
	if !constrained {
		t.Fatal("expected constrained sources when Sources map is present")
	}

	want := []string{"claude"}
	if !slices.Equal(allowed, want) {
		t.Fatalf("allowed = %v, want %v", allowed, want)
	}
}

func TestConfiguredEnabledSources_AllDisabledWithSourcesMap(t *testing.T) {
	cfg := config.Config{
		Sources: map[string]config.SourceConfig{
			"claude": {Enabled: false},
			"kimi":   {Enabled: false},
		},
	}

	allowed, constrained := configuredEnabledSources(cfg)
	if !constrained {
		t.Fatal("expected constrained sources when Sources map is present")
	}
	if len(allowed) != 0 {
		t.Fatalf("expected no enabled sources, got %v", allowed)
	}
}
