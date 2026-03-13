package export

import "testing"

func TestToolSummary(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    any
		want     string
	}{
		{"read file", "Read", map[string]any{"file_path": "src/main.go"}, "src/main.go"},
		{"bash command", "Bash", map[string]any{"command": "go test ./..."}, "go test ./..."},
		{"grep pattern", "Grep", map[string]any{"pattern": "TODO", "path": "src/"}, "TODO"},
		{"unknown tool fallback", "CustomTool", map[string]any{"query": "hello"}, "hello"},
		{"nil input", "Read", nil, ""},
		{"non-map input", "Read", "string", ""},
		{"truncate long value", "Bash", map[string]any{"command": "echo " + string(make([]byte, 200))}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolSummary(tt.toolName, tt.input)
			if tt.name == "truncate long value" {
				if len(got) > maxToolSummaryLen+1 {
					t.Errorf("expected truncated, got len=%d", len(got))
				}
				return
			}
			if got != tt.want {
				t.Errorf("toolSummary(%q, %v) = %q, want %q", tt.toolName, tt.input, got, tt.want)
			}
		})
	}
}
