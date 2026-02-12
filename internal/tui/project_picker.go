package tui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// sourceCount tracks sessions from one source for an aggregated project.
type sourceCount struct {
	Source       thinkt.Source
	SessionCount int
	Project      thinkt.Project
}

// aggregatedProject groups same-path projects across sources.
type aggregatedProject struct {
	Path          string
	Name          string // short display name
	Sources       []sourceCount
	TotalSessions int
	LastModified  time.Time
	PathExists    bool
}

// sortField identifies which field to sort projects by.
type sortField int

const (
	sortByDate sortField = iota
	sortByName
)

func (f sortField) String() string {
	switch f {
	case sortByName:
		return "name"
	default:
		return "date"
	}
}

// sortDir is the sort direction.
type sortDir int

const (
	sortDesc sortDir = iota
	sortAsc
)

func (d sortDir) arrow() string {
	if d == sortAsc {
		return "↑"
	}
	return "↓"
}

// aggregateProjects groups projects by Path, collecting per-source session counts.
func aggregateProjects(projects []thinkt.Project) []aggregatedProject {
	byPath := make(map[string]*aggregatedProject)
	var order []string

	for _, p := range projects {
		agg, exists := byPath[p.Path]
		if !exists {
			agg = &aggregatedProject{
				Path:       p.Path,
				Name:       shortenPath(p.Path),
				PathExists: p.PathExists,
			}
			byPath[p.Path] = agg
			order = append(order, p.Path)
		}

		agg.Sources = append(agg.Sources, sourceCount{
			Source:       p.Source,
			SessionCount: p.SessionCount,
			Project:      p,
		})
		agg.TotalSessions += p.SessionCount

		if p.LastModified.After(agg.LastModified) {
			agg.LastModified = p.LastModified
		}
		if p.PathExists {
			agg.PathExists = true
		}
	}

	result := make([]aggregatedProject, 0, len(byPath))
	for _, path := range order {
		result = append(result, *byPath[path])
	}

	return result
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string) string {
	if path == "" {
		return "~"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// relativeDate formats a time as a short relative string.
func relativeDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	d := now.Sub(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "1d ago"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

// flatProjectDelegate renders projects as a flat single-line list (no tree structure).
type flatProjectDelegate struct {
	normalStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimmedStyle   lipgloss.Style
	mutedStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
}

func newFlatProjectDelegate() flatProjectDelegate {
	t := theme.Current()
	return flatProjectDelegate{
		normalStyle:   lipgloss.NewStyle().PaddingLeft(2),
		selectedStyle: lipgloss.NewStyle().PaddingLeft(1).Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:   lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color(t.TextMuted.Fg)),
		mutedStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
	}
}

func (d flatProjectDelegate) Height() int  { return 1 }
func (d flatProjectDelegate) Spacing() int { return 0 }
func (d flatProjectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d flatProjectDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "sort date")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "sort name")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sources")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tree view")),
	}
}

func (d flatProjectDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d flatProjectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ti, ok := item.(treeItem)
	if !ok || ti.node.kind != treeNodeLeaf || ti.node.project == nil {
		return
	}
	if m.Width() <= 0 {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	agg := ti.node.project

	// Build source badges
	var badges []string
	for _, sc := range agg.Sources {
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(SourceColorHex(sc.Source))).
			Render(fmt.Sprintf("%s:%d", sc.Source, sc.SessionCount))
		badges = append(badges, badge)
	}
	badgeStr := strings.Join(badges, "  ")
	dateStr := relativeDate(agg.LastModified)

	var line string
	if emptyFilter {
		line = d.dimmedStyle.Render(agg.Name)
	} else if isSelected {
		marker := d.cursorStyle.Render("> ")
		name := d.selectedStyle.Render(agg.Name)
		date := d.mutedStyle.Render(dateStr)
		line = marker + name + "  " + badgeStr + "  " + date
	} else {
		name := d.normalStyle.Render(agg.Name)
		date := d.mutedStyle.Render(dateStr)
		line = name + "  " + badgeStr + "  " + date
	}

	fmt.Fprint(w, line) //nolint: errcheck
}

// treeProjectDelegate renders tree items (both dir headers and leaf projects).
type treeProjectDelegate struct {
	normalStyle    lipgloss.Style
	selectedStyle  lipgloss.Style
	dimmedStyle    lipgloss.Style
	mutedStyle     lipgloss.Style
	cursorStyle    lipgloss.Style
	connectorStyle lipgloss.Style
	dirStyle       lipgloss.Style
}

