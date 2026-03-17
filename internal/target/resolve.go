package target

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// Flags holds the CLI flags for session targeting.
type Flags struct {
	Project       string
	Session       string
	Sources       []string
	HeaderContext string // e.g. "export" — shown in picker header bar
}

// Result holds the resolved session and its loaded content.
type Result struct {
	SessionPath string
	ProjectName string
	Meta        thinkt.SessionMeta
	Entries     []thinkt.Entry
}

// IsTTY returns true if stdin is a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ResolveSession resolves a session using flags, cwd detection, and TUI pickers.
func ResolveSession(registry *thinkt.StoreRegistry, flags Flags) (*Result, error) {
	ctx := context.Background()

	// 1. Absolute path — use directly
	if flags.Session != "" && filepath.IsAbs(flags.Session) {
		return loadSession(flags.Session, "", registry)
	}

	// Try to resolve project
	projectID, projectName, err := resolveProject(registry, flags, ctx, flags.HeaderContext)
	if err != nil {
		return nil, err
	}

	// Get sessions for the project
	sessions, err := getSessionsForProject(registry, projectID, flags.Sources)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found in project %s", projectName)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})

	// 2. --session flag specified — match by ID/suffix
	if flags.Session != "" {
		for _, s := range sessions {
			if s.ID == flags.Session ||
				strings.HasSuffix(s.FullPath, flags.Session) ||
				strings.HasSuffix(s.FullPath, flags.Session+".jsonl") {
				return loadSession(s.FullPath, projectName, registry)
			}
		}
		absPath, err := filepath.Abs(flags.Session)
		if err == nil {
			if _, statErr := os.Stat(absPath); statErr == nil {
				return loadSession(absPath, projectName, registry)
			}
		}
		return nil, fmt.Errorf("session not found: %s", flags.Session)
	}

	// No --session flag — need a picker
	if !IsTTY() {
		return nil, fmt.Errorf("--project and --session are required when no TTY is available")
	}

	// Build breadcrumb for the session picker header: "export > projectName"
	sessionHeader := flags.HeaderContext
	if sessionHeader != "" && projectName != "" {
		sessionHeader += " > " + projectName
	}
	selected, err := tui.PickSessionWith(sessions, tui.SessionPickerOpts{
		HeaderContext: sessionHeader,
		DisableResume: flags.HeaderContext != "",
	})
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, fmt.Errorf("cancelled")
	}
	return loadSession(selected.FullPath, projectName, registry)
}

func resolveProject(registry *thinkt.StoreRegistry, flags Flags, ctx context.Context, headerContext string) (string, string, error) {
	if flags.Project != "" {
		projects, _ := getProjectsFromSources(registry, flags.Sources)
		for _, p := range projects {
			if p.ID == flags.Project || p.Path == flags.Project {
				return p.ID, p.Name, nil
			}
		}
		return flags.Project, flags.Project, nil
	}

	cwd, err := os.Getwd()
	if err == nil {
		if project := registry.FindProjectForPath(ctx, cwd); project != nil {
			return project.ID, project.Name, nil
		}
	}

	if !IsTTY() {
		return "", "", fmt.Errorf("--project and --session are required when no TTY is available")
	}

	projects, err := getProjectsFromSources(registry, flags.Sources)
	if err != nil {
		return "", "", err
	}
	if len(projects) == 0 {
		return "", "", fmt.Errorf("no projects found")
	}

	selected, err := tui.PickProject(projects, headerContext)
	if err != nil {
		return "", "", err
	}
	if selected == nil {
		return "", "", fmt.Errorf("cancelled")
	}
	return selected.ID, selected.Name, nil
}

func loadSession(path, projectName string, registry *thinkt.StoreRegistry) (*Result, error) {
	ls, err := tui.OpenLazySessionWithRegistry(path, registry)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	if err := ls.LoadAll(); err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	return &Result{
		SessionPath: path,
		ProjectName: projectName,
		Meta:        ls.Metadata(),
		Entries:     ls.Entries(),
	}, nil
}

// ProjectResolution holds the result of non-interactive project resolution.
type ProjectResolution struct {
	// Resolved is true if a project was found without needing a picker.
	Resolved    bool
	ProjectID   string
	ProjectName string
	// Projects is non-nil when Resolved is false — the picker should show these.
	Projects []thinkt.Project
}

