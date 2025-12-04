package retrieval

import (
	"testing"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestConfidenceCalculator_CalculateOverallConfidence_StructuredOnly(t *testing.T) {
	calc := NewConfidenceCalculator()

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.9,
		},
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.5",
			Confidence: 0.8,
		},
	}

	// Only structured facts, no chunks, no keyword confidence
	confidence := calc.CalculateOverallConfidence(facts, []SemanticChunk{}, 0.0)
	
	// Average of facts: (0.9 + 0.8) / 2 = 0.85
	// Weighted: 0.85 * 0.5 = 0.425
	assert.InDelta(t, 0.425, confidence, 0.01, "Confidence calculation mismatch")
}

func TestConfidenceCalculator_CalculateOverallConfidence_SemanticOnly(t *testing.T) {
	calc := NewConfidenceCalculator()

	chunks := []SemanticChunk{
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.2, // 0.8 similarity
		},
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.3, // 0.7 similarity
		},
	}

	// Only semantic chunks, no facts, no keyword confidence
	confidence := calc.CalculateOverallConfidence([]SpecFact{}, chunks, 0.0)
	
	// Average similarity: (0.8 + 0.7) / 2 = 0.75
	// Weighted: 0.75 * 0.3 = 0.225
	assert.InDelta(t, 0.225, confidence, 0.01, "Confidence calculation mismatch")
}

func TestConfidenceCalculator_CalculateOverallConfidence_KeywordOnly(t *testing.T) {
	calc := NewConfidenceCalculator()

	// Only keyword confidence, no facts, no chunks
	confidence := calc.CalculateOverallConfidence([]SpecFact{}, []SemanticChunk{}, 0.85)
	
	// Weighted: 0.85 * 0.2 = 0.17
	assert.InDelta(t, 0.17, confidence, 0.01, "Confidence calculation mismatch")
}

func TestConfidenceCalculator_CalculateOverallConfidence_AllSources(t *testing.T) {
	calc := NewConfidenceCalculator()

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.9,
		},
	}

	chunks := []SemanticChunk{
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.2, // 0.8 similarity
		},
	}

	keywordConf := 0.7

	// All sources: structured (0.9 * 0.5) + semantic (0.8 * 0.3) + keyword (0.7 * 0.2)
	// = 0.45 + 0.24 + 0.14 = 0.83
	confidence := calc.CalculateOverallConfidence(facts, chunks, keywordConf)
	
	assert.InDelta(t, 0.83, confidence, 0.01, "Confidence calculation mismatch")
}

func TestConfidenceCalculator_CalculateOverallConfidence_Clamping(t *testing.T) {
	calc := NewConfidenceCalculator()

	// Test that confidence is clamped to [0, 1]
	
	// Very high confidence (should be clamped to 1.0)
	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 2.0, // Invalid, but should be clamped
		},
	}
	confidence := calc.CalculateOverallConfidence(facts, []SemanticChunk{}, 0.0)
	assert.LessOrEqual(t, confidence, 1.0, "Confidence should be clamped to 1.0")

	// Negative confidence (should be clamped to 0.0)
	facts = []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: -0.5, // Invalid, but should be clamped
		},
	}
	confidence = calc.CalculateOverallConfidence(facts, []SemanticChunk{}, 0.0)
	assert.GreaterOrEqual(t, confidence, 0.0, "Confidence should be clamped to 0.0")
}

func TestConfidenceCalculator_CalculateConfidenceForResponse_WithSpecAvailability(t *testing.T) {
	calc := NewConfidenceCalculator()

	resp := &RetrievalResponse{
		SpecAvailability: []SpecAvailabilityStatus{
			{
				SpecName:   "Fuel Economy",
				Status:     AvailabilityStatusFound,
				Confidence: 0.9,
			},
			{
				SpecName:   "Ground Clearance",
				Status:     AvailabilityStatusFound,
				Confidence: 0.8,
			},
			{
				SpecName:   "Engine Torque",
				Status:     AvailabilityStatusUnavailable,
				Confidence: 0.0, // Should not be included
			},
		},
	}

	confidence := calc.CalculateConfidenceForResponse(resp)
	
	// Average of found specs: (0.9 + 0.8) / 2 = 0.85
	assert.InDelta(t, 0.85, confidence, 0.01, "Confidence calculation mismatch")
}

