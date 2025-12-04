// Package retrieval provides availability detection for spec queries.
package retrieval

// AvailabilityDetector determines availability status based on search results.
type AvailabilityDetector struct {
	minConfidenceThreshold float64
	minSimilarityThreshold float64
}

// NewAvailabilityDetector creates a new availability detector with default thresholds.
func NewAvailabilityDetector(minConfidenceThreshold, minSimilarityThreshold float64) *AvailabilityDetector {
	if minConfidenceThreshold <= 0 {
		minConfidenceThreshold = 0.6 // Default: 60% confidence required for "found"
	}
	if minSimilarityThreshold <= 0 {
		minSimilarityThreshold = 0.5 // Default: 50% similarity required for "found"
	}
	return &AvailabilityDetector{
		minConfidenceThreshold: minConfidenceThreshold,
		minSimilarityThreshold: minSimilarityThreshold,
	}
}

// DetermineAvailability determines the availability status for a spec based on search results.
func (ad *AvailabilityDetector) DetermineAvailability(
	specName string,
	structuredFacts []SpecFact,
	semanticChunks []SemanticChunk,
) SpecAvailabilityStatus {
	status := SpecAvailabilityStatus{
		SpecName:        specName,
		MatchedSpecs:    structuredFacts,
		MatchedChunks:   semanticChunks,
		AlternativeNames: []string{},
	}

	// Calculate confidence from structured facts
	var maxFactConfidence float64
	if len(structuredFacts) > 0 {
		for _, fact := range structuredFacts {
			if fact.Confidence > maxFactConfidence {
				maxFactConfidence = fact.Confidence
			}
		}
	}

	// Calculate confidence from semantic chunks (similarity scores)
	var maxChunkSimilarity float64
	if len(semanticChunks) > 0 {
		for _, chunk := range semanticChunks {
			// Distance is inverse of similarity, so similarity = 1 - distance
			similarity := 1.0 - float64(chunk.Distance)
			if similarity > maxChunkSimilarity {
				maxChunkSimilarity = similarity
			}
		}
	}

	// Determine overall confidence (weighted average)
	// Structured facts get higher weight (0.6) than semantic chunks (0.4)
	var overallConfidence float64
	if len(structuredFacts) > 0 && len(semanticChunks) > 0 {
		overallConfidence = (maxFactConfidence * 0.6) + (maxChunkSimilarity * 0.4)
	} else if len(structuredFacts) > 0 {
		overallConfidence = maxFactConfidence
	} else if len(semanticChunks) > 0 {
		overallConfidence = maxChunkSimilarity
	}

	status.Confidence = overallConfidence

	// Determine status based on results and thresholds
	if len(structuredFacts) == 0 && len(semanticChunks) == 0 {
		// No results found after all search attempts
		status.Status = AvailabilityStatusUnavailable
		status.Confidence = 0.0
	} else if maxFactConfidence >= ad.minConfidenceThreshold || maxChunkSimilarity >= ad.minSimilarityThreshold {
		// Found with sufficient confidence
		status.Status = AvailabilityStatusFound
	} else if overallConfidence > 0.3 {
		// Found but with low confidence
		status.Status = AvailabilityStatusPartial
	} else {
		// Very low confidence, treat as unavailable
		status.Status = AvailabilityStatusUnavailable
	}

	return status
}

// DetermineAvailabilityWithThresholds allows custom thresholds per request.
func (ad *AvailabilityDetector) DetermineAvailabilityWithThresholds(
	specName string,
	structuredFacts []SpecFact,
	semanticChunks []SemanticChunk,
	customConfidenceThreshold float64,
	customSimilarityThreshold float64,
) SpecAvailabilityStatus {
	// Use custom thresholds if provided, otherwise use defaults
	confThreshold := customConfidenceThreshold
	if confThreshold <= 0 {
		confThreshold = ad.minConfidenceThreshold
	}
	simThreshold := customSimilarityThreshold
	if simThreshold <= 0 {
		simThreshold = ad.minSimilarityThreshold
	}

	status := SpecAvailabilityStatus{
		SpecName:        specName,
		MatchedSpecs:    structuredFacts,
		MatchedChunks:   semanticChunks,
		AlternativeNames: []string{},
	}

	// Calculate confidence from structured facts
	var maxFactConfidence float64
	if len(structuredFacts) > 0 {
		for _, fact := range structuredFacts {
			if fact.Confidence > maxFactConfidence {
				maxFactConfidence = fact.Confidence
			}
		}
	}

	// Calculate confidence from semantic chunks
	var maxChunkSimilarity float64
	if len(semanticChunks) > 0 {
		for _, chunk := range semanticChunks {
			similarity := 1.0 - float64(chunk.Distance)
			if similarity > maxChunkSimilarity {
				maxChunkSimilarity = similarity
			}
		}
	}

	// Determine overall confidence
	var overallConfidence float64
	if len(structuredFacts) > 0 && len(semanticChunks) > 0 {
		overallConfidence = (maxFactConfidence * 0.6) + (maxChunkSimilarity * 0.4)
	} else if len(structuredFacts) > 0 {
		overallConfidence = maxFactConfidence
	} else if len(semanticChunks) > 0 {
		overallConfidence = maxChunkSimilarity
	}

	status.Confidence = overallConfidence

	// Determine status with custom thresholds
	if len(structuredFacts) == 0 && len(semanticChunks) == 0 {
		status.Status = AvailabilityStatusUnavailable
		status.Confidence = 0.0
	} else if maxFactConfidence >= confThreshold || maxChunkSimilarity >= simThreshold {
		status.Status = AvailabilityStatusFound
	} else if overallConfidence > 0.3 {
		status.Status = AvailabilityStatusPartial
	} else {
		status.Status = AvailabilityStatusUnavailable
	}

	return status
}

