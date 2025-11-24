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

// Process handles the complete extraction workflow
func (s *Service) Process(ctx context.Context, pdfPath string, eventCh chan<- domain.StreamEvent) error {
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
		return err
	}

	s.logger.Info("Converted %d pages", len(images))

	// Process each page sequentially
	var allMarkdown strings.Builder
	successCount := 0
	failCount := 0

	for _, image := range images {
		select {
		case <-ctx.Done():
			s.emitError(eventCh, ctx.Err())
			return ctx.Err()
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

	// Emit completion event
	duration := time.Since(startTime)
	s.emitEvent(eventCh, domain.StreamEvent{
		Type: domain.EventComplete,
		Payload: fmt.Sprintf("Extraction complete: %d/%d pages successful in %v",
			successCount, len(images), duration),
		Timestamp: time.Now(),
	})

	s.logger.Info("Extraction complete: %d successful, %d failed", successCount, failCount)

	if failCount == len(images) {
		return domain.ExtractionError("All pages failed to extract", nil)
	}

	return nil
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

