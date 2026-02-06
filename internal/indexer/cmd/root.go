package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

var (
	dbPath  string
	verbose bool
	quiet   bool
)

var rootCmd = &cobra.Command{
	Use:   "thinkt-indexer",
	Short: "thinkt search and analytics",
	Long: `thinkt-indexer provides a specialized tool for indexing and searching 
AI assistant sessions using DuckDB.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	defaultDBPath, _ := db.DefaultPath()
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath, "path to DuckDB database file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress output")
}

func getDB() (*db.DB, error) {
	return db.Open(dbPath)
}
