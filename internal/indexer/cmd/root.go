package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

var (
	dbPath    string
	embDBPath string
	logPath   string
	verbose   bool
	quiet     bool
)

var rootCmd = &cobra.Command{
	Use:          "thinkt-indexer",
	Short:        "DuckDB-powered indexer for thinkt",
	SilenceUsage: true, // Don't show usage on RunE errors
	Long: `thinkt-indexer provides a specialized tool for indexing and searching
AI assistant sessions using DuckDB.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if logPath != "" {
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			// Redirect both log output and stderr to the log file.
			// This captures panics, runtime errors, and fmt.Fprintf(os.Stderr, ...).
			log.SetOutput(f)
			os.Stderr = f
		}
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	defaultDBPath, _ := db.DefaultPath()
	defaultEmbDBPath, _ := db.DefaultEmbeddingsPath()
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath, "path to DuckDB index database file")
	rootCmd.PersistentFlags().StringVar(&embDBPath, "embeddings-db", defaultEmbDBPath, "path to DuckDB embeddings database file")
	rootCmd.PersistentFlags().StringVar(&logPath, "log", "", "path to log file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress output")
}

func getDB() (*db.DB, error) {
	return db.Open(dbPath)
}

func getReadOnlyDB() (*db.DB, error) {
	return db.OpenReadOnly(dbPath)
}

func getEmbeddingsDB() (*db.DB, error) {
	return db.OpenEmbeddings(embDBPath)
}

func getReadOnlyEmbeddingsDB() (*db.DB, error) {
	return db.OpenReadOnly(embDBPath)
}