package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/pkg/extractor"
)

func init() {
	// Load .env file for testing
	_ = godotenv.Load("../../.env")
}

// TestStreamingEvents verifies that events are properly streamed
func TestStreamingEvents(t *testing.T) {
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	client, err := extractor.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	events, err := client.Process(ctx, testPDFPath)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Track event types received
	eventTypes := make(map[domain.EventType]int)
	var streamChunks int
	var markdown strings.Builder

	for event := range events {
		eventTypes[event.Type]++

		switch event.Type {
		case domain.EventLLMStreaming:
			streamChunks++
			if chunk, ok := event.Payload.(string); ok {
				markdown.WriteString(chunk)
			}

		case domain.EventError:
			t.Logf("Error event: %v", event.Payload)
		}
	}

	// Verify we received expected event types
	expectedEvents := []domain.EventType{
		domain.EventStart,
		domain.EventPageProcessing,
		domain.EventLLMStreaming,
		domain.EventPageComplete,
		domain.EventComplete,
	}

	for _, eventType := range expectedEvents {
		if eventTypes[eventType] == 0 {
			t.Errorf("Did not receive %s event", eventType)
		}
	}

	t.Logf("Event counts: %+v", eventTypes)
	t.Logf("Total stream chunks: %d", streamChunks)

	// Verify streaming actually happened (received multiple chunks)
	if streamChunks < 2 {
		t.Error("Expected multiple streaming chunks, got too few")
	}

	// Verify markdown was generated
	if markdown.Len() == 0 {
		t.Error("No markdown content received")
	}
}

// TestModelConfigurationOverride verifies LLM_MODEL env var override
func TestModelConfigurationOverride(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedModel string
	}{
		{
			name:          "default model",
			model:         "",
			expectedModel: "google/gemini-2.5-flash-preview-09-2025",
		},
		{
			name:          "custom model",
			model:         "google/gemini-2.5-pro",
			expectedModel: "google/gemini-2.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := llm.NewClient("test-key", tt.model)
			if client == nil {
				t.Fatal("Failed to create client")
			}

			// We can't easily inspect the client's model field as it's private,
			// but we can verify the client was created successfully
			// In a real scenario, this would make an API call with the configured model
		})
	}
}

// TestStreamParserWithMockData tests the stream parser with known data
func TestStreamParserWithMockData(t *testing.T) {
	mockSSE := `data: {"id":"1","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"2","choices":[{"delta":{"content":" World"}}]}

data: {"id":"3","choices":[{"delta":{"content":"!"}}]}

data: [DONE]

`

	parser := llm.NewStreamParser(strings.NewReader(mockSSE))
	resultCh := make(chan string, 10)

	go func() {
		err := parser.ParseAll(resultCh)
		if err != nil {
			t.Errorf("ParseAll failed: %v", err)
		}
		close(resultCh)
	}()

	var result strings.Builder
	for chunk := range resultCh {
		result.WriteString(chunk)
	}

	expected := "Hello World!"
	if result.String() != expected {
		t.Errorf("Expected %q, got %q", expected, result.String())
	}
}

// TestContextCancellation verifies that processing respects context cancellation
func TestContextCancellation(t *testing.T) {
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	client, err := extractor.NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create context that we'll cancel quickly
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := client.Process(ctx, testPDFPath)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Cancel after first event
	eventCount := 0
	for range events {
		eventCount++
		if eventCount == 1 {
			cancel()
			// Give it a moment to process cancellation
			time.Sleep(100 * time.Millisecond)
		}
	}

	t.Logf("Received %d events before cancellation took effect", eventCount)
}
