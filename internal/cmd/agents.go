package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/agents"
	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

var (
	agentsLocal   bool
	agentsRemote  bool
	agentsSource  string
	agentsMachine string
	agentsJSON    bool
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List active agents (local and remote)",
	Long: `List all currently active AI coding agents across local and remote infrastructure.

Local agents are detected via process inspection, IDE lock files, and file modification times.
Remote agents are discovered from running collector instances.

Examples:
  thinkt agents                      # List all active agents
  thinkt agents --local              # Local agents only
  thinkt agents --remote             # Remote agents only (from collectors)
  thinkt agents --source claude      # Filter by source
  thinkt agents --json               # JSON output`,
	RunE: runAgentsList,
}

var agentsFollowCmd = &cobra.Command{
	Use:   "follow [session-id]",
	Short: "Live-tail an agent's conversation",
	Long: `Stream new conversation entries from an active agent in real-time.

Local agents are tailed directly from their session files.
Remote agents are streamed via WebSocket from the collector.

Examples:
  thinkt agents follow a3f8b2c1          # Tail agent conversation
  thinkt agents follow a3f8b2c1 --json   # Structured JSON output
  thinkt agents follow a3f8b2c1 --raw    # Raw JSONL`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgentsFollow,
}

var (
	followRaw  bool
	followJSON bool
)

func buildHub() *agents.AgentHub {
	registry := CreateSourceRegistry()
	detector := thinkt.NewActiveSessionDetector(registry)

	// Find collector URLs from running instances
	var collectorURLs []string
	instances, err := config.ListInstances()
	if err == nil {
		for _, inst := range instances {
			if inst.Type == collect.InstanceCollector {
				host := inst.Host
				if host == "" {
					host = "localhost"
				}
				collectorURLs = append(collectorURLs, fmt.Sprintf("http://%s:%d", host, inst.Port))
			}
		}
	}

	return agents.NewHub(agents.HubConfig{
		Detector:      detector,
		CollectorURLs: collectorURLs,
	})
}

func runAgentsList(cmd *cobra.Command, args []string) error {
	hub := buildHub()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hub.PollOnce(ctx)

	filter := agents.AgentFilter{
		Source:     agentsSource,
		MachineID:  agentsMachine,
		LocalOnly:  agentsLocal,
		RemoteOnly: agentsRemote,
	}

	list := hub.List(filter)

	if agentsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}

	if len(list) == 0 {
		fmt.Println("No active agents found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tSOURCE\tPROJECT\tSESSION\tMACHINE\tAGE")
	for _, a := range list {
		sessionID := a.SessionID
		if len(sessionID) > 8 {
			sessionID = sessionID[:8]
		}
		project := shortenPathCLI(a.ProjectPath)
		age := time.Since(a.DetectedAt).Truncate(time.Second).String()
		machine := a.MachineName
		if machine == "" {
			machine = a.Hostname
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			a.Status, a.Source, project, sessionID, machine, age)
	}
	return w.Flush()
}

// followModel wraps AgentTailModel for standalone CLI usage,
// converting the back/dismiss result into tea.Quit.
type followModel struct {
	inner tui.AgentTailModel
}

func (m followModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m followModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tui.AgentTailResult:
		return m, tea.Quit
	}
	updated, cmd := m.inner.Update(msg)
	m.inner = updated.(tui.AgentTailModel)
	return m, cmd
}

func (m followModel) View() tea.View {
	return m.inner.View()
}

func runAgentsFollow(cmd *cobra.Command, args []string) error {
	hub := buildHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt for raw/json modes
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	// Initial poll to find the agent
	hub.PollOnce(ctx)

	var sessionID string
	if len(args) > 0 {
		sessionID = args[0]
	} else {
		allAgents := hub.List(agents.AgentFilter{})
		if len(allAgents) == 0 {
			fmt.Println("No active agents found.")
			return nil
		}
		selected, err := pickAgent(allAgents)
		if err != nil {
			return err
		}
		if selected == nil {
			return nil
		}
		sessionID = selected.SessionID
	}

	// Raw/JSON modes: stream without TUI
	if followJSON || followRaw {
		ch, err := hub.Stream(ctx, sessionID, 0)
		if err != nil {
			return err
		}
		for entry := range ch {
			data, _ := json.Marshal(entry)
			fmt.Println(string(data))
		}
		return nil
	}

	// TUI mode: use AgentTailModel with themed rendering
	agent, ok := hub.FindBySessionID(sessionID)
	if !ok {
		return fmt.Errorf("agent %s not found", sessionID)
	}

	model := followModel{inner: tui.NewAgentTailModel(hub, agent)}
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}

// --- agent picker TUI ---

type agentPickItem struct {
	agent agents.UnifiedAgent
}

func (i agentPickItem) Title() string {
	project := shortenPathCLI(i.agent.ProjectPath)
	return fmt.Sprintf("[%s] %s", i.agent.Source, project)
}

func (i agentPickItem) Description() string {
	sid := i.agent.SessionID
	if len(sid) > 8 {
		sid = sid[:8]
	}
	age := time.Since(i.agent.DetectedAt).Truncate(time.Second).String()
	return fmt.Sprintf("%s  %s  %s", sid, i.agent.Status, age)
}

func (i agentPickItem) FilterValue() string {
	return i.agent.Source + " " + i.agent.ProjectPath + " " + i.agent.SessionID
}

type agentPickModel struct {
	list     list.Model
	selected *agents.UnifiedAgent
	quitting bool
}

func newAgentPickModel(agentList []agents.UnifiedAgent) agentPickModel {
	items := make([]list.Item, len(agentList))
	for i, a := range agentList {
		items[i] = agentPickItem{agent: a}
	}

	t := theme.Current()
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(lipgloss.Color(t.TextPrimary.Fg))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(lipgloss.Color(t.TextSecondary.Fg))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(t.GetAccent())).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(t.TextMuted.Fg))
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.
		Foreground(lipgloss.Color(t.TextMuted.Fg))
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.
		Foreground(lipgloss.Color(t.TextMuted.Fg))

	l := list.New(items, delegate, 60, min(len(items)*3+6, 20))
	l.SetShowTitle(true)
	l.Title = "Select an agent to follow"
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false)

	return agentPickModel{list: l}
}

func (m agentPickModel) Init() tea.Cmd { return nil }

func (m agentPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(agentPickItem); ok {
				m.selected = &item.agent
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m agentPickModel) View() tea.View {
	if m.quitting && m.selected == nil {
		return tea.NewView("")
	}
	return tea.NewView(m.list.View())
}

func pickAgent(agentList []agents.UnifiedAgent) (*agents.UnifiedAgent, error) {
	model := newAgentPickModel(agentList)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(agentPickModel)
	return m.selected, nil
}

func shortenPathCLI(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
