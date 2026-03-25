package server

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestGenerateSecureToken(t *testing.T) {
	token, err := GenerateSecureToken()
	if err != nil {
		t.Fatalf("GenerateSecureToken() error = %v", err)
	}

	// Token should be 64 characters (32 bytes hex encoded)
	if len(token) != 64 {
		t.Errorf("GenerateSecureToken() length = %d, want 64", len(token))
	}

	// Token should be different each time
	token2, err := GenerateSecureToken()
	if err != nil {
		t.Fatalf("GenerateSecureToken() second call error = %v", err)
	}
	if token == token2 {
		t.Error("GenerateSecureToken() should generate unique tokens")
	}
}

func TestGenerateSecureTokenWithPrefix(t *testing.T) {
	token, err := GenerateSecureTokenWithPrefix()
	if err != nil {
		t.Fatalf("GenerateSecureTokenWithPrefix() error = %v", err)
	}

	// Token should start with "thinkt_"
	if len(token) < 8 || token[:7] != "thinkt_" {
		t.Errorf("GenerateSecureTokenWithPrefix() should start with 'thinkt_', got %s", token)
	}

	// Token should contain date
	if len(token) < 16 {
		t.Errorf("GenerateSecureTokenWithPrefix() too short: %s", token)
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
			result := thinkt.IsPathWithinAny(tt.path, bases)
			if result != tt.expected {
				t.Errorf("IsPathWithinAny(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsPathWithinAny_WindowsPathForms(t *testing.T) {
	bases := []string{
		`C:\Users\alice`,
		`\\server\share\projects`,
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"drive exact backslashes", `C:\Users\alice`, true},
		{"drive exact forward slashes", `C:/Users/alice`, true},
		{"drive nested mixed slashes", `C:\Users/alice\repo`, true},
		{"drive case insensitive", `c:\users\ALICE\repo`, true},
		{"drive prefix should not match", `C:\Users\aliceevil`, false},
		{"different drive", `D:\Users\alice`, false},
		{"drive path traversal cleaned", `C:\Users\alice\repo\..\work`, true},
		{"unc nested backslashes", `\\server\share\projects\team-a`, true},
		{"unc nested forward slashes", `//server/share/projects/team-b`, true},
		{"unc different share", `\\server\other\projects`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := thinkt.IsPathWithinAny(tt.path, bases)
			if result != tt.expected {
				t.Errorf("IsPathWithinAny(%q) = %v, want %v", tt.path, result, tt.expected)
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
	otherDir := filepath.Join(tmpDir, "other")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}
	if err := os.MkdirAll(otherDir, 0755); err != nil {
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

	validator := thinkt.NewPathValidator(registry)

	// Explicitly allow only the safe directory hierarchy.
	validator.AdditionalBases = []string{safeDir}

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

		// Existing directory outside the allowlist
		{"outside explicit allowlist", otherDir, true},

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

func TestPathValidator_GetAllowedBaseDirectories_ProjectRootsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}
	deletedProjectDir := filepath.Join(tmpDir, "deleted-project")

	registry := thinkt.NewRegistry()
	registry.Register(&fakeProjectStore{
		source: thinkt.Source("fake"),
		projects: []thinkt.Project{
			{ID: "project", Path: projectDir, Source: thinkt.Source("fake")},
			{ID: "deleted", Path: deletedProjectDir, Source: thinkt.Source("fake")},
		},
	})

	validator := thinkt.NewPathValidator(registry)

	bases, err := validator.GetAllowedBaseDirectories()
	if err != nil {
		t.Fatalf("GetAllowedBaseDirectories() unexpected error: %v", err)
	}

	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		resolvedProjectDir = projectDir
	}

	if len(bases) != 1 || bases[0] != resolvedProjectDir {
		t.Fatalf("GetAllowedBaseDirectories() = %v, want [%q]", bases, resolvedProjectDir)
	}
}

func TestPathValidator_ValidateOpenInPath_RejectsNonProjectDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	otherDir := filepath.Join(tmpDir, "other")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatalf("Failed to create non-project directory: %v", err)
	}

	registry := thinkt.NewRegistry()
	registry.Register(&fakeProjectStore{
		source: thinkt.Source("fake"),
		projects: []thinkt.Project{
			{ID: "project", Path: projectDir, Source: thinkt.Source("fake")},
		},
	})

	validator := thinkt.NewPathValidator(registry)

	if _, err := validator.ValidateOpenInPath(projectDir); err != nil {
		t.Fatalf("ValidateOpenInPath(projectDir) unexpected error: %v", err)
	}

	if _, err := validator.ValidateOpenInPath(otherDir); err == nil {
		t.Fatal("ValidateOpenInPath(otherDir) expected error, got nil")
	}
}

type fakeProjectStore struct {
	source   thinkt.Source
	projects []thinkt.Project
}

func (f *fakeProjectStore) Source() thinkt.Source       { return f.source }
func (f *fakeProjectStore) Workspace() thinkt.Workspace { return thinkt.Workspace{} }
func (f *fakeProjectStore) ListProjects(context.Context) ([]thinkt.Project, error) {
	return append([]thinkt.Project(nil), f.projects...), nil
}
func (f *fakeProjectStore) GetProject(_ context.Context, id string) (*thinkt.Project, error) {
	for _, p := range f.projects {
		if p.ID == id || p.Path == id {
			cp := p
			return &cp, nil
		}
	}
	return nil, nil
}
func (f *fakeProjectStore) ListSessions(context.Context, string, ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	return nil, nil
}
func (f *fakeProjectStore) GetSessionMeta(context.Context, string) (*thinkt.SessionMeta, error) {
	return nil, nil
}
func (f *fakeProjectStore) LoadSession(context.Context, string) (*thinkt.Session, error) {
	return nil, nil
}
func (f *fakeProjectStore) OpenSession(context.Context, string) (thinkt.SessionReader, error) {
	return nil, nil
}
func (f *fakeProjectStore) WatchConfig() thinkt.WatchConfig { return thinkt.DefaultWatchConfig() }
