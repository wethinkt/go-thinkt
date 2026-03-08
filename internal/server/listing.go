package server

import (
	"context"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
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

// listProjects tries the indexer RPC first for richer metadata, falling back
// to the StoreRegistry filesystem path. Callers get consistent filtering,
// sorting, and pagination regardless of the data source.
func listProjects(ctx context.Context, registry *thinkt.StoreRegistry, source string, includeDeleted bool, limit, offset int) (*projectResult, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Try indexer RPC first.
	if data, err := indexerListProjects(rpc.ListProjectsParams{
		Source: source,
		Limit:  limit,
		Offset: offset,
	}); err == nil {
		projects := make([]thinkt.Project, 0, len(data.Projects))
		for _, p := range data.Projects {
			projects = append(projects, thinkt.Project{
				ID:           p.ID,
				Name:         p.Name,
				Path:         p.Path,
				Source:       thinkt.Source(p.Source),
				SessionCount: p.SessionCount,
				PathExists:   true,
			})
		}
		return &projectResult{Projects: projects, Total: data.Total, Returned: data.Returned}, nil
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

// sessionResult is the unified return type for listSessions.
type sessionResult struct {
	Sessions []thinkt.SessionMeta
	Total    int
	Returned int
}

// listSessions tries the indexer RPC first for richer metadata (accurate
// entry_count, model), falling back to the StoreRegistry filesystem path.
func listSessions(ctx context.Context, registry *thinkt.StoreRegistry, source thinkt.Source, projectID string, limit, offset int) (*sessionResult, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Try indexer RPC first.
	if data, err := indexerListSessions(rpc.ListSessionsParams{
		ProjectID: projectID,
		Source:    string(source),
		Limit:    limit,
		Offset:   offset,
	}); err == nil && data.Total > 0 {
		sessions := make([]thinkt.SessionMeta, 0, len(data.Sessions))
		for _, s := range data.Sessions {
			sm := thinkt.SessionMeta{
				ID:       s.ID,
				FullPath: s.Path,
				Model:    s.Model,
				EntryCount: s.EntryCount,
				Source:   source,
			}
			if t, err := parseTimeRFC3339(s.CreatedAt); err == nil {
				sm.CreatedAt = t
			}
			if t, err := parseTimeRFC3339(s.UpdatedAt); err == nil {
				sm.ModifiedAt = t
			}
			sessions = append(sessions, sm)
		}
		return &sessionResult{Sessions: sessions, Total: data.Total, Returned: data.Returned}, nil
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

// parseTimeRFC3339 parses an RFC3339 string, returning zero time on error.
func parseTimeRFC3339(s string) (t time.Time, err error) {
	if s == "" {
		return
	}
	return time.Parse(time.RFC3339, s)
}
