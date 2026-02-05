package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestValidateNoShellMetacharacters(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{"simple path", "/home/user/project", false},
		{"path with spaces", "/home/user/my project", false},
		{"path with dots", "/home/user/project.v2", false},
		{"path with dash", "/home/user/my-project", false},
		{"path with underscore", "/home/user/my_project", false},
		{"path with numbers", "/home/user/project123", false},

		// Invalid - command separators
		{"semicolon", "/home/user/project;rm -rf /", true},
		{"pipe", "/home/user/project|cat /etc/passwd", true},
		{"ampersand", "/home/user/project&&evil", true},

		// Invalid - variable expansion
		{"dollar", "/home/user/$HOME", true},
		{"backtick", "/home/user/`whoami`", true},

		// Invalid - redirects
		{"less than", "/home/user/project</etc/passwd", true},
		{"greater than", "/home/user/project>/tmp/pwned", true},

		// Invalid - quotes
		{"double quote", `/home/user/project"`, true},
		{"single quote", "/home/user/project'", true},
		{"backslash", "/home/user/project\\", true},

		// Invalid - other
		{"newline", "/home/user/project\nrm -rf /", true},
		{"null byte", "/home/user/project\x00/etc/passwd", true},
		{"glob", "/home/user/*", true},
		{"question", "/home/user/project?", true},
		{"bracket", "/home/user/project[abc]", true},
		{"tilde", "~/project", true},
		{"leading dash", "-rf /", true},
		{"hash", "/home/user/project#comment", true},
		{"exclamation", "/home/user/project!history", true},
		{"parentheses", "/home/user/project$(whoami)", true},
		{"curly braces", "/home/user/project${VAR}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoShellMetacharacters(tt.path)
			if tt.wantErr && err == nil {
				t.Errorf("validateNoShellMetacharacters(%q) expected error, got nil", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateNoShellMetacharacters(%q) unexpected error: %v", tt.path, err)
			}
		})
	}
}

func TestIsPathWithinAny(t *testing.T) {
	// Use platform-appropriate paths
	var bases []string
	if runtime.GOOS == "windows" {
		bases = []string{
			`C:\Users\alice`,
			`C:\Projects`,
		}
	} else {
		bases = []string{
			"/home/alice",
			"/projects",
		}
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Exact matches
		{"exact home", bases[0], true},
		{"exact projects", bases[1], true},

		// Subdirectories
		{"subdir of home", filepath.Join(bases[0], "project"), true},
		{"nested subdir", filepath.Join(bases[0], "project", "src"), true},
		{"subdir of projects", filepath.Join(bases[1], "foo"), true},

		// Outside
		{"different user", "/home/bob", false},
		{"system path", "/etc", false},
		{"root", "/", false},

		// Edge cases - prefix matching (should NOT match)
		{"prefix of base (false positive check)", bases[0] + "evil", false},
		{"prefix of projects", bases[1] + "_extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPathWithinAny(tt.path, bases)
			if result != tt.expected {
				t.Errorf("isPathWithinAny(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestPathValidator_ValidateOpenInPath(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create test directories
	safeDir := filepath.Join(tmpDir, "safe")
	nestedDir := filepath.Join(safeDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create a file (should be rejected - must be directory)
	testFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a symlink (should be checked)
	symlinkDir := filepath.Join(tmpDir, "link")
	if err := os.Symlink(safeDir, symlinkDir); err != nil {
		// Skip symlink tests on Windows if not supported
		if runtime.GOOS != "windows" {
			t.Fatalf("Failed to create symlink: %v", err)
		}
	}

	// Create a registry with a project in our temp dir
	registry := thinkt.NewRegistry()

	validator := NewPathValidator(registry)
	
	// Manually add the temp directory to allowed bases for testing
	// This is needed because t.TempDir() is typically outside home on macOS (/var/folders/...)
	validator.additionalBases = []string{tmpDir}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid directories
		{"safe directory", safeDir, false},
		{"nested directory", nestedDir, false},

		// Non-existent paths
		{"non-existent", filepath.Join(tmpDir, "does-not-exist"), true},

		// Files (not directories)
		{"file instead of dir", testFile, true},

		// Paths with shell metacharacters
		{"path with semicolon", filepath.Join(tmpDir, "safe;rm -rf /"), true},
		{"path with pipe", filepath.Join(tmpDir, "safe|cat /etc/passwd"), true},

		// Symlinks (valid if they point to allowed locations)
		{"symlink to safe", symlinkDir, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip symlink test on Windows if symlink wasn't created
			if tt.name == "symlink to safe" && runtime.GOOS == "windows" {
				t.Skip("Symlinks not supported on Windows")
			}

			_, err := validator.ValidateOpenInPath(tt.path)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateOpenInPath(%q) expected error, got nil", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateOpenInPath(%q) unexpected error: %v", tt.path, err)
			}
		})
	}
}

func TestPathValidator_getAllowedBaseDirectories(t *testing.T) {
	registry := thinkt.NewRegistry()
	validator := NewPathValidator(registry)

	bases, err := validator.getAllowedBaseDirectories()
	if err != nil {
		t.Fatalf("getAllowedBaseDirectories() unexpected error: %v", err)
	}

	// Should always include home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	foundHome := false
	for _, base := range bases {
		if base == homeDir {
			foundHome = true
			break
		}
	}

	if !foundHome {
		t.Errorf("getAllowedBaseDirectories() should include home directory %q, got %v", homeDir, bases)
	}
}

func TestSanitizePathForLogging(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"short path", "/home/user/project", "/home/user/project"},
		{"exactly 100 chars", string(make([]byte, 100)), string(make([]byte, 100))},
		{"long path", "/home/user/" + string(make([]byte, 200)), "/home/user/" + string(make([]byte, 50)) + "..." + string(make([]byte, 50))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the long path test, we need to adjust expected value
			expected := tt.expected
			if tt.name == "long path" {
				expected = tt.path[:50] + "..." + tt.path[len(tt.path)-50:]
			}

			result := SanitizePathForLogging(tt.path)
			if result != expected {
				t.Errorf("SanitizePathForLogging() length = %d, expected %d", len(result), len(expected))
			}
		})
	}
}
