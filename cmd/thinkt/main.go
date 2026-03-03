// thinkt provides tools for exploring and extracting from AI coding assistant sessions.
package main

import (
	"fmt"
	"os"

	"github.com/wethinkt/go-thinkt/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
