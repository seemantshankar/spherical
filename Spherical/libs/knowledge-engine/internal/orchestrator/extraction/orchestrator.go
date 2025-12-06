// Package extraction provides PDF extraction orchestration.
package extraction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdfextractor "github.com/spherical/pdf-extractor/pkg/extractor"
)

// Orchestrator handles PDF extraction operations.
type Orchestrator struct {
	apiKey string
	model  string
}

// NewOrchestrator creates a new extraction orchestrator.
func NewOrchestrator(apiKey, model string) *Orchestrator {
	return &Orchestrator{
		apiKey: apiKey,
		model:  model,
	}
}

// ExtractResult represents the result of PDF extraction.
type ExtractResult struct {
	MarkdownPath string
	Metadata     *pdfextractor.DocumentMetadata
	Duration     time.Duration
}

// Extract extracts content from a PDF file.
func (o *Orchestrator) Extract(ctx context.Context, pdfPath string, outputPath string) (*ExtractResult, error) {
	startTime := time.Now()
	
	// Validate PDF file
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("PDF file not found: %s", pdfPath)
	}
	
	// Create extractor client
	config := &pdfextractor.Config{
		APIKey: o.apiKey,
		Model:  o.model,
	}
	
	client, err := pdfextractor.NewClientWithConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create extractor client: %w", err)
	}
	defer client.Close()
	
	// Determine output path
	if outputPath == "" {
		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		outputPath = baseName + "-specs.md"
	}
	
	// Process PDF
	events, err := client.Process(ctx, pdfPath)
	if err != nil {
		return nil, fmt.Errorf("process PDF: %w", err)
	}
	
	// Collect markdown and metadata
	var markdown strings.Builder
	var metadata *pdfextractor.DocumentMetadata
	
	for event := range events {
		switch event.Type {
		case pdfextractor.EventLLMStreaming:
			if chunk, ok := event.Payload.(string); ok {
				markdown.WriteString(chunk)
			}
		case pdfextractor.EventComplete:
			// Extract metadata if available
			if payload, ok := event.Payload.(*pdfextractor.CompletePayload); ok && payload.Metadata != nil {
				metadata = payload.Metadata
			}
		case pdfextractor.EventError:
			if errMsg, ok := event.Payload.(string); ok {
				return nil, fmt.Errorf("extraction error: %s", errMsg)
			}
		}
	}
	
	// If no metadata was found, create an empty one
	if metadata == nil {
		metadata = &pdfextractor.DocumentMetadata{
			Domain:      "Unknown",
			Subdomain:   "Unknown",
			CountryCode: "Unknown",
			ModelYear:   0,
			Condition:   "Unknown",
			Make:        "Unknown",
			Model:       "Unknown",
			Confidence:  0.0,
		}
	}
	
	// Write markdown to file
	if err := os.WriteFile(outputPath, []byte(markdown.String()), 0644); err != nil {
		return nil, fmt.Errorf("write markdown file: %w", err)
	}
	
	return &ExtractResult{
		MarkdownPath: outputPath,
		Metadata:     metadata,
		Duration:     time.Since(startTime),
	}, nil
}

