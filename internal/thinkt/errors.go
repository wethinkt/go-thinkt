package thinkt

import "fmt"

// Sentinel errors for domain concepts that cross the Store -> API boundary.
var (
	// ErrSessionNotFound indicates that a session ID or path does not resolve.
	ErrSessionNotFound = fmt.Errorf("session not found")

	// ErrProjectNotFound indicates that a project ID does not resolve.
	ErrProjectNotFound = fmt.Errorf("project not found")

	// ErrSourceUnavailable indicates that the requested source is not configured or available.
	ErrSourceUnavailable = fmt.Errorf("source unavailable")

	// ErrResumeNotSupported indicates that the source does not support session resume.
	ErrResumeNotSupported = fmt.Errorf("resume not supported")
)

// ValidationError represents an input validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}
