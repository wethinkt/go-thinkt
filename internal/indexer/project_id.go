package indexer

import "github.com/wethinkt/go-thinkt/internal/thinkt"

const projectIDScopeSeparator = "::"

// ProjectIDScopeSeparator returns the separator used in scoped project IDs.
func ProjectIDScopeSeparator() string {
	return projectIDScopeSeparator
}

// ScopedProjectID builds a source-scoped project ID for index rows.
// This avoids collisions when multiple sources reuse the same raw project ID.
func ScopedProjectID(source thinkt.Source, projectID string) string {
	if source == "" {
		return projectID
	}
	return string(source) + projectIDScopeSeparator + projectID
}

// ScopedProjectIDCandidates returns both legacy and scoped project IDs.
// It is used by read paths while old rows may still exist in the index.
func ScopedProjectIDCandidates(source thinkt.Source, projectID string) []string {
	scoped := ScopedProjectID(source, projectID)
	if scoped == projectID {
		return []string{projectID}
	}
	return []string{projectID, scoped}
}
