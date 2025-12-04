// Package retrieval provides batch processing for structured spec requests.
package retrieval

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BatchProcessor handles parallel processing of multiple spec requests.
type BatchProcessor struct {
	router      *Router
	maxWorkers  int
	timeout     time.Duration
	normalizer  *SpecNormalizer
	detector    *AvailabilityDetector
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(router *Router, maxWorkers int, timeout time.Duration) *BatchProcessor {
	if maxWorkers <= 0 {
		maxWorkers = 5 // Default: 5 concurrent workers
	}
	if timeout <= 0 {
		timeout = 30 * time.Second // Default: 30 second timeout
	}
	return &BatchProcessor{
		router:     router,
		maxWorkers: maxWorkers,
		timeout:    timeout,
		normalizer: NewSpecNormalizer(),
		detector:   NewAvailabilityDetector(0.6, 0.5), // Use default thresholds
	}
}

// ProcessSpecsInParallel processes multiple specs concurrently with rate limiting.
func (bp *BatchProcessor) ProcessSpecsInParallel(
	ctx context.Context,
	specs []string,
	req RetrievalRequest,
) ([]SpecAvailabilityStatus, error) {
	if len(specs) == 0 {
		return []SpecAvailabilityStatus{}, nil
	}

	// Create context with timeout
	processCtx, cancel := context.WithTimeout(ctx, bp.timeout)
	defer cancel()

	// Worker pool pattern
	type workItem struct {
		index    int
		specName string
	}

	workChan := make(chan workItem, len(specs))
	results := make([]SpecAvailabilityStatus, len(specs))
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Send work items
	for i, spec := range specs {
		workChan <- workItem{index: i, specName: spec}
	}
	close(workChan)

	// Start workers
	for i := 0; i < bp.maxWorkers && i < len(specs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workChan {
				// Process single spec
				status := bp.processSingleSpec(processCtx, item.specName, req)

				// Store result
				mu.Lock()
				results[item.index] = status
				mu.Unlock()
			}
		}()
	}

	// Wait for all workers to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// All workers completed
	case <-processCtx.Done():
		// Timeout occurred
		return results, fmt.Errorf("batch processing timeout after %v", bp.timeout)
	}

	return results, nil
}

// processSingleSpec processes a single spec name.
func (bp *BatchProcessor) processSingleSpec(
	ctx context.Context,
	specName string,
	req RetrievalRequest,
) SpecAvailabilityStatus {
	// Normalize spec name
	canonical, alternatives := bp.normalizer.NormalizeSpecName(specName)

	// Create a request for this specific spec
	specReq := req
	specReq.Question = canonical // Use canonical name as question
	specReq.RequestedSpecs = nil  // Clear to use natural language path

	// Try structured keyword search first
	facts, confidence, err := bp.router.queryStructuredSpecs(ctx, specReq)
	if err != nil {
		// Log error but continue
		confidence = 0.0
	}

	// Try vector search if low confidence
	var chunks []SemanticChunk
	if confidence < bp.router.config.KeywordConfidenceThreshold {
		chunks, err = bp.router.querySemanticChunks(ctx, specReq)
		if err != nil {
			// Log error but continue
		}
	}

	// Determine availability
	status := bp.detector.DetermineAvailability(canonical, facts, chunks)
	status.SpecName = specName // Use original spec name
	status.AlternativeNames = alternatives

	return status
}

