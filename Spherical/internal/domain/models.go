package domain

import "time"

// Document represents the source PDF file being processed
type Document struct {
	FilePath   string
	TotalPages int
}

// PageImage represents a single converted PDF page
type PageImage struct {
	PageNumber int
	ImagePath  string // Path to temporary JPG file
	Width      int
	Height     int
}

// Specification represents a single product specification item
type Specification struct {
	Category string `json:"category"` // e.g. "Dimensions", "Power"
	Name     string `json:"name"`
	Value    string `json:"value"`
}

// ExtractionResult contains the structured data extracted from pages
type ExtractionResult struct {
	Specifications []Specification `json:"specifications"`
	Features       []string        `json:"features"`
	USPs           []string        `json:"usps"`
	RawMarkdown    string          `json:"raw_markdown"` // Full markdown output from LLM
}

// EventType represents the type of stream event
type EventType string

const (
	EventStart          EventType = "start"
	EventPageProcessing EventType = "page_processing"
	EventLLMStreaming   EventType = "llm_streaming" // Chunk of text
	EventPageComplete   EventType = "page_complete"
	EventError          EventType = "error"
	EventComplete       EventType = "complete"
)

// StreamEvent represents an event emitted during processing
type StreamEvent struct {
	Type       EventType   `json:"type"`
	PageNumber int         `json:"page_number,omitempty"`
	Payload    interface{} `json:"payload,omitempty"` // Text chunk or status message
	Timestamp  time.Time   `json:"timestamp"`
}

// ProcessingStats contains metadata about the extraction execution
type ProcessingStats struct {
	TotalTime       time.Duration
	PagesProcessed  int
	SuccessfulPages int
	FailedPages     int
	Errors          []error
}

