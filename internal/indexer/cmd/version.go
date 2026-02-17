package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/version"
)

var (
	versionJSON bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Run: func(cmd *cobra.Command, args []string) {
		info := version.GetInfo("thinkt-indexer")
		if versionJSON {
			_ = json.NewEncoder(os.Stdout).Encode(info) // Ignore encoding error
			return
		}
		fmt.Println(version.String("thinkt-indexer"))
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(versionCmd)
}