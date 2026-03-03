package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

var teamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "List and inspect agent teams",
	Long: `List and inspect multi-agent teams from supported sources.

Teams are groups of AI agents (team lead + teammates) that coordinate
via shared task boards and messaging to work on a project together.

Currently supported: Claude Code teams (~/.claude/teams/)

Examples:
  thinkt teams              # List all teams
  thinkt teams list         # Same as above
  thinkt teams --json       # Output as JSON`,
	RunE: runTeamsList,
}

var teamsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all discovered teams",
	RunE:  runTeamsList,
}

// Filter flags for teams command
var (
	teamsFilterActive   bool
	teamsFilterInactive bool
)

// teamSummary augments a team with computed activity info for display.
type teamSummary struct {
	thinkt.Team
	Active         bool   `json:"active"`
	TasksTotal     int    `json:"tasks_total"`
	TasksPending   int    `json:"tasks_pending"`
	TasksActive    int    `json:"tasks_active"`
	TasksCompleted int    `json:"tasks_completed"`
	LastActivity   string `json:"last_activity,omitempty"`
}

func runTeamsList(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
	ctx := context.Background()

	teamStores := registry.TeamStores()
	if len(teamStores) == 0 {
		fmt.Fprintln(os.Stderr, thinktI18n.T("cmd.teams.noSources", "No sources with team support found."))
		return nil
	}

	var summaries []teamSummary

	// Determine filter: if neither flag is set, show all
	showAll := !teamsFilterActive && !teamsFilterInactive

	for _, ts := range teamStores {
		teams, err := ts.ListTeams(ctx)
		if err != nil {
			continue
		}

		for _, team := range teams {
			// Apply status filter
			if !showAll {
				if teamsFilterActive && team.Status != thinkt.TeamStatusActive {
					continue
				}
				if teamsFilterInactive && team.Status != thinkt.TeamStatusInactive {
					continue
				}
			}
			s := buildTeamSummary(ctx, ts, team)
			summaries = append(summaries, s)
		}
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(summaries)
	}

	if len(summaries) == 0 {
		fmt.Println(thinktI18n.T("cmd.teams.noTeams", "No teams found."))
		return nil
	}

	th := theme.Current()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.GetAccent()))
	primaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.TextPrimary.Fg))
	secondaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.TextSecondary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.TextMuted.Fg))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.GetAccent()))

	const gap = 2
	colTeam := 4     // "TEAM"
	colSource := 6   // "SOURCE"
	colStatus := 6   // "STATUS"
	colMembers := 7  // "MEMBERS"
	colTasks := 5    // "TASKS"
	colCreated := 12 // "Jan 02 15:04"

	for _, s := range summaries {
		if len(s.Name) > colTeam {
			colTeam = len(s.Name)
		}
		if len(string(s.Source)) > colSource {
			colSource = len(string(s.Source))
		}
		status := string(s.Status)
		if status == "" {
			status = "active"
		}
		if len(status) > colStatus {
			colStatus = len(status)
		}
		members := fmt.Sprintf("%d", len(s.Members))
		if len(members) > colMembers {
			colMembers = len(members)
		}
		tasks := fmt.Sprintf("%d/%d/%d", s.TasksCompleted, s.TasksActive, s.TasksPending)
		if len(tasks) > colTasks {
			colTasks = len(tasks)
		}
	}
	colTeam += gap
	colSource += gap
	colStatus += gap
	colMembers += gap
	colTasks += gap
	colCreated += gap

	col := func(s lipgloss.Style, w int) lipgloss.Style { return s.Width(w) }

	fmt.Fprintf(os.Stdout, "%s%s%s%s%s%s%s\n",
		col(headerStyle, colTeam).Render(thinktI18n.T("cmd.teams.header.team", "TEAM")),
		col(headerStyle, colSource).Render(thinktI18n.T("common.header.source", "SOURCE")),
		col(headerStyle, colStatus).Render(thinktI18n.T("common.header.status", "STATUS")),
		col(headerStyle, colMembers).Render(thinktI18n.T("cmd.teams.header.members", "MEMBERS")),
		col(headerStyle, colTasks).Render(thinktI18n.T("cmd.teams.header.tasks", "TASKS")),
		col(headerStyle, colCreated).Render(thinktI18n.T("cmd.teams.header.created", "CREATED")),
		headerStyle.Render(thinktI18n.T("cmd.teams.header.lastActivity", "LAST ACTIVITY")))

	for _, s := range summaries {
		status := string(s.Status)
		if status == "" {
			status = "active"
		}

		tasks := fmt.Sprintf("%d/%d/%d",
			s.TasksCompleted, s.TasksActive, s.TasksPending)

		created := s.CreatedAt.Format("Jan 02 15:04")

		lastActivity := "-"
		if s.LastActivity != "" {
			lastActivity = s.LastActivity
		}

		statusStyle := mutedStyle
		if status == "active" {
			statusStyle = accentStyle
		}

		fmt.Fprintf(os.Stdout, "%s%s%s%s%s%s%s\n",
			col(primaryStyle, colTeam).Render(s.Name),
			col(secondaryStyle, colSource).Render(string(s.Source)),
			col(statusStyle, colStatus).Render(status),
			col(secondaryStyle, colMembers).Render(fmt.Sprintf("%d", len(s.Members))),
			col(primaryStyle, colTasks).Render(tasks),
			col(mutedStyle, colCreated).Render(created),
			mutedStyle.Render(lastActivity))
	}

	fmt.Println()
	fmt.Println(thinktI18n.T("cmd.teams.tasksLegend", "Tasks: completed/active/pending"))

	return nil
}

// buildTeamSummary computes activity info for a team by inspecting its tasks.
func buildTeamSummary(ctx context.Context, ts thinkt.TeamStore, team thinkt.Team) teamSummary {
	s := teamSummary{Team: team}

	tasks, err := ts.GetTeamTasks(ctx, team.Name)
	if err != nil {
		return s
	}

	var latestTaskTime time.Time

	for _, task := range tasks {
		if task.IsInternal {
			continue // skip agent lifecycle tasks
		}
		s.TasksTotal++
		switch task.Status {
		case "pending":
			s.TasksPending++
		case "in_progress":
			s.TasksActive++
		case "completed":
			s.TasksCompleted++
		}
	}

	// A team is active if it has in-progress tasks
	s.Active = s.TasksActive > 0

	// Compute last activity from team member join times and task timestamps
	for _, m := range team.Members {
		if m.JoinedAt.After(latestTaskTime) {
			latestTaskTime = m.JoinedAt
		}
	}

	if !latestTaskTime.IsZero() {
		s.LastActivity = thinktI18n.RelativeTime(latestTaskTime)
	}

	return s
}

