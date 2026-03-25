package indexer

import (
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ProjectIDScopeSeparator delegates to the shared implementation in internal/index/db.
func ProjectIDScopeSeparator() string {
	return indexdb.ProjectIDScopeSeparator()
}

// ScopedProjectID delegates to the shared implementation in internal/index/db.
func ScopedProjectID(source thinkt.Source, projectID string) string {
	return indexdb.ScopedProjectID(source, projectID)
}

// ScopedProjectIDCandidates delegates to the shared implementation in internal/index/db.
func ScopedProjectIDCandidates(source thinkt.Source, projectID string) []string {
	return indexdb.ScopedProjectIDCandidates(source, projectID)
}
