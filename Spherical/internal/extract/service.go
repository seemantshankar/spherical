package extract

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spherical/pdf-extractor/internal/domain"
)

// LLMClient defines the interface for LLM operations
type LLMClient interface {
	Extract(ctx context.Context, imagePath string, resultCh chan<- string) error
	DetectCategorization(ctx context.Context, pageImages []domain.PageImage) (*domain.DocumentMetadata, error)
	DetectCategorizationWithMajorityVote(ctx context.Context, pageImages []domain.PageImage) (*domain.DocumentMetadata, error)
}

// Service orchestrates the PDF extraction process
type Service struct {
	converter domain.Converter
	llm       LLMClient
	logger    *domain.Logger
}

// NewService creates a new extraction service
func NewService(converter domain.Converter, llm LLMClient) *Service {
	return &Service{
		converter: converter,
		llm:       llm,
		logger:    domain.DefaultLogger.WithPrefix("extract"),
	}
}

// ProcessResult contains the results of document processing (FR-016)
type ProcessResult struct {
	Markdown string
	Metadata *domain.DocumentMetadata
	Stats    ProcessStats
}

// ProcessStats contains processing statistics
type ProcessStats struct {
	TotalPages      int
	SuccessfulPages int
	FailedPages     int
	Duration        time.Duration
}

// Process handles the complete extraction workflow
func (s *Service) Process(ctx context.Context, pdfPath string, eventCh chan<- domain.StreamEvent) error {
	_, err := s.ProcessWithResult(ctx, pdfPath, eventCh)
	return err
}

// ProcessWithResult handles the complete extraction workflow and returns results with metadata (FR-016)
func (s *Service) ProcessWithResult(ctx context.Context, pdfPath string, eventCh chan<- domain.StreamEvent) (*ProcessResult, error) {
	startTime := time.Now()

	// Emit start event
	s.emitEvent(eventCh, domain.StreamEvent{
		Type:      domain.EventStart,
		Payload:   fmt.Sprintf("Starting extraction of %s", pdfPath),
		Timestamp: time.Now(),
	})

	// Convert PDF to images
	s.logger.Info("Converting PDF to images: %s", pdfPath)
	images, err := s.converter.Convert(ctx, pdfPath, 85)
	if err != nil {
		s.emitError(eventCh, err)
		return nil, err
	}

	s.logger.Info("Converted %d pages", len(images))

	// Detect document categorization (T609 - FR-016)
	// Analyze cover page (fallback to first page if cover blank/unreadable,
	// then sequentially to subsequent pages until clear categorization is found)
	metadata := s.detectCategorization(ctx, images)
	s.logger.Info("Document categorization detected: Domain=%s, Make=%s, Model=%s",
		metadata.Domain, metadata.Make, metadata.Model)

	// Process each page sequentially
	var allMarkdown strings.Builder

	// Prepend categorization header (FR-016)
	categorizationHeader := formatCategorizationHeader(metadata)
	allMarkdown.WriteString(categorizationHeader)

	// Emit categorization header as first streaming content
	s.emitEvent(eventCh, domain.StreamEvent{
		Type:       domain.EventLLMStreaming,
		PageNumber: 0,
		Payload:    categorizationHeader,
		Timestamp:  time.Now(),
	})

	successCount := 0
	failCount := 0

	for _, image := range images {
		select {
		case <-ctx.Done():
			s.emitError(eventCh, ctx.Err())
			return nil, ctx.Err()
		default:
		}

		// Emit page processing event
		s.emitEvent(eventCh, domain.StreamEvent{
			Type:       domain.EventPageProcessing,
			PageNumber: image.PageNumber,
			Payload:    fmt.Sprintf("Processing page %d", image.PageNumber),
			Timestamp:  time.Now(),
		})

		s.logger.Info("Processing page %d", image.PageNumber)

		// Emit page separator before starting extraction
		pageSeparator := fmt.Sprintf("\n\n# Page %d\n\n", image.PageNumber)
		s.emitEvent(eventCh, domain.StreamEvent{
			Type:       domain.EventLLMStreaming,
			PageNumber: image.PageNumber,
			Payload:    pageSeparator,
			Timestamp:  time.Now(),
		})

		// Extract data from page
		pageMarkdown, err := s.extractPage(ctx, image, eventCh)
		if err != nil {
			s.logger.Error("Failed to extract page %d: %v", image.PageNumber, err)
			failCount++
			s.emitError(eventCh, fmt.Errorf("page %d: %w", image.PageNumber, err))
			continue
		}

		// Aggregate markdown (for internal tracking, though CLI uses events)
		allMarkdown.WriteString(pageSeparator)
		allMarkdown.WriteString(pageMarkdown)

		successCount++

		// Emit page complete event
		s.emitEvent(eventCh, domain.StreamEvent{
			Type:       domain.EventPageComplete,
			PageNumber: image.PageNumber,
			Payload:    fmt.Sprintf("Completed page %d", image.PageNumber),
			Timestamp:  time.Now(),
		})
	}

	duration := time.Since(startTime)

	// Build process result
	result := &ProcessResult{
		Markdown: allMarkdown.String(),
		Metadata: metadata,
		Stats: ProcessStats{
			TotalPages:      len(images),
			SuccessfulPages: successCount,
			FailedPages:     failCount,
			Duration:        duration,
		},
	}

	// Emit completion event with metadata (T610 - FR-016)
	s.emitEvent(eventCh, domain.StreamEvent{
		Type: domain.EventComplete,
		Payload: &CompletePayload{
			Message: fmt.Sprintf("Extraction complete: %d/%d pages successful in %v",
				successCount, len(images), duration),
			Metadata: metadata,
			Stats:    result.Stats,
		},
		Timestamp: time.Now(),
	})

	s.logger.Info("Extraction complete: %d successful, %d failed", successCount, failCount)

	if failCount == len(images) {
		return nil, domain.ExtractionError("All pages failed to extract", nil)
	}

	return result, nil
}