func newTreeProjectDelegate() treeProjectDelegate {
	t := theme.Current()
	return treeProjectDelegate{
		normalStyle:    lipgloss.NewStyle(),
		selectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
		mutedStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		cursorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		connectorStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetBorderInactive())),
		dirStyle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TextSecondary.Fg)),
	}
}

func (d treeProjectDelegate) Height() int  { return 1 }
func (d treeProjectDelegate) Spacing() int { return 0 }
func (d treeProjectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// ShortHelp returns key bindings for the short help view.
func (d treeProjectDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "sort date")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "sort name")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sources")),
		key.NewBinding(key.WithKeys("left", "right", "space"), key.WithHelp("←/→/space", "collapse/expand")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "flat view")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	}
}

// FullHelp returns key bindings for the full help view.
func (d treeProjectDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d treeProjectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ti, ok := item.(treeItem)
	if !ok {
		return
	}
	if m.Width() <= 0 {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""

	// Tree connector prefix
	prefix := d.connectorStyle.Render(treePrefix(ti.depth, ti.isLast))

	// Cursor marker
	cursor := "  "
	if isSelected {
		cursor = d.cursorStyle.Render("> ")
	}

	if ti.node.kind == treeNodeDir {
		// Directory node: show expand/collapse indicator + label
		indicator := "▼ "
		if !ti.node.expanded {
			indicator = "▶ "
		}
		var line string
		if emptyFilter {
			line = "  " + prefix + d.dimmedStyle.Render(indicator+ti.node.label)
		} else if isSelected {
			line = cursor + prefix + d.selectedStyle.Render(indicator+ti.node.label)
		} else {
			line = "  " + prefix + d.dirStyle.Render(indicator+ti.node.label)
		}
		fmt.Fprint(w, line) //nolint: errcheck
		return
	}

	// Leaf node: show project with badges and date
	agg := ti.node.project
	if agg == nil {
		return
	}

	// Build source badges: "claude:5  kimi:3"
	var badges []string
	for _, sc := range agg.Sources {
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(SourceColorHex(sc.Source))).
			Render(fmt.Sprintf("%s:%d", sc.Source, sc.SessionCount))
		badges = append(badges, badge)
	}
	badgeStr := strings.Join(badges, "  ")

	dateStr := relativeDate(agg.LastModified)

	var line string
	if emptyFilter {
		line = "  " + prefix + d.dimmedStyle.Render(ti.node.label)
	} else if isSelected {
		name := d.selectedStyle.Render(ti.node.label)
		date := d.mutedStyle.Render(dateStr)
		line = cursor + prefix + name + "  " + badgeStr + "  " + date
	} else {
		name := d.normalStyle.Render(ti.node.label)
		date := d.mutedStyle.Render(dateStr)
		line = "  " + prefix + name + "  " + badgeStr + "  " + date
	}

	fmt.Fprint(w, line) //nolint: errcheck
}

// ProjectPickerResult holds the result of the project picker.
type ProjectPickerResult struct {
	Selected     *thinkt.Project
	AllProjects  []thinkt.Project // all source variants for the selected path
	SourceFilter []thinkt.Source  // active source filter (empty = all)
	Cancelled    bool
}

// ProjectPickerModel is a project browser TUI.
type ProjectPickerModel struct {
	list         list.Model
	allProjects  []thinkt.Project    // original unfiltered projects
	aggregated   []aggregatedProject // current aggregated+filtered view
	treeRoots    []*treeNode         // tree structure for display
	expandState  map[string]bool     // preserved expand/collapse state by fullPath
	result       ProjectPickerResult
	quitting     bool
	width        int
	height       int
	ready        bool
	standalone   bool // true when run via PickProject(), false when embedded in Shell
	treeView     bool // true = tree, false = flat list
	sortField    sortField
	sortDir      sortDir
	sourceFilter []thinkt.Source // empty = all sources
	showSources  bool            // true when source picker overlay is active
	sourcePicker SourcePickerModel
	showApps     bool // true when app picker overlay is active
	appPicker    AppPickerModel
}

type projectPickerKeyMap struct {
	Enter      key.Binding
	Back       key.Binding
	Quit       key.Binding
	SortDate   key.Binding
	SortName   key.Binding
	Sources    key.Binding
	OpenIn     key.Binding
	Left       key.Binding
	Right      key.Binding
	Toggle     key.Binding
	TreeToggle key.Binding
	Search     key.Binding // / key for search
}

