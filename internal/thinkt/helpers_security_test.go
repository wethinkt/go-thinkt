package thinkt

import (
	"testing"
)

func TestValidateSessionPath(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		baseDir   string
		wantErr   bool
	}{
		{"valid path", "/home/user/.claude/session.jsonl", "/home/user/.claude", false},
		{"valid path with slash", "/home/user/.claude/session.jsonl", "/home/user/.claude/", false},
		{"invalid path prefix", "/home/user/.claude_backups/secrets.jsonl", "/home/user/.claude", true},
		{"directory traversal", "/home/user/.claude/../../etc/passwd", "/home/user/.claude", true},
		{"same as base", "/home/user/.claude", "/home/user/.claude", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionPath(tt.sessionID, tt.baseDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSessionPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
