package claude

import "testing"

func TestDecodeDirName(t *testing.T) {
	tests := []struct {
		input       string
		wantDisplay string
		wantPath    string
	}{
		{"-Users-evan-brainstm-foo", "foo", "/Users/evan/brainstm/foo"},
		// Hyphens in names are ambiguous — best-effort decode
		{"-Users-evan-brainstm-thinking-tracer-tools", "tools", "/Users/evan/brainstm/thinking/tracer/tools"},
		{"-", "~", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotDisplay, gotPath := DecodeDirName(tt.input)
			if gotDisplay != tt.wantDisplay {
				t.Errorf("DecodeDirName(%q) display = %q, want %q", tt.input, gotDisplay, tt.wantDisplay)
			}
			if gotPath != tt.wantPath {
				t.Errorf("DecodeDirName(%q) path = %q, want %q", tt.input, gotPath, tt.wantPath)
			}
		})
	}
}

func TestListProjects(t *testing.T) {
	// Integration test: only runs when ~/.claude exists
	projects, err := ListProjects("")
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}

	if len(projects) == 0 {
		t.Skip("No projects found")
	}

	for _, p := range projects {
		if p.DirName == "" {
			t.Error("Project DirName is empty")
		}
		if p.DisplayName == "" {
			t.Error("Project DisplayName is empty")
		}
		if p.SessionCount == 0 {
			t.Errorf("Project %q has 0 sessions (should have been filtered)", p.DisplayName)
		}
	}
}

func TestListProjectSessions(t *testing.T) {
	// Integration test: find a project directory with sessions-index.json
	projects, err := ListProjects("")
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found")
	}

	sessions, err := ListProjectSessions(projects[0].DirPath)
	if err != nil {
		t.Fatalf("ListProjectSessions() error = %v", err)
	}

	if len(sessions) == 0 {
		t.Skip("No sessions found in first project")
	}

	// Verify ascending sort
	for i := 1; i < len(sessions); i++ {
		if sessions[i].Created.Before(sessions[i-1].Created) {
			t.Errorf("Sessions not sorted ascending: [%d]=%v > [%d]=%v",
				i-1, sessions[i-1].Created, i, sessions[i].Created)
		}
	}
}