func defaultProjectPickerKeyMap() projectPickerKeyMap {
	return projectPickerKeyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		SortDate: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "sort date"),
		),
		SortName: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "sort name"),
		),
		Sources: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sources"),
		),
		OpenIn: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in"),
		),
		Left: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "collapse"),
		),
		Right: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "expand"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "toggle"),
		),
		TreeToggle: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tree/flat"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
	}
}

// pickerTitle returns the list title with sort info, item count, and source filter.
func pickerTitle(field sortField, dir sortDir, count int, sourceFilter []thinkt.Source) string {
	title := fmt.Sprintf("%d projects · %s %s", count, field, dir.arrow())
	if len(sourceFilter) > 0 {
		names := make([]string, len(sourceFilter))
		for i, s := range sourceFilter {
			names[i] = string(s)
		}
		title += " · " + strings.Join(names, ",")
	}
	return title
}

// NewProjectPickerModel creates a new project picker with a tree view.
func NewProjectPickerModel(projects []thinkt.Project) ProjectPickerModel {
	sf := sortByDate
	sd := sortDesc

	aggregated := aggregateProjects(projects)

	// Build tree
	roots := buildProjectTree(aggregated)
	compactTree(roots)
	sortTree(roots, sf, sd)
	flat := flattenTree(roots)

	items := make([]list.Item, len(flat))
	for i, fi := range flat {
		items[i] = fi
	}

	delegate := newTreeProjectDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = pickerTitle(sf, sd, countLeaves(roots), nil)
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return ProjectPickerModel{
		list:        l,
		allProjects: projects,
		aggregated:  aggregated,
		treeRoots:   roots,
		expandState: make(map[string]bool),
		treeView:    true,
		sortField:   sf,
		sortDir:     sd,
	}
}

// reflattened re-flattens the tree after expand/collapse changes and updates the list.
func (m *ProjectPickerModel) reflattened() tea.Cmd {
	// Preserve selection by finding the currently selected node
	var selectedNode *treeNode
	if item := m.list.SelectedItem(); item != nil {
		if ti, ok := item.(treeItem); ok {
			selectedNode = ti.node
		}
	}

	flat := flattenTree(m.treeRoots)
	items := make([]list.Item, len(flat))
	newIndex := 0
	for i, fi := range flat {
		items[i] = fi
		if fi.node == selectedNode {
			newIndex = i
		}
	}

	cmd := m.list.SetItems(items)
	m.list.Select(newIndex)
	return cmd
}

func (m ProjectPickerModel) Init() tea.Cmd {
	tuilog.Log.Info("ProjectPicker.Init", "projectCount", len(m.aggregated))
	return nil
}

// buildFlatItems creates flat treeItem leaves (depth 0) sorted by the current sort settings.
func (m *ProjectPickerModel) buildFlatItems() []list.Item {
	// Sort aggregated for flat mode
	sorted := make([]aggregatedProject, len(m.aggregated))
	copy(sorted, m.aggregated)
	sortFlatProjects(sorted, m.sortField, m.sortDir)

	items := make([]list.Item, len(sorted))
	for i := range sorted {
		items[i] = treeItem{
			node: &treeNode{
				label:    sorted[i].Name,
				fullPath: sorted[i].Path,
				kind:     treeNodeLeaf,
				project:  &sorted[i],
			},
			depth:  0,
			isLast: []bool{i == len(sorted)-1},
		}
	}
	return items
}

// sortFlatProjects sorts aggregated projects in place for flat view.
func sortFlatProjects(projects []aggregatedProject, field sortField, dir sortDir) {
	sort.Slice(projects, func(i, j int) bool {
		var less bool
		switch field {
		case sortByName:
			less = strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
		default:
			less = projects[i].LastModified.Before(projects[j].LastModified)
		}
		if dir == sortDesc {
			return !less
		}
		return less
	})
}

// rebuildAndRefresh re-aggregates from allProjects with current source filter,
// rebuilds the tree or flat list, and updates the list items.
func (m *ProjectPickerModel) rebuildAndRefresh() tea.Cmd {
	// Filter source projects before aggregation
	var filtered []thinkt.Project
	if len(m.sourceFilter) == 0 {
		filtered = m.allProjects
	} else {
		allowed := make(map[thinkt.Source]bool, len(m.sourceFilter))
		for _, s := range m.sourceFilter {
			allowed[s] = true
		}
		for _, p := range m.allProjects {
			if allowed[p.Source] {
				filtered = append(filtered, p)
			}
		}
	}

	m.aggregated = aggregateProjects(filtered)

	var items []list.Item
	var count int

	if m.treeView {
		// Save expand state before rebuilding
		saveExpandState(m.treeRoots, m.expandState)

		// Build tree
		m.treeRoots = buildProjectTree(m.aggregated)
		compactTree(m.treeRoots)
		sortTree(m.treeRoots, m.sortField, m.sortDir)

		// Restore expand state
		restoreExpandState(m.treeRoots, m.expandState)

		flat := flattenTree(m.treeRoots)
		items = make([]list.Item, len(flat))
		for i, fi := range flat {
			items[i] = fi
		}
		count = countLeaves(m.treeRoots)
	} else {
		items = m.buildFlatItems()
		count = len(items)
	}

	m.list.Title = pickerTitle(m.sortField, m.sortDir, count, m.sourceFilter)
	return m.list.SetItems(items)
}

