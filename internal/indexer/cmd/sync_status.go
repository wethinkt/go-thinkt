package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var syncStatusJSON bool

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current sync/embedding status of the indexer server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !rpc.ServerAvailable() {
			if syncStatusJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"running": false,
					"state":   "stopped",
				})
			}
			fmt.Println(thinktI18n.T("indexer.syncStatus.notRunning", "Indexer server is not running."))
			return nil
		}

		resp, err := rpc.Call(rpc.MethodStatus, nil, nil)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		if !resp.OK {
			return fmt.Errorf("status: %s", resp.Error)
		}

		var status rpc.StatusData
		if err := json.Unmarshal(resp.Data, &status); err != nil {
			return fmt.Errorf("parse status: %w", err)
		}

		if syncStatusJSON {
			out := map[string]any{
				"running":        true,
				"state":          status.State,
				"uptime_seconds": status.UptimeSeconds,
				"watching":       status.Watching,
				"model":          status.Model,
				"model_dim":      status.ModelDim,
			}
			if status.SyncProgress != nil {
				out["sync_progress"] = status.SyncProgress
			}
			if status.EmbedProgress != nil {
				out["embed_progress"] = status.EmbedProgress
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}

		fmt.Print(thinktI18n.Tf("indexer.syncStatus.state", "State:   %s\n", status.State))
		fmt.Print(thinktI18n.Tf("indexer.syncStatus.uptime", "Uptime:  %ds\n", status.UptimeSeconds))
		fmt.Print(thinktI18n.Tf("indexer.syncStatus.watcher", "Watcher: %v\n", status.Watching))
		if status.Model != "" {
			fmt.Print(thinktI18n.Tf("indexer.syncStatus.model", "Model:   %s (%d dim)\n", status.Model, status.ModelDim))
		}
		if status.SyncProgress != nil {
			p := status.SyncProgress
			fmt.Printf("Sync:    ")
			if p.ProjectTotal > 0 {
				fmt.Printf("%d/%d projects  ", p.Project, p.ProjectTotal)
			}
			fmt.Printf("%d/%d sessions", p.Done, p.Total)
			if p.ProjectName != "" {
				fmt.Printf("  %s", p.ProjectName)
			}
			fmt.Println()
		}
		if status.EmbedProgress != nil {
			p := status.EmbedProgress
			fmt.Printf("Embed:   %d/%d sessions", p.Done, p.Total)
			if p.ChunksTotal > 0 {
				fmt.Printf("  %d/%d chunks", p.ChunksDone, p.ChunksTotal)
			}
			if p.SessionID != "" {
				sid := p.SessionID
				if len(sid) > 8 {
					sid = sid[:8]
				}
				fmt.Printf(" [%s]", sid)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	syncStatusCmd.Flags().BoolVar(&syncStatusJSON, "json", false, "Output as JSON")
	syncCmd.AddCommand(syncStatusCmd)
}
