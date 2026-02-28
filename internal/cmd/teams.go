package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		thinktI18n.T("cmd.teams.header.team", "TEAM"),
		thinktI18n.T("common.header.source", "SOURCE"),
		thinktI18n.T("common.header.status", "STATUS"),
		thinktI18n.T("cmd.teams.header.members", "MEMBERS"),
		thinktI18n.T("cmd.teams.header.tasks", "TASKS"),
		thinktI18n.T("cmd.teams.header.created", "CREATED"),
		thinktI18n.T("cmd.teams.header.lastActivity", "LAST ACTIVITY"))

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

		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			s.Name, s.Source, status, len(s.Members),
			tasks, created, lastActivity)
	}
	w.Flush()

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

