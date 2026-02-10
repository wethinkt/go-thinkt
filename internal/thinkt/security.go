package thinkt

import (
	"context"
	"fmt"
	"os"
	pathpkg "path"
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
//   - Is not a symlink to outside allowed locations
//
// Returns the cleaned, absolute path if valid, or an error if invalid.
func (v *PathValidator) ValidateOpenInPath(path string) (string, error) {
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

// IsPathWithinAny checks if the given path is within any of the allowed base directories.
// Both paths are canonicalized (including slash and Windows volume normalization).
// The path must be equal to or a subdirectory of a base.
func IsPathWithinAny(path string, bases []string) bool {
	canonicalPath, isWindowsPath := canonicalPathForMatch(path)
	if canonicalPath == "" {
		return false
	}

	for _, base := range bases {
		canonicalBase, isWindowsBase := canonicalPathForMatch(base)
		if canonicalBase == "" {
			continue
		}
		if (isWindowsPath || isWindowsBase) && isWindowsPath != isWindowsBase {
			continue
		}

		// Check if path is exactly the base or is a subdirectory
		if canonicalPath == canonicalBase {
			return true
		}

		// Add trailing separator to ensure proper prefix matching
		// (prevents /foo/bar matching /foo/barbaz and C:/foo matching C:/foobar)
		if !strings.HasSuffix(canonicalBase, "/") {
			canonicalBase += "/"
		}

		if strings.HasPrefix(canonicalPath+"/", canonicalBase) {
			return true
		}
	}

	return false
}

func canonicalPathForMatch(input string) (canonical string, isWindowsStyle bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	volume, rest, isWindows := splitPathVolume(normalized)

	restClean := pathpkg.Clean(rest)
	if strings.HasPrefix(rest, "/") && !strings.HasPrefix(restClean, "/") {
		restClean = "/" + restClean
	}
	if restClean == "." {
		if strings.HasPrefix(rest, "/") {
			restClean = "/"
		} else {
			restClean = ""
		}
	}

	if isWindows {
		return strings.ToLower(volume + restClean), true
	}
	return pathpkg.Clean(normalized), false
}

func splitPathVolume(input string) (volume string, rest string, isWindows bool) {
	if len(input) >= 2 && isASCIIAlpha(input[0]) && input[1] == ':' {
		return strings.ToUpper(input[:2]), input[2:], true
	}

	if strings.HasPrefix(input, "//") {
		withoutPrefix := strings.TrimPrefix(input, "//")
		parts := strings.SplitN(withoutPrefix, "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			vol := "//" + strings.ToUpper(parts[0]) + "/" + strings.ToUpper(parts[1])
			if len(parts) == 3 {
				return vol, "/" + parts[2], true
			}
			return vol, "/", true
		}
	}

	return "", input, false
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
