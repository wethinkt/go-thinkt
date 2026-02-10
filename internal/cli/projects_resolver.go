package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveProject resolves a user-provided project query to a known project.
// Query can be project ID, absolute path, relative path, or path suffix.
func ResolveProject(registry *thinkt.StoreRegistry, query string) (*thinkt.Project, error) {
	if registry == nil {
		return nil, fmt.Errorf("store registry is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("project query is required")
	}

	projects, err := registry.ListAllProjects(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	pathQuery := query
	if !filepath.IsAbs(pathQuery) {
		if absQuery, err := filepath.Abs(pathQuery); err == nil {
			pathQuery = absQuery
		}
	}

	// First pass: exact ID or exact canonical path.
	exact := matchProjects(projects, func(p thinkt.Project) bool {
		if p.ID == query {
			return true
		}
		return samePath(p.Path, pathQuery)
	})
	if len(exact) > 0 {
		return projectMatchResult(exact, query)
	}

	// Second pass: suffix matches for ergonomic queries.
	suffix := matchProjects(projects, func(p thinkt.Project) bool {
		return pathHasSuffix(p.Path, query)
	})
	if len(suffix) > 0 {
		return projectMatchResult(suffix, query)
	}

	if looksLikePathQuery(query) {
		if info, err := os.Stat(pathQuery); err == nil && info.IsDir() {
			return nil, fmt.Errorf("no sessions found in %s", pathQuery)
		}
	}
	return nil, fmt.Errorf("project not found: %s", query)
}

func projectMatchResult(matches []thinkt.Project, query string) (*thinkt.Project, error) {
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("project not found: %s", query)
	case 1:
		match := matches[0]
		return &match, nil
	default:
		var b strings.Builder
		b.WriteString("project query is ambiguous, matched multiple projects:\n")
		max := len(matches)
		if max > 5 {
			max = 5
		}
		for i := 0; i < max; i++ {
			b.WriteString("  - ")
			b.WriteString(matches[i].Path)
			b.WriteByte('\n')
		}
		if len(matches) > max {
			b.WriteString(fmt.Sprintf("  ... and %d more", len(matches)-max))
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(b.String()))
	}
}

func matchProjects(projects []thinkt.Project, matchFn func(thinkt.Project) bool) []thinkt.Project {
	matches := make([]thinkt.Project, 0, 2)
	for _, p := range projects {
		if matchFn(p) {
			matches = append(matches, p)
		}
	}
	return matches
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func pathHasSuffix(path, suffix string) bool {
	if path == "" || suffix == "" {
		return false
	}

	pathNorm := filepath.ToSlash(filepath.Clean(path))
	suffixNorm := filepath.ToSlash(filepath.Clean(suffix))

	if pathNorm == suffixNorm {
		return true
	}

	if strings.HasSuffix(pathNorm, suffixNorm) {
		prefixLen := len(pathNorm) - len(suffixNorm)
		return prefixLen == 0 || pathNorm[prefixLen-1] == '/'
	}

	if strings.Contains(suffixNorm, "/") {
		return false
	}

	baseSuffix := filepath.Base(suffixNorm)
	return filepath.Base(pathNorm) == baseSuffix
}

func looksLikePathQuery(query string) bool {
	return filepath.IsAbs(query) ||
		strings.HasPrefix(query, ".") ||
		strings.ContainsRune(query, '/') ||
		strings.ContainsRune(query, '\\')
}