// ResolveProjectNonInteractive attempts to resolve a project without any TUI.
// If it can't (no CWD match, no --project flag), it returns the list of projects
// for the caller to present in a picker.
func ResolveProjectNonInteractive(registry *thinkt.StoreRegistry, flags Flags) (*ProjectResolution, error) {
	ctx := context.Background()

	if flags.Project != "" {
		projects, _ := getProjectsFromSources(registry, flags.Sources)
		for _, p := range projects {
			if p.ID == flags.Project || p.Path == flags.Project {
				return &ProjectResolution{Resolved: true, ProjectID: p.ID, ProjectName: p.Name}, nil
			}
		}
		return &ProjectResolution{Resolved: true, ProjectID: flags.Project, ProjectName: flags.Project}, nil
	}

	cwd, err := os.Getwd()
	if err == nil {
		if project := registry.FindProjectForPath(ctx, cwd); project != nil {
			return &ProjectResolution{Resolved: true, ProjectID: project.ID, ProjectName: project.Name}, nil
		}
	}

	projects, err := getProjectsFromSources(registry, flags.Sources)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found")
	}
	return &ProjectResolution{Projects: projects}, nil
}

// GetSessionsForProject returns sessions for a project, sorted by most recent first.
func GetSessionsForProject(registry *thinkt.StoreRegistry, projectID string, sources []string) ([]thinkt.SessionMeta, error) {
	sessions, err := getSessionsForProject(registry, projectID, sources)
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})
	return sessions, nil
}

// LoadSession loads a session's entries and metadata from a file path.
func LoadSession(path, projectName string, registry *thinkt.StoreRegistry) (*Result, error) {
	return loadSession(path, projectName, registry)
}

// ResolveSessionByID tries to match a session ID/suffix against the given sessions.
// Returns nil if no match found.
func ResolveSessionByID(sessions []thinkt.SessionMeta, sessionFlag string) *thinkt.SessionMeta {
	for i, s := range sessions {
		if s.ID == sessionFlag ||
			strings.HasSuffix(s.FullPath, sessionFlag) ||
			strings.HasSuffix(s.FullPath, sessionFlag+".jsonl") {
			return &sessions[i]
		}
	}
	return nil
}

// getProjectsFromSources returns projects from the selected sources.
// If no sources specified, returns projects from all available sources.
// Copied from internal/cmd/registry.go to avoid circular dependency.
func getProjectsFromSources(registry *thinkt.StoreRegistry, sources []string) ([]thinkt.Project, error) {
	ctx := context.Background()

	// If no sources specified, use all available sources
	if len(sources) == 0 {
		return registry.ListAllProjects(ctx)
	}

	// Validate and collect projects from specified sources
	var allProjects []thinkt.Project
	for _, sourceName := range sources {
		source := thinkt.Source(sourceName)
		store, ok := registry.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: claude, kimi, gemini, copilot, codex, qwen)", sourceName)
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects from %s: %w", sourceName, err)
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// getSessionsForProject returns sessions for a project from the selected sources.
// If no sources specified, searches all available sources.
// The projectID can be a store-specific ID or a filesystem path; both are tried.
// Copied from internal/cmd/registry.go to avoid circular dependency.
func getSessionsForProject(registry *thinkt.StoreRegistry, projectID string, sources []string) ([]thinkt.SessionMeta, error) {
	ctx := context.Background()

	// Determine which stores to search
	var stores []thinkt.Store
	if len(sources) == 0 {
		stores = registry.All()
	} else {
		for _, sourceName := range sources {
			source := thinkt.Source(sourceName)
			store, ok := registry.Get(source)
			if !ok {
				return nil, fmt.Errorf("unknown source: %s (available: claude, kimi, gemini, copilot, codex, qwen)", sourceName)
			}
			stores = append(stores, store)
		}
	}

	// Resolve projectID to a filesystem path by checking all stores.
	// projectID might be a store-specific ID (e.g., ~/.claude/projects/...)
	// or a filesystem path (e.g., /Users/evan/myproject).
	resolvedPath := projectID
	for _, store := range stores {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, p := range projects {
			if p.ID == projectID {
				resolvedPath = p.Path
				break
			}
		}
		if resolvedPath != projectID {
			break
		}
	}

	// Collect sessions from all stores that have a project matching the resolved path.
	var allSessions []thinkt.SessionMeta
	for _, store := range stores {
		// Try the projectID directly first
		sessions, err := store.ListSessions(ctx, projectID)
		if err == nil && len(sessions) > 0 {
			allSessions = append(allSessions, sessions...)
			continue
		}

		// Find this store's project by filesystem path and use its ID
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, p := range projects {
			if p.Path == resolvedPath {
				sessions, err := store.ListSessions(ctx, p.ID)
				if err == nil && len(sessions) > 0 {
					allSessions = append(allSessions, sessions...)
				}
				break
			}
		}
	}

	return allSessions, nil
}
