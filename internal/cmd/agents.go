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

	"github.com/wethinkt/go-thinkt/internal/agents"
	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
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
	Use:   "follow <session-id>",
	Short: "Live-tail an agent's conversation",
	Long: `Stream new conversation entries from an active agent in real-time.

Local agents are tailed directly from their session files.
Remote agents are streamed via WebSocket from the collector.

Examples:
  thinkt agents follow a3f8b2c1          # Tail agent conversation
  thinkt agents follow a3f8b2c1 --json   # Structured JSON output
  thinkt agents follow a3f8b2c1 --raw    # Raw JSONL`,
	Args: cobra.ExactArgs(1),
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

func runAgentsFollow(cmd *cobra.Command, args []string) error {
	sessionID := args[0]
	hub := buildHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	// Initial poll to find the agent
	hub.PollOnce(ctx)

	ch, err := hub.Stream(ctx, sessionID)
	if err != nil {
		return err
	}

	for entry := range ch {
		if followJSON || followRaw {
			data, _ := json.Marshal(entry)
			fmt.Println(string(data))
		} else {
			printFollowEntry(entry)
		}
	}
	return nil
}

func printFollowEntry(e agents.StreamEntry) {
	ts := e.Timestamp.Format("15:04:05")
	switch e.Role {
	case "user":
		fmt.Printf("\n[user] %s\n%s\n", ts, e.Text)
	case "assistant":
		model := ""
		if e.Model != "" {
			model = " " + e.Model
		}
		fmt.Printf("\n[assistant]%s %s\n%s\n", model, ts, e.Text)
	case "system":
		fmt.Printf("\n--- %s ---\n", e.Text)
	default:
		if e.ToolName != "" {
			fmt.Printf("\n[%s] %s %s\n", e.Role, e.ToolName, ts)
		} else {
			fmt.Printf("\n[%s] %s\n%s\n", e.Role, ts, e.Text)
		}
	}
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