// CompletePayload contains the payload for EventComplete (FR-016)
type CompletePayload struct {
	Message  string                  `json:"message"`
	Metadata *domain.DocumentMetadata `json:"metadata"`
	Stats    ProcessStats            `json:"stats"`
}

// detectCategorization detects document metadata from page images (T609)
// Implements sequential page fallback: cover page → first page → subsequent pages
func (s *Service) detectCategorization(ctx context.Context, images []domain.PageImage) *domain.DocumentMetadata {
	if len(images) == 0 {
		s.logger.Warn("No images to detect categorization from")
		return domain.NewDocumentMetadata()
	}

	// Try categorization detection
	metadata, err := s.llm.DetectCategorization(ctx, images)
	if err != nil {
		s.logger.Warn("Categorization detection failed: %v", err)
		return domain.NewDocumentMetadata()
	}

	// If we have multiple pages and need conflict resolution, use majority vote (T611)
	if len(images) > 1 && !metadata.IsValid() {
		s.logger.Info("Initial categorization unclear, trying majority vote")
		metadata, err = s.llm.DetectCategorizationWithMajorityVote(ctx, images)
		if err != nil {
			s.logger.Warn("Majority vote categorization failed: %v", err)
			return domain.NewDocumentMetadata()
		}
	}

	// Log confidence scores (T622)
	s.logger.Info("Categorization confidence: %.2f", metadata.Confidence)
	if metadata.Domain == "Unknown" {
		s.logger.Warn("Domain field marked as Unknown (low confidence or not detected)")
	}
	if metadata.Make == "Unknown" {
		s.logger.Warn("Make field marked as Unknown (low confidence or not detected)")
	}
	if metadata.Model == "Unknown" {
		s.logger.Warn("Model field marked as Unknown (low confidence or not detected)")
	}

	return metadata
}

// formatCategorizationHeader generates YAML frontmatter for categorization (T613 - FR-016)
func formatCategorizationHeader(metadata *domain.DocumentMetadata) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("domain: %s\n", metadata.Domain))
	sb.WriteString(fmt.Sprintf("subdomain: %s\n", metadata.Subdomain))
	sb.WriteString(fmt.Sprintf("country_code: %s\n", metadata.CountryCode))
	if metadata.ModelYear > 0 {
		sb.WriteString(fmt.Sprintf("model_year: %d\n", metadata.ModelYear))
	} else {
		sb.WriteString("model_year: Unknown\n")
	}
	sb.WriteString(fmt.Sprintf("condition: %s\n", metadata.Condition))
	sb.WriteString(fmt.Sprintf("make: %s\n", metadata.Make))
	sb.WriteString(fmt.Sprintf("model: %s\n", metadata.Model))
	sb.WriteString("---\n\n")

	return sb.String()
}

// extractPage extracts data from a single page image
func (s *Service) extractPage(ctx context.Context, image domain.PageImage, eventCh chan<- domain.StreamEvent) (string, error) {
	// Create result channel for LLM streaming
	resultCh := make(chan string, 100)
	errCh := make(chan error, 1)

	// Start LLM extraction in goroutine
	go func() {
		err := s.llm.Extract(ctx, image.ImagePath, resultCh)
		if err != nil {
			errCh <- err
		}
		close(resultCh)
		close(errCh)
	}()

	// Collect and stream results
	var markdown strings.Builder

	for {
		select {
		case chunk, ok := <-resultCh:
			if !ok {
				// Channel closed, check for errors
				select {
				case err := <-errCh:
					if err != nil {
						return "", err
					}
				default:
				}
				return markdown.String(), nil
			}

			// Emit streaming event
			s.emitEvent(eventCh, domain.StreamEvent{
				Type:       domain.EventLLMStreaming,
				PageNumber: image.PageNumber,
				Payload:    chunk,
				Timestamp:  time.Now(),
			})

			markdown.WriteString(chunk)

		case err := <-errCh:
			if err != nil {
				return "", err
			}

		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// emitEvent safely emits an event to the channel
func (s *Service) emitEvent(eventCh chan<- domain.StreamEvent, event domain.StreamEvent) {
	if eventCh != nil {
		select {
		case eventCh <- event:
		default:
			s.logger.Warn("Event channel full, dropping event: %s", event.Type)
		}
	}
}

// emitError emits an error event
func (s *Service) emitError(eventCh chan<- domain.StreamEvent, err error) {
	s.emitEvent(eventCh, domain.StreamEvent{
		Type:      domain.EventError,
		Payload:   err.Error(),
		Timestamp: time.Now(),
	})
}

