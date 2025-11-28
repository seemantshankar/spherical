package main

import (
	"fmt"
	"os"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <markdown_file>")
		os.Exit(1)
	}

	content, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	parser := ingest.NewParser(ingest.ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(string(content))
	if err != nil {
		fmt.Printf("Error parsing: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed USPs: %d\n", len(result.USPs))
	fmt.Println("\nUSP List:")
	for i, usp := range result.USPs {
		fmt.Printf("%d. %s\n", i+1, usp.Body)
		if i >= 50 {
			fmt.Printf("... (showing first 50)\n")
			break
		}
	}
}

