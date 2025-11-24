# PDF Specification Extractor

A Go library and CLI tool to extract product specifications from PDF documents using vision-capable LLMs via OpenRouter.

## Features

- 🔍 **High-Quality PDF Conversion**: Converts PDF pages to high-quality JPG images (85%+ quality)
- 🤖 **AI-Powered Extraction**: Uses Gemini 2.5 Flash/Pro via OpenRouter for accurate extraction
- 📊 **Table Support**: Preserves complex table structures in Markdown output
- 🌊 **Streaming Support**: Real-time progress updates via event streaming
- 💪 **Robust Error Handling**: Automatic retries with exponential backoff for rate limits
- 🧹 **Memory Efficient**: Sequential page processing for low memory footprint
- 📚 **Library & CLI**: Use as a library in your Go code or as a standalone CLI tool

## Prerequisites

- **Go 1.25.4+** (Latest Stable)
- **CGO Enabled** (required for `go-fitz` PDF rendering)
  - Requires **MuPDF 1.24.9** libraries installed on your system
- **OpenRouter API Key** ([Get one here](https://openrouter.ai))

### Installing MuPDF

**macOS (via Homebrew)**:
```bash
brew install mupdf
```

**Ubuntu/Debian**:
```bash
sudo apt-get install libmupdf-dev
```

**Other platforms**: See [MuPDF installation guide](https://mupdf.com/docs/index.html)

## Installation

```bash
go get github.com/spherical/pdf-extractor
```

## Quick Start

### CLI Usage

1. **Set up your API key**:

Create a `.env` file in your project root:
```bash
OPENROUTER_API_KEY=sk-or-your-api-key-here

# Optional: Override default model
# LLM_MODEL=google/gemini-2.5-pro
```

2. **Run the extractor**:

```bash
go run cmd/pdf-extractor/main.go brochure.pdf
```

Or build and run:
```bash
go build -o pdf-extractor cmd/pdf-extractor/main.go
./pdf-extractor brochure.pdf
```

**Options**:
```bash
pdf-extractor [options] <pdf-file>

Options:
  -o, --output <file>   Output file path (default: <input-name>-specs.md)
  -v, --version         Show version information
  --verbose             Enable verbose logging

Examples:
  pdf-extractor brochure.pdf
  pdf-extractor -o specs.md brochure.pdf
  pdf-extractor --verbose brochure.pdf
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
    // 1. Create client (loads OPENROUTER_API_KEY from environment)
    client, err := extractor.NewClient()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // 2. Start processing (returns a channel for streaming events)
    events, err := client.Process(context.Background(), "./brochure.pdf")
    if err != nil {
        log.Fatal(err)
    }

    // 3. Consume events
    var markdown string
    for event := range events {
        switch event.Type {
        case extractor.EventPageProcessing:
            fmt.Printf("Processing page %d...\n", event.PageNumber)
            
        case extractor.EventLLMStreaming:
            // Accumulate markdown content
            chunk := event.Payload.(string)
            markdown += chunk
            fmt.Print(chunk) // Stream to console
            
        case extractor.EventComplete:
            fmt.Println("\n✓ Done!")
            
        case extractor.EventError:
            log.Printf("Error: %v", event.Payload)
        }
    }
    
    // 4. Save the result
    // ... save markdown to file ...
}
```

### Advanced Configuration

```go
// Use custom configuration
config := &extractor.Config{
    APIKey: "your-api-key",
    Model:  "google/gemini-2.5-pro", // Override default model
}

client, err := extractor.NewClientWithConfig(config)
```

## Event Types

The library streams events as processing progresses:

| Event Type | Description | Payload |
|------------|-------------|---------|
| `EventStart` | Processing started | Status message |
| `EventPageProcessing` | Started processing a page | Page number |
| `EventLLMStreaming` | LLM generated text chunk | String chunk |
| `EventPageComplete` | Finished processing a page | Page number |
| `EventError` | Error occurred | Error message |
| `EventComplete` | All processing complete | Summary message |

## Output Format

The extracted specifications are returned as structured Markdown:

```markdown
# Page 1

## Specifications
| Category | Specification | Value |
|----------|---------------|-------|
| Dimensions | Length | 3845 mm |
| Dimensions | Width | 1695 mm |
| Engine | Type | 1.2L Petrol |

## Key Features
- Dual front airbags
- ABS with EBD
- Smart Play Studio infotainment

## USPs (Unique Selling Points)
- Best-in-class fuel efficiency
- Spacious interior with boot space of 341L
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OPENROUTER_API_KEY` | OpenRouter API key (required) | - |
| `LLM_MODEL` | LLM model to use | `google/gemini-2.5-flash-preview-09-2025` |

### Supported Models

- `google/gemini-2.5-flash-preview-09-2025` (default, fast)
- `google/gemini-2.5-pro` (more accurate, slower)

## Architecture

```
pdf-extractor/
├── cmd/
│   └── pdf-extractor/       # CLI application
├── pkg/
│   └── extractor/           # Public API
├── internal/
│   ├── domain/              # Core models and interfaces
│   ├── pdf/                 # PDF conversion (go-fitz)
│   ├── llm/                 # OpenRouter API client
│   └── extract/             # Extraction orchestration
└── tests/
    ├── integration/         # Integration tests
    └── unit/                # Unit tests
```

## Error Handling

The library provides robust error handling:

- **Validation Errors**: Invalid file paths, non-PDF files
- **Conversion Errors**: PDF rendering failures
- **API Errors**: Network issues, rate limits (with automatic retry)
- **Extraction Errors**: LLM processing failures

All errors are wrapped with context using custom domain error types.

### Retry Logic

API requests automatically retry on:
- Rate limits (429)
- Server errors (500, 502, 503, 504)

With exponential backoff:
- Initial delay: 1 second
- Max delay: 30 seconds
- Max retries: 3

## Performance

- **Memory**: Sequential page processing keeps memory usage low (<500MB)
- **Speed**: ~2 minutes for a 20-page brochure (depends on API)
- **Quality**: 85% JPG quality for optimal OCR/vision accuracy

## Testing

```bash
# Run all tests
go test ./...

# Run only fast tests
go test -short ./...

# Run integration tests with real API
go test -v ./tests/integration/

# Run with coverage
go test -cover ./...

# Run performance tests
go test -v -run TestMemory ./tests/integration/
```

## Troubleshooting

### CGO/MuPDF Issues

If you get CGO or MuPDF-related errors:

1. Ensure MuPDF is installed: `brew install mupdf` (macOS)
2. Verify CGO is enabled: `go env CGO_ENABLED` should return `1`
3. Set CGO flags if needed:
   ```bash
   export CGO_CFLAGS="-I/usr/local/include"
   export CGO_LDFLAGS="-L/usr/local/lib -lmupdf"
   ```

### API Rate Limits

If you hit rate limits:
- The library automatically retries with exponential backoff
- Consider upgrading your OpenRouter plan
- Use the slower `gemini-2.5-flash` model which has higher limits

### Memory Issues

If processing large PDFs:
- Sequential processing should handle files up to 100+ pages
- Monitor with: `go test -v -run TestMemory ./tests/integration/`
- Temporary images are cleaned up automatically

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Submit a pull request

## License

[Add your license here]

## Acknowledgments

- **MuPDF** - PDF rendering engine
- **OpenRouter** - LLM API gateway
- **Google Gemini** - Vision-capable LLM models

## Support

- **Issues**: [GitHub Issues](https://github.com/spherical/pdf-extractor/issues)
- **Discussions**: [GitHub Discussions](https://github.com/spherical/pdf-extractor/discussions)

---

Built with ❤️ using Go and AI


