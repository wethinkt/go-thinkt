package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const shellHeaderHeight = 1

// shellContent is implemented by models that can render their content as a string
// for composition with the Shell's header bar.
type shellContent interface {
	viewContent() string
}

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

// OpenSearchMsg signals the shell to open the search picker.
// This is sent when the user presses the search key (e.g., '/').
type OpenSearchMsg struct{}

// InitialPage defines which page to start on
type InitialPage int

const (
	InitialPageAuto     InitialPage = iota // Auto-detect project from CWD
	InitialPageProjects                    // Always start at projects list
)

// resumeFinishedMsg is sent when a resumed CLI process exits and the TUI resumes.
type resumeFinishedMsg struct{}

// Shell is the main TUI container with navigation
type Shell struct {
	width       int
	height      int
	stack       *NavStack
	registry    *thinkt.StoreRegistry
	loading     bool
	initialPage InitialPage
	preloaded   bool
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
	return NewShellWithSessionsAndRegistry(sessions, nil, "")
}

// NewShellWithSessionsAndRegistry creates a Shell that starts with a pre-loaded
// session picker and uses the provided registry for source-aware viewing.
// projectName is used for the header breadcrumb; if empty, the project path
// from the first session is used.
func NewShellWithSessionsAndRegistry(sessions []thinkt.SessionMeta, registry *thinkt.StoreRegistry, projectName string) *Shell {
	s := &Shell{
		stack:     NewNavStack(),
		registry:  registry,
		loading:   false,
		preloaded: true,
	}

	title := "Sessions"
	if projectName != "" {
		title = projectName
	}

	picker := NewSessionPickerModel(sessions, nil)
	picker.SetResumableSources(s.resumableSources())
	s.stack.items = append(s.stack.items, NavItem{
		Title: title,
		Model: picker,
	})
	return s
}

// resumableSources returns a set of sources that support session resume.
func (s *Shell) resumableSources() map[thinkt.Source]bool {
	if s.registry == nil {
		return nil
	}
	result := make(map[thinkt.Source]bool)
	for _, store := range s.registry.All() {
		if _, ok := store.(thinkt.SessionResumer); ok {
			result[store.Source()] = true
		}
	}
	return result
}

// childHeight returns the height available for child models (terminal height minus header).
func (s *Shell) childHeight() int {
	h := s.height - shellHeaderHeight
	if h < 0 {
		h = 0
	}
	return h
}

// hasWindowSize reports whether shell has a known terminal size.
func (s *Shell) hasWindowSize() bool {
	return s.width > 0 && s.height > 0
}

// windowSizeCmd returns a size message using full terminal dimensions.
// Shell.Update normalizes all WindowSizeMsg values to child dimensions before
// forwarding to page models, so internal broadcasts must use full size.
func (s *Shell) windowSizeCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: s.width, Height: s.height}
	}
}

// renderHeader renders the top header bar with breadcrumb on the left and "thinkt" on the right.
func (s *Shell) renderHeader() string {
	t := theme.Current()

	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.TextPrimary.Fg))

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.TextMuted.Fg))

	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.TextSecondary.Fg))

	brandStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.TextMuted.Fg))

	sep := sepStyle.Render("  >  ")

	// Build left side (breadcrumb)
	var left string
	if !s.stack.IsEmpty() {
		items := s.stack.items
		current := items[len(items)-1]

		switch current.Model.(type) {
		case ProjectPickerModel:
			left = actionStyle.Render("Select project...")

		case SessionPickerModel:
			if current.Title == "Sessions" {
				left = actionStyle.Render("Select session...")
			} else {
				left = nameStyle.Render(current.Title) + sep + actionStyle.Render("Select session...")
			}

		case MultiViewerModel:
			// Find the project name from the stack
			for i := len(items) - 2; i >= 0; i-- {
				if _, ok := items[i].Model.(SessionPickerModel); ok {
					left = nameStyle.Render(items[i].Title) + sep + nameStyle.Render(current.Title)
					break
				}
			}
			if left == "" {
				left = nameStyle.Render(current.Title)
			}
			// Append role filters
			left += "  " + current.Model.(MultiViewerModel).FilterStatus()

		case SearchInputModel:
			left = actionStyle.Render("Search...")

		case SearchPickerModel:
			left = nameStyle.Render(current.Title)

		default:
			left = nameStyle.Render(current.Title)
		}
	}

	// Right side
	right := brandStyle.Render("🧠 thinkt")

	// Compose: left-justified breadcrumb, right-justified brand
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	padding := s.width - leftWidth - rightWidth
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) + right
}

