// Package server provides security utilities for the HTTP server.
package server

import (
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// PathValidator provides secure path validation for file operations.
// Deprecated: Use thinkt.PathValidator instead.
type PathValidator = thinkt.PathValidator

// NewPathValidator creates a new path validator with access to project information.
func NewPathValidator(registry *thinkt.StoreRegistry) *PathValidator {
	return thinkt.NewPathValidator(registry)
}

// SanitizePathForLogging returns a sanitized version of the path safe for logging.
// It truncates long paths and removes potentially sensitive information.
func SanitizePathForLogging(path string) string {
	if len(path) > 100 {
		return path[:50] + "..." + path[len(path)-50:]
	}
	return path
}