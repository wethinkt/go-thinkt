package tui

import (
	"context"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
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

// Shell is the main TUI container with navigation
type Shell struct {
	width    int
	height   int
	stack    *NavStack
	registry *thinkt.StoreRegistry
	loading  bool
}

// NewShell creates the main TUI shell
func NewShell() *Shell {
	return &Shell{
		stack:    NewNavStack(),
		registry: thinkt.NewRegistry(),
		loading:  true,
	}
}

func (s *Shell) Init() tea.Cmd {
	tuilog.Log.Info("Shell.Init: starting")
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
		cwd, err := os.Getwd()
		if err == nil {
			if project := s.registry.FindProjectForPath(ctx, cwd); project != nil {
				tuilog.Log.Info("Shell.Update: auto-detected project from cwd", "project", project.Name, "path", project.Path)
				// Skip project picker, go directly to session picker
				store, ok := s.registry.Get(project.Source)
				if ok {
					sessions, err := store.ListSessions(ctx, project.ID)
					if err == nil && len(sessions) > 0 {
						tuilog.Log.Info("Shell.Update: pushing session picker for auto-detected project", "sessionCount", len(sessions))
						picker := NewSessionPickerModel(sessions)
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
			return s, nil
		}
		if msg.Selected != nil {
			tuilog.Log.Info("Shell.Update: project selected", "project", msg.Selected.Name, "source", msg.Selected.Source)
			ctx := context.Background()
			store, ok := s.registry.Get(msg.Selected.Source)
			if !ok {
				tuilog.Log.Error("Shell.Update: store not found for source", "source", msg.Selected.Source)
				return s, nil
			}
			sessions, err := store.ListSessions(ctx, msg.Selected.ID)
			if err != nil {
				tuilog.Log.Error("Shell.Update: failed to list sessions", "error", err)
				return s, nil
			}
			tuilog.Log.Info("Shell.Update: pushing session picker", "sessionCount", len(sessions))
			picker := NewSessionPickerModel(sessions)
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
			return s, nil
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
		// Try Kimi
		kimiDir, err := kimi.DefaultDir()
		tuilog.Log.Info("Shell: kimi DefaultDir", "dir", kimiDir, "error", err)
		if err == nil && kimiDir != "" {
			store := kimi.NewStore(kimiDir)
			registry.Register(store)
			tuilog.Log.Info("Shell: registered kimi store")
		}

		// Try Claude
		claudeDir, err := claude.DefaultDir()
		tuilog.Log.Info("Shell: claude DefaultDir", "dir", claudeDir, "error", err)
		if err == nil && claudeDir != "" {
			store := claude.NewStore(claudeDir)
			registry.Register(store)
			tuilog.Log.Info("Shell: registered claude store")
		}

		tuilog.Log.Info("Shell: sources loading complete")
		return sourcesLoadedMsg{}
	}
}