func (s *Shell) Init() tea.Cmd {
	tuilog.Log.Info("Shell.Init: starting")
	if s.preloaded || s.registry == nil {
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

	// Store full terminal dimensions, then adjust for header before forwarding to children
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		s.width = wsm.Width
		s.height = wsm.Height
		msg = tea.WindowSizeMsg{Width: wsm.Width, Height: s.childHeight()}
	}

	switch msg := msg.(type) {
	case sourcesLoadedMsg:
		tuilog.Log.Info("Shell.Update: sourcesLoadedMsg received", "hasError", msg.err != nil)
		s.loading = false
		if msg.err != nil {
			tuilog.Log.Error("Shell.Update: sources loading failed", "error", msg.err)
			return s, nil
		}

		ctx := context.Background()

		// Always load all projects and push project picker as the base page
		allProjects, err := s.registry.ListAllProjects(ctx)
		if err != nil {
			tuilog.Log.Error("Shell.Update: failed to list projects", "error", err)
			return s, nil
		}
		tuilog.Log.Info("Shell.Update: pushing project picker", "projectCount", len(allProjects))
		projectPicker := NewProjectPickerModel(allProjects)
		projCmd := s.stack.Push(NavItem{
			Title: "Projects",
			Model: projectPicker,
		}, s.width, s.height)
		cmds = append(cmds, projCmd)

		// If auto-detect finds a project, push session picker on top
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
						picker.SetResumableSources(s.resumableSources())
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

		// No auto-detect match; send size to the project picker so it renders
		if s.hasWindowSize() {
			cmds = append(cmds, s.windowSizeCmd())
		}

	case ProjectPickerResult:
		tuilog.Log.Info("Shell.Update: ProjectPickerResult received", "cancelled", msg.Cancelled, "hasSelection", msg.Selected != nil)
		if msg.Cancelled {
			tuilog.Log.Info("Shell.Update: project picker cancelled, popping")
			s.stack.Pop()
			if s.stack.IsEmpty() {
				return s, tea.Quit
			}
			// Send WindowSizeMsg to the revealed page so it re-renders
			if s.hasWindowSize() {
				cmds = append(cmds, s.windowSizeCmd())
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
			picker.SetResumableSources(s.resumableSources())
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
			if s.hasWindowSize() {
				cmds = append(cmds, s.windowSizeCmd())
			}
			return s, tea.Batch(cmds...)
		}
		if msg.Selected != nil {
			tuilog.Log.Info("Shell.Update: session selected", "sessionID", msg.Selected.ID, "path", msg.Selected.FullPath)
			// Use multi-viewer with single session for now
			viewer := NewMultiViewerModelWithRegistry([]string{msg.Selected.FullPath}, s.registry)
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
		if s.hasWindowSize() {
			cmds = append(cmds, s.windowSizeCmd())
		}

	case SearchPickerResult:
		tuilog.Log.Info("Shell.Update: SearchPickerResult received", "cancelled", msg.Cancelled, "hasSelection", msg.Selected != nil)
		if msg.Cancelled {
			tuilog.Log.Info("Shell.Update: search picker cancelled, popping")
			s.stack.Pop()
			if s.stack.IsEmpty() {
				return s, tea.Quit
			}
			// Send WindowSizeMsg to the revealed page so it re-renders
			if s.hasWindowSize() {
				cmds = append(cmds, s.windowSizeCmd())
			}
			return s, tea.Batch(cmds...)
		}
		if msg.Selected != nil {
			tuilog.Log.Info("Shell.Update: search result selected", "sessionID", msg.Selected.SessionID, "path", msg.Selected.Path)
			// Open the selected session in the viewer
			viewer := NewMultiViewerModelWithRegistry([]string{msg.Selected.Path}, s.registry)
			cmd := s.stack.Push(NavItem{
				Title: msg.Selected.SessionID[:8],
				Model: viewer,
			}, s.width, s.height)
			cmds = append(cmds, cmd)
		}

	case ResumeSessionMsg:
		tuilog.Log.Info("Shell.Update: ResumeSessionMsg received", "sessionID", msg.Session.ID, "source", msg.Session.Source)
		if s.registry != nil {
			if resumer, ok := s.registry.GetResumer(msg.Session.Source); ok {
				info, err := resumer.ResumeCommand(msg.Session)
				if err != nil {
					tuilog.Log.Error("Shell.Update: ResumeCommand failed", "error", err)
					return s, nil
				}
				tuilog.Log.Info("Shell.Update: executing resume", "command", info.Command, "args", info.Args, "dir", info.Dir)
				c := exec.Command(info.Command, info.Args[1:]...)
				if info.Dir != "" {
					c.Dir = info.Dir
				}
				return s, tea.ExecProcess(c, func(err error) tea.Msg {
					if err != nil {
						tuilog.Log.Error("Shell.Update: resume process exited with error", "error", err)
					}
					return resumeFinishedMsg{}
				})
			}
			tuilog.Log.Info("Shell.Update: source does not support resume", "source", msg.Session.Source)
		}
		return s, nil

	case resumeFinishedMsg:
		tuilog.Log.Info("Shell.Update: resume finished, re-rendering")
		if s.hasWindowSize() {
			cmds = append(cmds, s.windowSizeCmd())
		}

	case OpenSearchMsg:
		tuilog.Log.Info("Shell.Update: OpenSearchMsg received")
		// Check if indexer is available
		if !IndexerAvailable() {
			tuilog.Log.Warn("Shell.Update: indexer not available, cannot search")
			return s, nil
		}
		// Open the search input overlay
		input := NewSearchInputModel()
		cmd := s.stack.Push(NavItem{
			Title: "Search",
			Model: input,
		}, s.width, s.height)
		cmds = append(cmds, cmd)

	case SearchInputResult:
		tuilog.Log.Info("Shell.Update: SearchInputResult received", "cancelled", msg.Cancelled, "query", msg.Query)
		if msg.Cancelled {
			tuilog.Log.Info("Shell.Update: search input cancelled, popping")
			s.stack.Pop()
			// Send WindowSizeMsg to the revealed page so it re-renders
			if s.hasWindowSize() {
				cmds = append(cmds, s.windowSizeCmd())
			}
			return s, tea.Batch(cmds...)
		}
		if msg.Query != "" {
			tuilog.Log.Info("Shell.Update: performing search", "query", msg.Query)
			// Perform the search
			results, err := PerformSearch(msg.Query, search.DefaultSearchOptions())
			if err != nil {
				tuilog.Log.Error("Shell.Update: search failed", "error", err)
				s.stack.Pop()
				return s, tea.Batch(cmds...)
			}
			if len(results) == 0 {
				tuilog.Log.Info("Shell.Update: no search results")
				s.stack.Pop()
				return s, tea.Batch(cmds...)
			}
			// Replace the search input with the results picker
			s.stack.Pop()
			picker := NewSearchPickerModel(results, msg.Query)
			cmd := s.stack.Push(NavItem{
				Title: fmt.Sprintf("Search: %s", msg.Query),
				Model: picker,
			}, s.width, s.height)
			cmds = append(cmds, cmd)
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
		t := theme.Current()
		brandStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted.Fg))
		loadingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary.Fg))

		brand := brandStyle.Render("🧠 thinkt")
		loading := loadingStyle.Render("Loading...")

		// Brand in top-right corner
		brandWidth := lipgloss.Width(brand)
		pad := s.width - brandWidth
		if pad < 0 {
			pad = 0
		}
		header := strings.Repeat(" ", pad) + brand

		// Centered loading text
		loadingWidth := lipgloss.Width(loading)
		leftPad := (s.width - loadingWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		topPad := (s.height - 3) / 2 // -3 for header + loading + buffer
		if topPad < 0 {
			topPad = 0
		}
		center := strings.Repeat("\n", topPad) + strings.Repeat(" ", leftPad) + loading

		v := tea.NewView(header + center)
		v.AltScreen = true
		return v
	}

	if s.stack.IsEmpty() {
		v := tea.NewView("No pages to display")
		v.AltScreen = true
		return v
	}

	current, _ := s.stack.Peek()

	// Compose header + child content if the model supports it
	if cv, ok := current.Model.(shellContent); ok {
		header := s.renderHeader()
		content := cv.viewContent()
		v := tea.NewView(header + "\n" + content)
		v.AltScreen = true
		return v
	}

	// Fallback for models that don't implement shellContent
	return current.Model.View()
}

// Internal message for source loading
type sourcesLoadedMsg struct {
	err error
}

func loadSourcesCmd(registry *thinkt.StoreRegistry) tea.Cmd {
	return func() tea.Msg {
		tuilog.Log.Info("Shell: loading sources")

		discovery := thinkt.NewDiscovery(sources.AllFactories()...)

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
