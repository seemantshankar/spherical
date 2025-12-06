package main

import (
	"fmt"
	"os"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/commands"
)

var (
	version = "0.1.0"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

