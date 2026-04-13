package server

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// defaultListLimit is the default number of results returned by listing endpoints.
const defaultListLimit = 20

// projectResult is the unified return type for listProjects.
type projectResult struct {
	Projects []thinkt.Project
	Total    int
	Returned int
}

// listProjects tries SQLite first, then indexer RPC, falling back
// to the StoreRegistry filesystem path. Callers get consistent filtering,
// sorting, and pagination regardless of the data source.
func listProjects(ctx context.Context, idb *indexdb.DB, registry *thinkt.StoreRegistry, source string, includeDeleted bool, limit, offset int) (*projectResult, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Derive enabled sources from registry for SQLite filtering.
	var enabledSources []string
	for _, src := range registry.Sources() {
		enabledSources = append(enabledSources, string(src))
	}

	// Try direct SQLite first.
	if idb != nil {
		if result, err := sqliteListProjects(idb, source, enabledSources, limit, offset); err == nil && result.Total > 0 {
			return result, nil
		}
	}

	// Fallback: filesystem via StoreRegistry with filtering/pagination.
	var opts []thinkt.ListProjectsOption
	if source != "" {
		opts = append(opts, thinkt.WithSources(source))
	}
	opts = append(opts, thinkt.WithIncludeDeleted(includeDeleted))
	// Fetch all matching to get total count, then paginate.
	all, err := registry.ListAllProjects(ctx, opts...)
	if err != nil {
		return nil, err
	}
	total := len(all)

	// Paginate.
	if offset >= total {
		return &projectResult{Projects: []thinkt.Project{}, Total: total, Returned: 0}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := all[offset:end]

	return &projectResult{Projects: page, Total: total, Returned: len(page)}, nil
}

// sqliteListProjects queries the SQLite index for project listing.
func sqliteListProjects(idb *indexdb.DB, source string, enabledSources []string, limit, offset int) (*projectResult, error) {
	// Build source filter.
	whereClause := " WHERE 1=1"
	var filterArgs []any
	if source != "" {
		whereClause += " AND p.source = ?"
		filterArgs = append(filterArgs, strings.ToLower(source))
	}
	srcClause, srcArgs := indexdb.SourceFilter(enabledSources, "p.source")
	whereClause += " " + srcClause
	filterArgs = append(filterArgs, srcArgs...)

	// Count query.
	var total int
	if err := idb.QueryRow("SELECT count(*) FROM projects p"+whereClause, filterArgs...).Scan(&total); err != nil {
		return nil, err
	}

	// Data query — sort by session count (projects table has no updated_at).
	dataSQL := `SELECT p.id, p.name, p.path, p.source,
		(SELECT count(*) FROM sessions s WHERE s.project_id = p.id) AS session_count
		FROM projects p` + whereClause + ` ORDER BY session_count DESC LIMIT ? OFFSET ?`
	dataArgs := append(filterArgs, limit, offset) //nolint: gocritic

	rows, err := idb.Query(dataSQL, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []thinkt.Project
	for rows.Next() {
		var p thinkt.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.Source, &p.SessionCount); err != nil {
			continue
		}
		p.PathExists = true
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []thinkt.Project{}
	}

	return &projectResult{Projects: projects, Total: total, Returned: len(projects)}, nil
}

// sessionResult is the unified return type for listSessions.
type sessionResult struct {
	Sessions []thinkt.SessionMeta
	Total    int
	Returned int
}

// listSessions tries SQLite first, then indexer RPC, falling back to the
// StoreRegistry filesystem path.
func listSessions(ctx context.Context, idb *indexdb.DB, registry *thinkt.StoreRegistry, source thinkt.Source, projectID string, limit, offset int) (*sessionResult, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Try direct SQLite first.
	if idb != nil {
		if result, err := sqliteListSessions(idb, source, projectID, limit, offset); err == nil && result.Total > 0 {
			return result, nil
		}
	}

	// Fallback: filesystem via store.
	store, ok := registry.Get(source)
	if !ok {
		return &sessionResult{Sessions: []thinkt.SessionMeta{}, Total: 0, Returned: 0}, nil
	}
	all, err := store.ListSessions(ctx, projectID, thinkt.WithEnrich(func(_ string, _ []thinkt.SessionMeta) {}))
	if err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool { return all[i].ModifiedAt.After(all[j].ModifiedAt) })
	total := len(all)

	if offset >= total {
		return &sessionResult{Sessions: []thinkt.SessionMeta{}, Total: total, Returned: 0}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := all[offset:end]

	return &sessionResult{Sessions: page, Total: total, Returned: len(page)}, nil
}

// sqliteListSessions queries the SQLite index for session listing.
func sqliteListSessions(idb *indexdb.DB, source thinkt.Source, projectID string, limit, offset int) (*sessionResult, error) {
	countSQL := `SELECT count(*) FROM sessions s
		JOIN projects p ON s.project_id = p.id
		WHERE p.id = ? AND p.source = ?`
	var total int
	if err := idb.QueryRow(countSQL, projectID, strings.ToLower(string(source))).Scan(&total); err != nil {
		return nil, err
	}

	dataSQL := `SELECT s.id, s.path, s.model, s.created_at, s.updated_at,
		(SELECT count(*) FROM entries e WHERE e.session_id = s.id) AS entry_count
		FROM sessions s
		JOIN projects p ON s.project_id = p.id
		WHERE p.id = ? AND p.source = ?
		ORDER BY s.updated_at DESC LIMIT ? OFFSET ?`

	rows, err := idb.Query(dataSQL, projectID, strings.ToLower(string(source)), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []thinkt.SessionMeta
	for rows.Next() {
		var sm thinkt.SessionMeta
		var createdAt, updatedAt sql.NullString
		var model sql.NullString
		if err := rows.Scan(&sm.ID, &sm.FullPath, &model, &createdAt, &updatedAt, &sm.EntryCount); err != nil {
			continue
		}
		sm.Source = source
		if model.Valid {
			sm.Model = model.String
		}
		if createdAt.Valid {
			if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
				sm.CreatedAt = t
			}
		}
		if updatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
				sm.ModifiedAt = t
			}
		}
		sessions = append(sessions, sm)
	}
	if sessions == nil {
		sessions = []thinkt.SessionMeta{}
	}

	return &sessionResult{Sessions: sessions, Total: total, Returned: len(sessions)}, nil
}

// parseTimeRFC3339 parses an RFC3339 string, returning zero time on error.
func parseTimeRFC3339(s string) (t time.Time, err error) {
	if s == "" {
		return
	}
	return time.Parse(time.RFC3339, s)
}
