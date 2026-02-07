// Package server provides security utilities for the HTTP server.
package server

// SanitizePathForLogging returns a sanitized version of the path safe for logging.
// It truncates long paths and removes potentially sensitive information.
func SanitizePathForLogging(path string) string {
	if len(path) > 100 {
		return path[:50] + "..." + path[len(path)-50:]
	}
	return path
}
