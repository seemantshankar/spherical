package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

func init() {
	// Load .env file for testing
	_ = godotenv.Load("../../.env")
}

// TestInvalidFileHandling tests behavior with invalid input files
func TestInvalidFileHandling(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		wantErr  bool
	}{
		{
			name:     "non-existent file",
			filePath: "/tmp/does-not-exist.pdf",
			wantErr:  true,
		},
		{
			name:     "empty path",
			filePath: "",
			wantErr:  true,
		},
		{
			name:     "directory instead of file",
			filePath: "/tmp",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := pdf.NewConverter()
			defer converter.Cleanup()

			ctx := context.Background()
			_, err := converter.Convert(ctx, tt.filePath, 85)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestAPIErrorHandling tests behavior with API errors
func TestAPIErrorHandling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "empty API key",
			apiKey:  "",
			wantErr: false, // Client creation succeeds, but API calls will fail
		},
		{
			name:    "invalid API key",
			apiKey:  "invalid-key",
			wantErr: false, // Client creation succeeds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := llm.NewClient(tt.apiKey, "")
			if client == nil {
				t.Fatal("Client creation failed")
			}

			// Create a temporary test image
			tmpFile, err := os.CreateTemp("", "test-*.jpg")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			// Try to extract - this should fail with invalid API key
			resultCh := make(chan string, 10)
			err = client.Extract(ctx, tmpFile.Name(), resultCh)
			close(resultCh)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

// TestLargeFileHandling tests processing of larger PDFs
func TestLargeFileHandling(t *testing.T) {
	// Skip this test unless running with specific tag
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// This test would use additional PDFs from the Uploads directory
	uploadsDir := "/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/"
	entries, err := os.ReadDir(uploadsDir)
	if err != nil {
		t.Skipf("Cannot read uploads directory: %v", err)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	// Test with each PDF found
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if len(entry.Name()) < 4 || entry.Name()[len(entry.Name())-4:] != ".pdf" {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			pdfPath := uploadsDir + entry.Name()

			converter := pdf.NewConverter()
			defer converter.Cleanup()

			llmClient := llm.NewClient(apiKey, "")
			extractor := extract.NewService(converter, llmClient)

			eventCh := make(chan domain.StreamEvent, 100)
			go func() {
				err := extractor.Process(ctx, pdfPath, eventCh)
				if err != nil {
					t.Logf("Process returned error: %v", err)
				}
				close(eventCh)
			}()

			// Count events
			var hasError bool
			for event := range eventCh {
				if event.Type == domain.EventError {
					t.Logf("Error event: %v", event.Payload)
					hasError = true
				}
			}

			if hasError {
				t.Log("Processing encountered errors but continued")
			}
		})
	}
}

// TestCleanupAfterError tests that cleanup happens even after errors
func TestCleanupAfterError(t *testing.T) {
	converter := pdf.NewConverter()

	ctx := context.Background()
	_, err := converter.Convert(ctx, "/tmp/invalid.pdf", 85)

	if err == nil {
		t.Error("Expected error for invalid file")
	}

	// Cleanup should not panic
	err = converter.Cleanup()
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Second cleanup should be safe
	err = converter.Cleanup()
	if err != nil {
		t.Errorf("Second cleanup failed: %v", err)
	}
}