func TestConfidenceCalculator_CalculateConfidenceForResponse_NoFoundSpecs(t *testing.T) {
	calc := NewConfidenceCalculator()

	resp := &RetrievalResponse{
		SpecAvailability: []SpecAvailabilityStatus{
			{
				SpecName:   "Ground Clearance",
				Status:     AvailabilityStatusUnavailable,
				Confidence: 0.0,
			},
		},
	}

	confidence := calc.CalculateConfidenceForResponse(resp)
	
	// No found specs, should return 0.0
	assert.Equal(t, 0.0, confidence, "Confidence should be 0.0 when no specs found")
}

func TestConfidenceCalculator_CalculateConfidenceForResponse_FallbackToFactsAndChunks(t *testing.T) {
	calc := NewConfidenceCalculator()

	resp := &RetrievalResponse{
		StructuredFacts: []SpecFact{
			{
				Category:   "Fuel Efficiency",
				Name:       "Fuel Economy",
				Value:      "25.49",
				Confidence: 0.9,
			},
		},
		SemanticChunks: []SemanticChunk{
			{
				ChunkID:   uuid.New(),
				ChunkType: storage.ChunkTypeSpecRow,
				Distance:  0.2, // 0.8 similarity
			},
		},
		// No SpecAvailability, should fallback to facts/chunks
	}

	confidence := calc.CalculateConfidenceForResponse(resp)
	
	// Should calculate from facts and chunks
	// Structured: 0.9 * 0.5 = 0.45
	// Semantic: 0.8 * 0.3 = 0.24
	// Keyword (estimated from facts): 0.9 * 0.2 = 0.18
	// Total: 0.45 + 0.24 + 0.18 = 0.87
	assert.Greater(t, confidence, 0.8, "Confidence should be calculated from facts and chunks")
}

func TestConfidenceCalculator_CustomWeights(t *testing.T) {
	// Test with custom weights
	calc := NewConfidenceCalculatorWithWeights(0.7, 0.2, 0.1)

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.9,
		},
	}

	chunks := []SemanticChunk{
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.2, // 0.8 similarity
		},
	}

	keywordConf := 0.7

	// Custom weights: structured (0.9 * 0.7) + semantic (0.8 * 0.2) + keyword (0.7 * 0.1)
	// = 0.63 + 0.16 + 0.07 = 0.86
	confidence := calc.CalculateOverallConfidence(facts, chunks, keywordConf)
	
	assert.InDelta(t, 0.86, confidence, 0.01, "Confidence calculation mismatch with custom weights")
}

func TestConfidenceCalculator_WeightNormalization(t *testing.T) {
	// Test that weights are normalized to sum to 1.0
	calc := NewConfidenceCalculatorWithWeights(10.0, 5.0, 2.5) // Should normalize to ~0.588, 0.294, 0.118

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 1.0,
		},
	}

	// With normalized weights, confidence should still be in [0, 1]
	confidence := calc.CalculateOverallConfidence(facts, []SemanticChunk{}, 0.0)
	assert.GreaterOrEqual(t, confidence, 0.0)
	assert.LessOrEqual(t, confidence, 1.0)
}

func TestConfidenceCalculator_EmptyInputs(t *testing.T) {
	calc := NewConfidenceCalculator()

	// All empty inputs
	confidence := calc.CalculateOverallConfidence([]SpecFact{}, []SemanticChunk{}, 0.0)
	assert.Equal(t, 0.0, confidence, "Confidence should be 0.0 with no inputs")
}

func TestConfidenceCalculator_MultipleFactsAverage(t *testing.T) {
	calc := NewConfidenceCalculator()

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.6,
		},
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.5",
			Confidence: 0.8,
		},
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.4",
			Confidence: 1.0,
		},
	}

	// Average: (0.6 + 0.8 + 1.0) / 3 = 0.8
	// Weighted: 0.8 * 0.5 = 0.4
	confidence := calc.CalculateOverallConfidence(facts, []SemanticChunk{}, 0.0)
	assert.InDelta(t, 0.4, confidence, 0.01, "Should average multiple facts")
}

func TestConfidenceCalculator_MultipleChunksAverage(t *testing.T) {
	calc := NewConfidenceCalculator()

	chunks := []SemanticChunk{
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.1, // 0.9 similarity
		},
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.3, // 0.7 similarity
		},
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.5, // 0.5 similarity
		},
	}

	// Average similarity: (0.9 + 0.7 + 0.5) / 3 = 0.7
	// Weighted: 0.7 * 0.3 = 0.21
	confidence := calc.CalculateOverallConfidence([]SpecFact{}, chunks, 0.0)
	assert.InDelta(t, 0.21, confidence, 0.01, "Should average multiple chunks")
}

