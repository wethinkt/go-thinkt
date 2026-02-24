package thinkt

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateSessionPath(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}

	validPath := filepath.Join(baseDir, "session.jsonl")
	if err := os.WriteFile(validPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to create valid session file: %v", err)
	}

	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	outsidePath := filepath.Join(outsideDir, "secrets.jsonl")
	if err := os.WriteFile(outsidePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	tests := []struct {
		name      string
		sessionID string
		baseDir   string
		wantErr   bool
	}{
		{"valid path", validPath, baseDir, false},
		{"valid path with slash", validPath, baseDir + string(os.PathSeparator), false},
		{"invalid sibling prefix", filepath.Join(tmpDir, "base_backups", "secrets.jsonl"), baseDir, true},
		{"directory traversal", filepath.Join(baseDir, "..", "outside", "secrets.jsonl"), baseDir, true},
		{"same as base", baseDir, baseDir, false},
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

func TestValidateSessionPathRejectsSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	outsidePath := filepath.Join(outsideDir, "secret.jsonl")
	if err := os.WriteFile(outsidePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	linkPath := filepath.Join(baseDir, "link")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		// Windows may require elevated privileges for symlink creation.
		if runtime.GOOS == "windows" && (errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrInvalid)) {
			t.Skipf("skipping symlink test on windows: %v", err)
		}
		t.Fatalf("failed to create symlink: %v", err)
	}

	escapedPath := filepath.Join(linkPath, "secret.jsonl")
	if err := ValidateSessionPath(escapedPath, baseDir); err == nil {
		t.Fatalf("expected symlink escape path to be rejected: %s", escapedPath)
	}
}
