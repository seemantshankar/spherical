package domain

import "context"

// Converter defines the interface for converting PDF to images
type Converter interface {
	// Convert turns a PDF into a slice of page images
	Convert(ctx context.Context, pdfPath string, quality int) ([]PageImage, error)

	// Cleanup removes temporary files created during conversion
	Cleanup() error
}

// Extractor defines the interface for extracting data from images
type Extractor interface {
	// Extract processes a single page image and returns the extraction result
	// StreamCallback is called with chunks of generated text if streamCh is provided
	Extract(ctx context.Context, image PageImage, streamCh chan<- StreamEvent) (*ExtractionResult, error)
}

// Pipeline orchestrates the conversion and extraction process
type Pipeline interface {
	// Process handles the complete workflow: convert PDF -> extract data -> stream events
	Process(ctx context.Context, pdfPath string) (<-chan StreamEvent, error)
}

