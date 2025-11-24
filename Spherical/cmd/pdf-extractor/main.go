package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

const (
	version = "1.0.0"
)

var (
	outputPath  string
	showVersion bool
	verbose     bool
)

func init() {
	flag.StringVar(&outputPath, "output", "", "Output file path (default: <input-name>-specs.md)")
	flag.StringVar(&outputPath, "o", "", "Output file path (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.Usage = usage
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Printf("pdf-extractor version %s\n", version)
		os.Exit(0)
	}

	// Check for input file
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: PDF file path required\n\n")
		usage()
		os.Exit(1)
	}

	pdfPath := flag.Arg(0)

	// Load environment variables
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	// Get API key
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: OPENROUTER_API_KEY environment variable not set\n")
		fmt.Fprintf(os.Stderr, "Please set it in your .env file or environment\n")
		os.Exit(1)
	}

	// Get optional model override
	model := os.Getenv("LLM_MODEL")

	// Set up logger
	logLevel := domain.LogLevelInfo
	if verbose {
		logLevel = domain.LogLevelDebug
	}
	logger := domain.NewLogger(logLevel)

	// Determine output path
	if outputPath == "" {
		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		outputPath = baseName + "-specs.md"
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\n\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Initialize components
	logger.Info("Initializing PDF extractor")
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, model)
	extractor := extract.NewService(converter, llmClient)

	// Create event channel
	eventCh := make(chan domain.StreamEvent, 100)

	// Start extraction in goroutine
	errCh := make(chan error, 1)
	go func() {
		err := extractor.Process(ctx, pdfPath, eventCh)
		close(eventCh)
		errCh <- err
	}()

	// Process events and display progress
	var markdown strings.Builder
	startTime := time.Now()

	fmt.Printf("Processing PDF: %s\n", pdfPath)
	fmt.Println(strings.Repeat("=", 60))

	for event := range eventCh {
		switch event.Type {
		case domain.EventStart:
			fmt.Printf("✓ %s\n", event.Payload)

		case domain.EventPageProcessing:
			fmt.Printf("\n📄 Processing page %d...\n", event.PageNumber)

		case domain.EventLLMStreaming:
			if chunk, ok := event.Payload.(string); ok {
				markdown.WriteString(chunk)
				if verbose {
					fmt.Print(chunk)
				} else {
					// Show spinner or progress indicator
					fmt.Print(".")
				}
			}

		case domain.EventPageComplete:
			if !verbose {
				fmt.Println() // New line after dots
			}
			fmt.Printf("✓ Page %d complete\n", event.PageNumber)

		case domain.EventError:
			fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", event.Payload)

		case domain.EventComplete:
			duration := time.Since(startTime)
			fmt.Println(strings.Repeat("=", 60))
			fmt.Printf("✓ %s\n", event.Payload)
			fmt.Printf("Total time: %v\n", duration.Round(time.Second))
		}
	}

	// Wait for extraction to complete
	if err := <-errCh; err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Extraction failed: %v\n", err)
		os.Exit(1)
	}

	// Write output file
	fmt.Printf("\nWriting output to: %s\n", outputPath)
	err := os.WriteFile(outputPath, []byte(markdown.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully extracted specifications to %s\n", outputPath)
}

func usage() {
	fmt.Fprintf(os.Stderr, `pdf-extractor - Extract product specifications from PDF documents

Usage:
  pdf-extractor [options] <pdf-file>

Options:
  -o, --output <file>   Output file path (default: <input-name>-specs.md)
  -v, --version         Show version information
  --verbose             Enable verbose logging

Environment Variables:
  OPENROUTER_API_KEY    OpenRouter API key (required)
  LLM_MODEL             Override default LLM model (optional)

Examples:
  pdf-extractor brochure.pdf
  pdf-extractor -o specs.md brochure.pdf
  pdf-extractor --verbose brochure.pdf

`)
}
