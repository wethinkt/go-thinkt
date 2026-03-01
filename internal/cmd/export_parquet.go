package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/config"
)

var (
	parquetStorage string
	parquetOut     string
	parquetSince   string
	parquetUntil   string
)

var exportParquetCmd = &cobra.Command{
	Use:   "export-parquet",
	Short: "Export collected traces to Parquet files",
	Long: `Export collected traces from the DuckDB store to Parquet files.

This is a standalone operation that should be run when the collector is not running.
DuckDB handles Parquet encoding natively — no external library needed.

Examples:
  thinkt collect export-parquet
  thinkt collect export-parquet --out /tmp/parquet-export
  thinkt collect export-parquet --since 2025-01-01 --until 2025-02-01`,
	RunE: runExportParquet,
}

func parseTimeFlag(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	// Try RFC3339 first, then YYYY-MM-DD.
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", s)
}

func runExportParquet(cmd *cobra.Command, args []string) error {
	dbPath := parquetStorage
	if dbPath == "" {
		dir, err := config.Dir()
		if err != nil {
			return fmt.Errorf("resolve config dir: %w", err)
		}
		dbPath = filepath.Join(dir, "dbs", "collector.duckdb")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found: %s", dbPath)
	}

	outDir := parquetOut
	if outDir == "" {
		dir, err := config.Dir()
		if err != nil {
			return fmt.Errorf("resolve config dir: %w", err)
		}
		outDir = filepath.Join(dir, "exports", "parquet")
	}

	since, err := parseTimeFlag(parquetSince)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	until, err := parseTimeFlag(parquetUntil)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}

	opts := collect.ExportOptions{
		Since: since,
		Until: until,
	}

	// Open store directly — this is a standalone offline operation.
	store, err := collect.NewDuckDBStore(dbPath, collect.DefaultBatchSize, collect.DefaultFlushInterval)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	fmt.Fprintf(os.Stderr, "Exporting from %s to %s ...\n", dbPath, outDir)

	if err := store.ExportParquet(cmd.Context(), outDir, opts); err != nil {
		return fmt.Errorf("export: %w", err)
	}

	// Report output files.
	var totalSize int64
	var fileCount int
	_ = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".parquet" {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})

	fmt.Fprintf(os.Stderr, "Done: %d file(s), %.1f KB total\n", fileCount, float64(totalSize)/1024)
	return nil
}
