package cli

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

type testProjectStore struct {
	source            thinkt.Source
	workspace         thinkt.Workspace
	projects          []thinkt.Project
	sessionsByProject map[string][]thinkt.SessionMeta
}

func (s *testProjectStore) Source() thinkt.Source {
	return s.source
}

func (s *testProjectStore) Workspace() thinkt.Workspace {
	if s.workspace.Source == "" {
		s.workspace.Source = s.source
	}
	if s.workspace.ID == "" {
		s.workspace.ID = "test-workspace"
	}
	return s.workspace
}

func (s *testProjectStore) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.projects, nil
}

func (s *testProjectStore) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	for _, p := range s.projects {
		if p.ID == id || p.Path == id {
			project := p
			return &project, nil
		}
	}
	return nil, nil
}

func (s *testProjectStore) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	return s.sessionsByProject[projectID], nil
}

func (s *testProjectStore) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	for _, sessions := range s.sessionsByProject {
		for _, meta := range sessions {
			if meta.ID == sessionID || meta.FullPath == sessionID {
				match := meta
				return &match, nil
			}
		}
	}
	return nil, nil
}

func (s *testProjectStore) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	return nil, nil
}

func (s *testProjectStore) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	return &projectNoopSessionReader{}, nil
}

type projectNoopSessionReader struct{}

func (r *projectNoopSessionReader) ReadNext() (*thinkt.Entry, error) { return nil, io.EOF }
func (r *projectNoopSessionReader) Metadata() thinkt.SessionMeta     { return thinkt.SessionMeta{} }
func (r *projectNoopSessionReader) Close() error                     { return nil }

func makeSingleProjectRegistry(source thinkt.Source, projectID, projectPath string, sessionPaths []string) *thinkt.StoreRegistry {
	sessions := make([]thinkt.SessionMeta, 0, len(sessionPaths))
	for _, p := range sessionPaths {
		name := filepath.Base(p)
		id := strings.TrimSuffix(name, filepath.Ext(name))
		sessions = append(sessions, thinkt.SessionMeta{
			ID:          id,
			FullPath:    p,
			Source:      source,
			ProjectPath: projectPath,
		})
	}

	store := &testProjectStore{
		source: source,
		projects: []thinkt.Project{
			{
				ID:     projectID,
				Name:   filepath.Base(projectPath),
				Path:   projectPath,
				Source: source,
			},
		},
		sessionsByProject: map[string][]thinkt.SessionMeta{
			projectID: sessions,
		},
	}

	registry := thinkt.NewRegistry()
	registry.Register(store)
	return registry
}
