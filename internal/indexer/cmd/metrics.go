package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Show Prometheus metrics from the running indexer server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !rpc.ServerAvailable() {
			return fmt.Errorf("indexer server is not running (start it with 'thinkt-indexer server')")
		}

		resp, err := rpc.Call(rpc.MethodMetrics, nil, nil)
		if err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
		if !resp.OK {
			return fmt.Errorf("metrics: %s", resp.Error)
		}

		var out rpc.MetricsData
		if err := json.Unmarshal(resp.Data, &out); err != nil {
			return fmt.Errorf("parse metrics: %w", err)
		}

		_, _ = fmt.Fprint(os.Stdout, out.Text)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(metricsCmd)
}
