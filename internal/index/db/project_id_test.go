package db

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestScopedProjectID(t *testing.T) {
	tests := []struct {
		source    thinkt.Source
		projectID string
		want      string
	}{
		{thinkt.SourceClaude, "my-project", "claude::my-project"},
		{thinkt.SourceKimi, "my-project", "kimi::my-project"},
		{"", "my-project", "my-project"},
	}
	for _, tt := range tests {
		got := ScopedProjectID(tt.source, tt.projectID)
		if got != tt.want {
			t.Errorf("ScopedProjectID(%q, %q) = %q, want %q", tt.source, tt.projectID, got, tt.want)
		}
	}
}

func TestScopedProjectIDCandidates(t *testing.T) {
	candidates := ScopedProjectIDCandidates(thinkt.SourceClaude, "my-project")
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0] != "my-project" || candidates[1] != "claude::my-project" {
		t.Fatalf("unexpected candidates: %v", candidates)
	}

	candidates = ScopedProjectIDCandidates("", "my-project")
	if len(candidates) != 1 || candidates[0] != "my-project" {
		t.Fatalf("unexpected candidates for empty source: %v", candidates)
	}
}
