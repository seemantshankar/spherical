package extractor

import (
	"context"
	"os"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

// Re-export event types for public API
type (
	StreamEvent      = domain.StreamEvent
	EventType        = domain.EventType
	DocumentMetadata = domain.DocumentMetadata // FR-016: Document categorization metadata
)

// Re-export processing types from extract package
type (
	CompletePayload = extract.CompletePayload // FR-016: EventComplete payload with metadata
	ProcessResult   = extract.ProcessResult   // FR-016: Processing result with metadata
)

// Event type constants
const (
	EventStart          = domain.EventStart
	EventPageProcessing = domain.EventPageProcessing
	EventLLMStreaming   = domain.EventLLMStreaming
	EventPageComplete   = domain.EventPageComplete
	EventError          = domain.EventError
	EventComplete       = domain.EventComplete
)

// Client is the main entry point for the PDF extractor library
type Client struct {
	service   *extract.Service
	converter *pdf.Converter
}

// Config holds configuration options for the client
type Config struct {
	APIKey string // OpenRouter API key
	Model  string // Optional: LLM model override
}

// NewClient creates a new extractor client
func NewClient() (*Client, error) {
	// Load environment variables
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, domain.ConfigError("OPENROUTER_API_KEY not set", nil)
	}

	model := os.Getenv("LLM_MODEL")

	return NewClientWithConfig(&Config{
		APIKey: apiKey,
		Model:  model,
	})
}

// NewClientWithConfig creates a new extractor client with custom configuration
func NewClientWithConfig(config *Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, domain.ConfigError("API key is required", nil)
	}

	// Initialize components
	converter := pdf.NewConverter()
	llmClient := llm.NewClient(config.APIKey, config.Model)
	service := extract.NewService(converter, llmClient)

	return &Client{
		service:   service,
		converter: converter,
	}, nil
}

// Process extracts specifications from a PDF file
// Returns a channel that streams events as extraction progresses
func (c *Client) Process(ctx context.Context, pdfPath string) (<-chan StreamEvent, error) {
	// Validate input
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return nil, domain.ValidationError("PDF file not found", err)
	}

	// Create event channel
	eventCh := make(chan StreamEvent, 100)

	// Start processing in goroutine
	go func() {
		defer close(eventCh)
		err := c.service.Process(ctx, pdfPath, eventCh)
		if err != nil {
			// Emit error event if processing fails
			eventCh <- StreamEvent{
				Type:    EventError,
				Payload: err.Error(),
			}
		}
	}()

	return eventCh, nil
}

// Close cleans up resources
func (c *Client) Close() error {
	return c.converter.Cleanup()
}