func (m ProjectPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If source picker overlay is active, delegate to it
	if m.showSources {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			updated, cmd := m.sourcePicker.Update(msg)
			m.sourcePicker = updated.(SourcePickerModel)

			if m.sourcePicker.quitting {
				m.showSources = false
				result := m.sourcePicker.Result()
				if !result.Cancelled {
					m.sourceFilter = result.Sources
					rebuildCmd := m.rebuildAndRefresh()
					return m, tea.Batch(cmd, rebuildCmd)
				}
			}
			return m, cmd

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.list.SetSize(msg.Width, msg.Height-2)
			updated, cmd := m.sourcePicker.Update(msg)
			m.sourcePicker = updated.(SourcePickerModel)
			return m, cmd
		}
		return m, nil
	}

	// If app picker overlay is active, delegate to it
	if m.showApps {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			updated, cmd := m.appPicker.Update(msg)
			m.appPicker = updated.(AppPickerModel)

			if m.appPicker.quitting {
				m.showApps = false
				result := m.appPicker.Result()
				if !result.Cancelled && result.App != nil {
					// We need the selected project path
					var path string
					if item := m.list.SelectedItem(); item != nil {
						if ti, ok := item.(treeItem); ok && ti.node.project != nil {
							path = ti.node.project.Path
						}
					}

					if path != "" {
						// Launch the app!
						err := result.App.Launch(path)
						if err != nil {
							tuilog.Log.Error("ProjectPicker: failed to launch app", "app", result.App.Name, "error", err)
						}
					}
				}
			}
			return m, cmd

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.list.SetSize(msg.Width, msg.Height-2)
			updated, cmd := m.appPicker.Update(msg)
			m.appPicker = updated.(AppPickerModel)
			return m, cmd
		}
		return m, nil
	}

	keys := defaultProjectPickerKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		// Don't handle keys if filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, keys.Back):
			tuilog.Log.Info("ProjectPicker.Update: Back key pressed")
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case key.Matches(msg, keys.Quit):
			tuilog.Log.Info("ProjectPicker.Update: Quit key pressed")
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.SortDate):
			if m.sortField == sortByDate {
				// Same field: toggle direction
				if m.sortDir == sortDesc {
					m.sortDir = sortAsc
				} else {
					m.sortDir = sortDesc
				}
			} else {
				m.sortField = sortByDate
				m.sortDir = sortDesc
			}
			cmd := m.rebuildAndRefresh()
			return m, cmd

		case key.Matches(msg, keys.SortName):
			if m.sortField == sortByName {
				// Same field: toggle direction
				if m.sortDir == sortAsc {
					m.sortDir = sortDesc
				} else {
					m.sortDir = sortAsc
				}
			} else {
				m.sortField = sortByName
				m.sortDir = sortAsc
			}
			cmd := m.rebuildAndRefresh()
			return m, cmd

		case key.Matches(msg, keys.Sources):
			// Build source options from allProjects
			seen := make(map[thinkt.Source]bool)
			for _, p := range m.allProjects {
				seen[p.Source] = true
			}
			selectedSet := make(map[thinkt.Source]bool)
			for _, s := range m.sourceFilter {
				selectedSet[s] = true
			}
			// If no filter active, pre-select all available
			if len(m.sourceFilter) == 0 {
				for s := range seen {
					selectedSet[s] = true
				}
			}

			allSources := []thinkt.Source{
				thinkt.SourceClaude,
				thinkt.SourceKimi,
				thinkt.SourceGemini,
				thinkt.SourceCopilot,
				thinkt.SourceCodex,
			}
			var options []SourceOption
			for _, s := range allSources {
				options = append(options, SourceOption{
					Source:   s,
					Enabled:  seen[s],
					Selected: selectedSet[s],
				})
			}

			m.sourcePicker = NewSourcePickerModel(options, true)
			m.sourcePicker.SetSize(m.width, m.height)
			m.showSources = true
			return m, nil

		case key.Matches(msg, keys.OpenIn):
			// Only if a project is selected
			if item := m.list.SelectedItem(); item != nil {
				if ti, ok := item.(treeItem); ok && ti.node.project != nil {
					projectPath := ti.node.project.Path
					cfg, err := config.Load()
					if err == nil {
						m.appPicker = NewAppPickerModel(cfg.AllowedApps, projectPath)
						m.appPicker.SetSize(m.width, m.height)
						m.showApps = true
						return m, nil
					}
				}
			}
			return m, nil

		case key.Matches(msg, keys.TreeToggle):
			m.treeView = !m.treeView
			// Switch delegate
			if m.treeView {
				m.list.SetDelegate(newTreeProjectDelegate())
			} else {
				m.list.SetDelegate(newFlatProjectDelegate())
			}
			cmd := m.rebuildAndRefresh()
			return m, cmd

		case key.Matches(msg, keys.Search):
			tuilog.Log.Info("ProjectPicker.Update: Search key pressed")
			// Signal the shell to open search
			return m, func() tea.Msg { return OpenSearchMsg{} }

		case m.treeView && key.Matches(msg, keys.Toggle):
			// Toggle expand/collapse on directory nodes
			if item := m.list.SelectedItem(); item != nil {
				if ti, ok := item.(treeItem); ok && ti.node.kind == treeNodeDir {
					ti.node.expanded = !ti.node.expanded
					cmd := m.reflattened()
					return m, cmd
				}
			}
			return m, nil

		case m.treeView && key.Matches(msg, keys.Left):
			// Collapse current directory
			if item := m.list.SelectedItem(); item != nil {
				if ti, ok := item.(treeItem); ok && ti.node.kind == treeNodeDir && ti.node.expanded {
					ti.node.expanded = false
					cmd := m.reflattened()
					return m, cmd
				}
			}
			return m, nil

		case m.treeView && key.Matches(msg, keys.Right):
			// Expand current directory
			if item := m.list.SelectedItem(); item != nil {
				if ti, ok := item.(treeItem); ok && ti.node.kind == treeNodeDir && !ti.node.expanded {
					ti.node.expanded = true
					cmd := m.reflattened()
					return m, cmd
				}
			}
			return m, nil

		case key.Matches(msg, keys.Enter):
			tuilog.Log.Info("ProjectPicker.Update: Enter key pressed")
			if item := m.list.SelectedItem(); item != nil {
				if ti, ok := item.(treeItem); ok {
					if ti.node.kind == treeNodeDir {
						// Toggle expand/collapse
						ti.node.expanded = !ti.node.expanded
						cmd := m.reflattened()
						return m, cmd
					}

					// Leaf node: select project
					agg := ti.node.project
					if agg != nil {
						tuilog.Log.Info("ProjectPicker.Update: project selected", "path", agg.Path, "sources", len(agg.Sources))

						// Select the most recently modified source variant
						var best *thinkt.Project
						for i := range agg.Sources {
							p := &agg.Sources[i].Project
							if best == nil || p.LastModified.After(best.LastModified) {
								best = p
							}
						}
						m.result.Selected = best

						// Collect all projects for this path
						all := make([]thinkt.Project, len(agg.Sources))
						for i, sc := range agg.Sources {
							all[i] = sc.Project
						}
						m.result.AllProjects = all
						m.result.SourceFilter = m.sourceFilter
					}
				}
			}
			if m.standalone {
				m.quitting = true
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

var projectPickerStyle = lipgloss.NewStyle().Padding(1, 2)

func (m ProjectPickerModel) viewContent() string {
	if !m.ready {
		return "Loading..."
	}
	if m.quitting {
		return ""
	}
	if m.showSources {
		return m.sourcePicker.viewContent()
	}
	if m.showApps {
		return m.appPicker.viewContent()
	}
	return projectPickerStyle.Render(m.list.View())
}

func (m ProjectPickerModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

// Result returns the picker result after the program exits.
func (m ProjectPickerModel) Result() ProjectPickerResult {
	return m.result
}

// PickProject runs the project picker and returns the selected project.
func PickProject(projects []thinkt.Project) (*thinkt.Project, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects available")
	}

	model := NewProjectPickerModel(projects)
	model.standalone = true
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(ProjectPickerModel).Result()
	if result.Cancelled {
		return nil, nil
	}
	return result.Selected, nil
}
