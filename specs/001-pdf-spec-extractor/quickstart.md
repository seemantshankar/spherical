# Quickstart: PDF Specification Extractor

## Prerequisites

- **Go 1.25.4+** (Latest Stable) installed.
- **CGO Enabled** (required for `go-fitz` PDF rendering).
  - Requires **MuPDF 1.24.9** headers/libraries installed on the system.
- **OpenRouter API Key**.

## Installation

```bash
go get github.com/spherical/pdf-extractor
```

## Configuration

Create a `.env` file in your project root:

```bash
OPENROUTER_API_KEY=sk-or-your-key-here
# Optional: Override model
# LLM_MODEL=google/gemini-2.5-pro
```

## Usage

### Basic CLI Usage

```bash
# Run the extraction
go run cmd/pdf-extractor/main.go process ./brochure.pdf --output ./specs.md
```

### Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/spherical/pdf-extractor/pkg/extractor"
)

func main() {
    // 1. Initialize the client
    client, err := extractor.NewClient()
    if err != nil {
        log.Fatal(err)
    }

    // 2. Start processing (returns a channel for streaming events)
    events, err := client.Process(context.Background(), "./brochure.pdf")
    if err != nil {
        log.Fatal(err)
    }

    // 3. Consume events
    for event := range events {
        switch event.Type {
        case extractor.EventPageProcessing:
            fmt.Printf("Processing page %d...\n", event.PageNumber)
        case extractor.EventLLMStreaming:
            // Print the raw text chunk from the LLM
            fmt.Print(event.Payload.(string))
        case extractor.EventComplete:
            fmt.Println("\nDone!")
        case extractor.EventError:
            log.Printf("Error: %v", event.Payload)
        }
    }
}
```

