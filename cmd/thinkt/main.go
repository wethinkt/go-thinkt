// thinkt provides tools for exploring and extracting from AI coding assistant sessions.
package main

import (
	"os"

	"github.com/wethinkt/go-thinkt/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
