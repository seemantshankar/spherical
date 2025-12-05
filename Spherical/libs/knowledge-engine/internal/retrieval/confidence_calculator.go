// Package retrieval provides confidence calculation for retrieval results.
package retrieval

import (
	"math"
)

// ConfidenceCalculator calculates weighted confidence scores.
type ConfidenceCalculator struct {
	weights struct {
		structuredWeight float64
		semanticWeight   float64
		keywordWeight    float64
	}
}

// NewConfidenceCalculator creates a new confidence calculator with default weights.
func NewConfidenceCalculator() *ConfidenceCalculator {
	return &ConfidenceCalculator{
		weights: struct {
			structuredWeight float64
			semanticWeight   float64
			keywordWeight    float64
		}{
			structuredWeight: 0.5, // Structured facts are most reliable
			semanticWeight:   0.3, // Semantic similarity is good but less precise
			keywordWeight:    0.2, // Keyword matching is least reliable
		},
	}
}

// NewConfidenceCalculatorWithWeights creates a calculator with custom weights.
func NewConfidenceCalculatorWithWeights(structuredWeight, semanticWeight, keywordWeight float64) *ConfidenceCalculator {
	// Normalize weights to sum to 1.0
	total := structuredWeight + semanticWeight + keywordWeight
	if total > 0 {
		structuredWeight /= total
		semanticWeight /= total
		keywordWeight /= total
	}
	return &ConfidenceCalculator{
		weights: struct {
			structuredWeight float64
			semanticWeight   float64
			keywordWeight    float64
		}{
			structuredWeight: structuredWeight,
			semanticWeight:   semanticWeight,
			keywordWeight:    keywordWeight,
		},
	}
}

// CalculateOverallConfidence calculates overall confidence from multiple sources.
func (cc *ConfidenceCalculator) CalculateOverallConfidence(
	facts []SpecFact,
	chunks []SemanticChunk,
	keywordConfidence float64,
) float64 {
	var structuredConf float64
	var semanticConf float64

	// Calculate average confidence from structured facts
	if len(facts) > 0 {
		sum := 0.0
		for _, fact := range facts {
			sum += fact.Confidence
		}
		structuredConf = sum / float64(len(facts))
	}

	// Calculate average similarity from semantic chunks
	if len(chunks) > 0 {
		sum := 0.0
		for _, chunk := range chunks {
			// Distance is inverse of similarity
			similarity := 1.0 - float64(chunk.Distance)
			sum += similarity
		}
		semanticConf = sum / float64(len(chunks))
	}

	// Weighted combination
	overallConf := (structuredConf * cc.weights.structuredWeight) +
		(semanticConf * cc.weights.semanticWeight) +
		(keywordConfidence * cc.weights.keywordWeight)

	// Clamp to [0, 1]
	return math.Max(0.0, math.Min(1.0, overallConf))
}

// CalculateConfidenceForResponse calculates confidence for an entire retrieval response.
func (cc *ConfidenceCalculator) CalculateConfidenceForResponse(resp *RetrievalResponse) float64 {
	// If we have spec availability statuses, use those
	if len(resp.SpecAvailability) > 0 {
		sum := 0.0
		count := 0
		for _, status := range resp.SpecAvailability {
			if status.Status == AvailabilityStatusFound {
				sum += status.Confidence
				count++
			}
		}
		if count > 0 {
			return sum / float64(count)
		}
		// If no found specs, return 0
		return 0.0
	}

	// Fallback to calculating from facts and chunks
	keywordConf := 0.0
	if len(resp.StructuredFacts) > 0 {
		// Estimate keyword confidence from fact count and average confidence
		avgConf := 0.0
		for _, fact := range resp.StructuredFacts {
			avgConf += fact.Confidence
		}
		if len(resp.StructuredFacts) > 0 {
			avgConf /= float64(len(resp.StructuredFacts))
		}
		keywordConf = avgConf
	}

	return cc.CalculateOverallConfidence(resp.StructuredFacts, resp.SemanticChunks, keywordConf)
}



