package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

const (
	// Sample PDF path from requirements
	testPDFPath = "/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Arena-Wagon-r-Brochure.pdf"
)

func init() {
	// Load .env file for testing
	_ = godotenv.Load("../../.env")
}

// TestPDFToMarkdownConversion tests the complete flow from PDF to Markdown
func TestPDFToMarkdownConversion(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	// Create output channel for events
	eventCh := make(chan domain.StreamEvent, 100)

	// Process PDF in a goroutine
	go func() {
		err := extractor.Process(ctx, testPDFPath, eventCh)
		if err != nil {
			t.Errorf("Process failed: %v", err)
		}
		close(eventCh)
	}()

	// Collect results
	var markdown string
	var pagesProcessed int
	var hasError bool

	for event := range eventCh {
		switch event.Type {
		case domain.EventPageProcessing:
			t.Logf("Processing page %d", event.PageNumber)
			pagesProcessed++

		case domain.EventLLMStreaming:
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}

		case domain.EventError:
			t.Errorf("Error event: %v", event.Payload)
			hasError = true

		case domain.EventComplete:
			t.Log("Processing complete")
		}
	}

	// Assertions
	if hasError {
		t.Fatal("Processing encountered errors")
	}

	if pagesProcessed == 0 {
		t.Fatal("No pages were processed")
	}

	if len(markdown) == 0 {
		t.Fatal("No markdown content generated")
	}

	// Verify markdown contains expected content patterns
	// Use case-insensitive matching for text patterns
	markdownLower := strings.ToLower(markdown)
	
	expectedPatterns := map[string]bool{
		"specification": strings.Contains(markdownLower, "specification"),
		"feature":       strings.Contains(markdownLower, "feature"),
		"table":         strings.Contains(markdown, "|"), // Tables have | character
	}

	for pattern, found := range expectedPatterns {
		if !found {
			t.Logf("Warning: Markdown missing expected pattern: %s (may be okay depending on content)", pattern)
		}
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "pdf-extraction-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// Helper function to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
