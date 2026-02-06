package main

import (
	"os"

	"github.com/wethinkt/go-thinkt/internal/indexer/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}