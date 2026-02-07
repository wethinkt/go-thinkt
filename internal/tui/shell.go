package tui

import (
	"context"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// NavItem represents a page in the navigation stack
type NavItem struct {
	Title string
	Model tea.Model
}

// NavStack manages navigation history
type NavStack struct {
	items []NavItem
}

func NewNavStack() *NavStack {
	return &NavStack{items: make([]NavItem, 0)}
}

func (ns *NavStack) Push(item NavItem, width, height int) tea.Cmd {
	ns.items = append(ns.items, item)
	initCmd := item.Model.Init()
	// Send current window size to the new model so it can initialize its viewport
	if width > 0 && height > 0 {
		sizeCmd := func() tea.Msg {
			return tea.WindowSizeMsg{Width: width, Height: height}
		}
		return tea.Batch(initCmd, sizeCmd)
	}
	return initCmd
}

func (ns *NavStack) Pop() {
	if len(ns.items) > 0 {
		ns.items = ns.items[:len(ns.items)-1]
	}
}

func (ns *NavStack) Peek() (NavItem, bool) {
	if len(ns.items) == 0 {
		return NavItem{}, false
	}
	return ns.items[len(ns.items)-1], true
}

func (ns *NavStack) IsEmpty() bool {
	return len(ns.items) == 0
}

func (ns *NavStack) Path() []string {
	path := make([]string, len(ns.items))
	for i, item := range ns.items {
		path[i] = item.Title
	}
	return path
}

// Navigation messages
type PushPageMsg struct {
	Item NavItem
}

type PopPageMsg struct{}

// InitialPage defines which page to start on
type InitialPage int

const (
	InitialPageAuto InitialPage = iota // Auto-detect project from CWD
	InitialPageProjects                // Always start at projects list
)

// Shell is the main TUI container with navigation
type Shell struct {
	width       int
	height      int
	stack       *NavStack
	registry    *thinkt.StoreRegistry
	loading     bool
	initialPage InitialPage
}

// NewShell creates the main TUI shell
func NewShell(initial InitialPage) *Shell {
	return &Shell{
		stack:       NewNavStack(),
		registry:    thinkt.NewRegistry(),
		loading:     true,
		initialPage: initial,
	}
}

// NewShellWithSessions creates a Shell that starts with a pre-loaded session picker.
// Back navigation from the viewer returns to the picker via PopPageMsg.
// Cancelling the picker exits the program.
func NewShellWithSessions(sessions []thinkt.SessionMeta) *Shell {
	s := &Shell{
		stack:   NewNavStack(),
		loading: false,
	}
	picker := NewSessionPickerModel(sessions, nil)
	s.stack.items = append(s.stack.items, NavItem{
		Title: "Sessions",
		Model: picker,
	})
	return s
}

func (s *Shell) Init() tea.Cmd {
	tuilog.Log.Info("Shell.Init: starting")
	if s.registry == nil {
		// Pre-loaded shell (e.g. NewShellWithSessions), init the current page
		if current, ok := s.stack.Peek(); ok {
			return current.Model.Init()
		}
		return nil
	}
	return loadSourcesCmd(s.registry)
}

func (s *Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height

	case sourcesLoadedMsg:
		tuilog.Log.Info("Shell.Update: sourcesLoadedMsg received", "hasError", msg.err != nil)
		s.loading = false
		if msg.err != nil {
			tuilog.Log.Error("Shell.Update: sources loading failed", "error", msg.err)
			return s, nil
		}

		ctx := context.Background()

		// Check if we're in a known project directory
		if s.initialPage == InitialPageAuto {
			cwd, err := os.Getwd()
			if err == nil {
				if project := s.registry.FindProjectForPath(ctx, cwd); project != nil {
					tuilog.Log.Info("Shell.Update: auto-detected project from cwd", "project", project.Name, "path", project.Path)
					// Collect sessions from all sources that have this project path
					var allSessions []thinkt.SessionMeta
					for _, store := range s.registry.All() {
						projects, err := store.ListProjects(ctx)
						if err != nil {
							continue
						}
						for _, p := range projects {
							if p.Path == project.Path {
								sessions, err := store.ListSessions(ctx, p.ID)
								if err == nil {
									allSessions = append(allSessions, sessions...)
								}
							}
						}
					}
					if len(allSessions) > 0 {
						tuilog.Log.Info("Shell.Update: pushing session picker for auto-detected project", "sessionCount", len(allSessions))
						picker := NewSessionPickerModel(allSessions, nil)
						cmd := s.stack.Push(NavItem{
							Title: project.Name,
							Model: picker,
						}, s.width, s.height)
						cmds = append(cmds, cmd)
						return s, tea.Batch(cmds...)
					}
				}
			}
		}

		// Fallback: Push project picker as first page
		projects, err := s.registry.ListAllProjects(ctx)
		if err != nil {
			tuilog.Log.Error("Shell.Update: failed to list projects", "error", err)
			return s, nil
		}
		tuilog.Log.Info("Shell.Update: pushing project picker", "projectCount", len(projects))
		picker := NewProjectPickerModel(projects)
		cmd := s.stack.Push(NavItem{
			Title: "Projects",
			Model: picker,
		}, s.width, s.height)
		cmds = append(cmds, cmd)

	case ProjectPickerResult:
		tuilog.Log.Info("Shell.Update: ProjectPickerResult received", "cancelled", msg.Cancelled, "hasSelection", msg.Selected != nil)
		if msg.Cancelled {
			tuilog.Log.Info("Shell.Update: project picker cancelled, popping")
			s.stack.Pop()
			if s.stack.IsEmpty() {
				return s, tea.Quit
			}
			// Send WindowSizeMsg to the revealed page so it re-renders
			if s.width > 0 && s.height > 0 {
				cmds = append(cmds, func() tea.Msg {
					return tea.WindowSizeMsg{Width: s.width, Height: s.height}
				})
			}
			return s, tea.Batch(cmds...)
		}
		if msg.Selected != nil {
			tuilog.Log.Info("Shell.Update: project selected", "project", msg.Selected.Name, "allProjects", len(msg.AllProjects))
			ctx := context.Background()

			// List sessions from all source variants of the selected project
			var allSessions []thinkt.SessionMeta
			for _, proj := range msg.AllProjects {
				tuilog.Log.Info("Shell.Update: listing sessions", "source", proj.Source, "id", proj.ID)
				store, ok := s.registry.Get(proj.Source)
				if !ok {
					tuilog.Log.Warn("Shell.Update: store not found for source", "source", proj.Source)
					continue
				}
				sessions, err := store.ListSessions(ctx, proj.ID)
				if err != nil {
					tuilog.Log.Error("Shell.Update: failed to list sessions", "source", proj.Source, "error", err)
					continue
				}
				tuilog.Log.Info("Shell.Update: got sessions", "source", proj.Source, "count", len(sessions))
				allSessions = append(allSessions, sessions...)
			}

			tuilog.Log.Info("Shell.Update: pushing session picker", "sessionCount", len(allSessions))
			picker := NewSessionPickerModel(allSessions, msg.SourceFilter)
			cmd := s.stack.Push(NavItem{
				Title: msg.Selected.Name,
				Model: picker,
			}, s.width, s.height)
			cmds = append(cmds, cmd)
		}

	case SessionPickerResult:
		tuilog.Log.Info("Shell.Update: SessionPickerResult received", "cancelled", msg.Cancelled, "hasSelection", msg.Selected != nil)
		if msg.Cancelled {
			tuilog.Log.Info("Shell.Update: session picker cancelled, popping")
			s.stack.Pop()
			if s.stack.IsEmpty() {
				return s, tea.Quit
			}
			// Send WindowSizeMsg to the revealed page so it re-renders
			if s.width > 0 && s.height > 0 {
				cmds = append(cmds, func() tea.Msg {
					return tea.WindowSizeMsg{Width: s.width, Height: s.height}
				})
			}
			return s, tea.Batch(cmds...)
		}
		if msg.Selected != nil {
			tuilog.Log.Info("Shell.Update: session selected", "sessionID", msg.Selected.ID, "path", msg.Selected.FullPath)
			// Use multi-viewer with single session for now
			viewer := NewMultiViewerModel([]string{msg.Selected.FullPath})
			cmd := s.stack.Push(NavItem{
				Title: msg.Selected.ID[:8],
				Model: viewer,
			}, s.width, s.height)
			cmds = append(cmds, cmd)
		}

	case PopPageMsg:
		tuilog.Log.Info("Shell.Update: PopPageMsg received")
		s.stack.Pop()
		if s.stack.IsEmpty() {
			tuilog.Log.Info("Shell.Update: stack empty, quitting")
			return s, tea.Quit
		}
		// Send WindowSizeMsg to the revealed page so it re-renders
		if s.width > 0 && s.height > 0 {
			cmds = append(cmds, func() tea.Msg {
				return tea.WindowSizeMsg{Width: s.width, Height: s.height}
			})
		}
	}

	// Pass message to current page
	if current, ok := s.stack.Peek(); ok {
		newModel, cmd := current.Model.Update(msg)
		current.Model = newModel
		// Update the item in the stack
		s.stack.items[len(s.stack.items)-1] = current
		cmds = append(cmds, cmd)
	}

	return s, tea.Batch(cmds...)
}

func (s *Shell) View() tea.View {
	if s.loading {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	if s.stack.IsEmpty() {
		v := tea.NewView("No pages to display")
		v.AltScreen = true
		return v
	}

	// Get current page view
	current, _ := s.stack.Peek()
	return current.Model.View()
}

// Internal message for source loading
type sourcesLoadedMsg struct {
	err error
}

func loadSourcesCmd(registry *thinkt.StoreRegistry) tea.Cmd {
	return func() tea.Msg {
		tuilog.Log.Info("Shell: loading sources")

		discovery := thinkt.NewDiscovery(
			kimi.Factory(),
			claude.Factory(),
			gemini.Factory(),
			copilot.Factory(),
		)

		ctx := context.Background()
		discovered, err := discovery.Discover(ctx)
		if err != nil {
			tuilog.Log.Error("Shell: discovery failed", "error", err)
			return sourcesLoadedMsg{err: err}
		}

		for _, store := range discovered.All() {
			registry.Register(store)
			tuilog.Log.Info("Shell: registered store", "source", store.Source())
		}

		tuilog.Log.Info("Shell: sources loading complete", "count", len(registry.All()))
		return sourcesLoadedMsg{}
	}
}
