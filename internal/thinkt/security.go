package thinkt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathValidator provides secure path validation for file operations.
type PathValidator struct {
	registry        *StoreRegistry
	AdditionalBases []string // For testing - additional allowed base directories
}

// NewPathValidator creates a new path validator with access to project information.
func NewPathValidator(registry *StoreRegistry) *PathValidator {
	return &PathValidator{registry: registry}
}

// ValidateOpenInPath validates a path for the open-in feature.
// It ensures the path:
//   - Exists on the filesystem
//   - Is a directory (not a file)
//   - Is within an allowed location (user's home or known project)
//   - Does not contain shell metacharacters
//   - Is not a symlink to outside allowed locations
//
// Returns the cleaned, absolute path if valid, or an error if invalid.
func (v *PathValidator) ValidateOpenInPath(path string) (string, error) {
	// Check for shell metacharacters first (before any file operations)
	if err := ValidateNoShellMetacharacters(path); err != nil {
		return "", err
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("cannot access path: %w", err)
	}

	// Must be a directory
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Resolve symlinks to prevent symlink attacks
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}

	// Get allowed base directories
	allowedBases, err := v.GetAllowedBaseDirectories()
	if err != nil {
		return "", fmt.Errorf("cannot determine allowed directories: %w", err)
	}

	// Check if path is within allowed directories
	if !IsPathWithinAny(realPath, allowedBases) {
		return "", fmt.Errorf("path is outside allowed directories: %s", realPath)
	}

	return realPath, nil
}

// GetAllowedBaseDirectories returns the list of directories that are allowed
// for the open-in feature. This includes:
//   - The user's home directory
//   - All known project directories from the registry
//   - Any additional bases set for testing
//
// All paths are resolved to their real (symlink-free) paths.
func (v *PathValidator) GetAllowedBaseDirectories() ([]string, error) {
	var bases []string

	// Helper to add a base directory, resolving symlinks
	addBase := func(path string) {
		if path == "" {
			return
		}
		// Resolve symlinks to get the real path
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			// If we can't resolve, use the original path as fallback
			realPath = path
		}
		bases = append(bases, realPath)
	}

	// Add additional bases first (for testing)
	for _, base := range v.AdditionalBases {
		addBase(base)
	}

	// Add home directory
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		addBase(homeDir)
	}

	// Add all project directories from the registry
	if v.registry != nil {
		projects, err := v.registry.ListAllProjects(context.TODO())
		if err == nil {
			for _, p := range projects {
				addBase(p.Path)
			}
		}
	}

	return bases, nil
}

// ValidateNoShellMetacharacters checks if the path contains any shell
// metacharacters that could lead to command injection.
func ValidateNoShellMetacharacters(path string) error {
	// List of dangerous shell metacharacters
	dangerous := []string{
		";",  // Command separator
		"|",  // Pipe
		"&",  // Background/AND
		"$",  // Variable expansion
		"`",  // Command substitution
		"(",  // Subshell start
		")",  // Subshell end
		"{",  // Brace expansion start
		"}",  // Brace expansion end
		"<",  // Input redirection
		">",  // Output redirection
		"\"", // Quote
		"'",  // Single quote
		"\\", // Escape
		"\n", // Newline (command separator)
		"\r", // Carriage return
		"\t", // Tab
		"*",  // Glob (wildcard)
		"?",  // Single char wildcard
		"[",  // Character class start
		"]",  // Character class end
		"#",  // Comment
		"!",  // History expansion (bash)
		"~",  // Tilde expansion (home directory)
	}

	// Check for null bytes (can be used to bypass filters)
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	// Check each dangerous character
	for _, char := range dangerous {
		if strings.Contains(path, char) {
			return fmt.Errorf("path contains invalid character: %q", char)
		}
	}

	// Additional check: ensure path doesn't start with '-' (could be interpreted as flag)
	if strings.HasPrefix(strings.TrimSpace(path), "-") {
		return fmt.Errorf("path cannot start with '-'")
	}

	return nil
}

// IsPathWithinAny checks if the given path is within any of the allowed base directories.
// Both paths are cleaned and compared. The path must be equal to or a subdirectory of a base.
func IsPathWithinAny(path string, bases []string) bool {
	// Clean the path (remove .., ., etc.)
	cleanPath := filepath.Clean(path)

	for _, base := range bases {
		cleanBase := filepath.Clean(base)

		// Check if path is exactly the base or is a subdirectory
		if cleanPath == cleanBase {
			return true
		}

		// Add trailing separator to ensure proper prefix matching
		// (prevents /foo/bar matching /foo/barbaz)
		if !strings.HasSuffix(cleanBase, string(filepath.Separator)) {
			cleanBase += string(filepath.Separator)
		}

		if strings.HasPrefix(cleanPath+string(filepath.Separator), cleanBase) {
			return true
		}
	}

	return false
}