package main

import (
	"fmt"
	"os"

	_ "github.com/duckdb/duckdb-go/v2"
)

func main() {
	fmt.Println("thinkt-indexer")
	if len(os.Args) > 1 {
		fmt.Printf("Command received: %v\n", os.Args[1:])
	}
}
