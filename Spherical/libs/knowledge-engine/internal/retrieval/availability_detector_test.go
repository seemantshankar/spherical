package retrieval

import (
	"testing"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestAvailabilityDetector_DetermineAvailability_Found(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	tests := []struct {
		name           string
		facts          []SpecFact
		chunks         []SemanticChunk
		expectedStatus AvailabilityStatus
		minConfidence  float64
	}{
		{
			name: "High confidence structured fact",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Unit:       "km/l",
					Confidence: 0.95,
				},
			},
			chunks:         []SemanticChunk{},
			expectedStatus: AvailabilityStatusFound,
			minConfidence:  0.9,
		},
		{
			name: "High similarity semantic chunk",
			facts: []SpecFact{},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Text:      "Fuel Economy: 25.49 km/l",
					Distance:  0.1, // High similarity (1.0 - 0.1 = 0.9)
				},
			},
			expectedStatus: AvailabilityStatusFound,
			minConfidence:  0.8,
		},
		{
			name: "Both facts and chunks with high confidence",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.85,
				},
			},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.2, // 0.8 similarity
				},
			},
			expectedStatus: AvailabilityStatusFound,
			minConfidence:  0.8,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.DetermineAvailability("Fuel Economy", tc.facts, tc.chunks)
			assert.Equal(t, tc.expectedStatus, status.Status, "Status mismatch")
			assert.GreaterOrEqual(t, status.Confidence, tc.minConfidence, "Confidence too low")
			assert.Equal(t, len(tc.facts), len(status.MatchedSpecs), "Facts count mismatch")
			assert.Equal(t, len(tc.chunks), len(status.MatchedChunks), "Chunks count mismatch")
		})
	}
}

func TestAvailabilityDetector_DetermineAvailability_Unavailable(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	tests := []struct {
		name           string
		facts          []SpecFact
		chunks         []SemanticChunk
		expectedStatus AvailabilityStatus
	}{
		{
			name:           "No results",
			facts:          []SpecFact{},
			chunks:         []SemanticChunk{},
			expectedStatus: AvailabilityStatusUnavailable,
		},
		{
			name:  "Very low confidence fact",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.2, // Below threshold
				},
			},
			chunks:         []SemanticChunk{},
			expectedStatus: AvailabilityStatusUnavailable,
		},
		{
			name:  "Very low similarity chunk",
			facts: []SpecFact{},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.9, // Very low similarity (1.0 - 0.9 = 0.1)
				},
			},
			expectedStatus: AvailabilityStatusUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.DetermineAvailability("Ground Clearance", tc.facts, tc.chunks)
			assert.Equal(t, tc.expectedStatus, status.Status, "Status mismatch")
			assert.LessOrEqual(t, status.Confidence, 0.3, "Confidence should be low for unavailable")
		})
	}
}

func TestAvailabilityDetector_DetermineAvailability_Partial(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	tests := []struct {
		name           string
		facts          []SpecFact
		chunks         []SemanticChunk
		expectedStatus AvailabilityStatus
	}{
		{
			name: "Medium confidence fact",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.5, // Below found threshold but above unavailable
				},
			},
			chunks:         []SemanticChunk{},
			expectedStatus: AvailabilityStatusPartial,
		},
		{
			name:  "Medium similarity chunk",
			facts: []SpecFact{},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.6, // Medium similarity (1.0 - 0.6 = 0.4), below 0.5 threshold
				},
			},
			expectedStatus: AvailabilityStatusPartial,
		},
		{
			name: "Mixed low confidence results",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.4,
				},
			},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.6, // 0.4 similarity
				},
			},
			expectedStatus: AvailabilityStatusPartial,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.DetermineAvailability("Fuel Economy", tc.facts, tc.chunks)
			assert.Equal(t, tc.expectedStatus, status.Status, "Status mismatch")
			assert.Greater(t, status.Confidence, 0.3, "Confidence should be above unavailable threshold")
			assert.Less(t, status.Confidence, 0.6, "Confidence should be below found threshold")
		})
	}
}

func TestAvailabilityDetector_DetermineAvailabilityWithThresholds(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	tests := []struct {
		name                    string
		facts                   []SpecFact
		chunks                  []SemanticChunk
		customConfThreshold     float64
		customSimThreshold      float64
		expectedStatus          AvailabilityStatus
	}{
		{
			name: "Custom higher threshold - should be partial",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.7, // Would be found with default, but partial with custom
				},
			},
			chunks:              []SemanticChunk{},
			customConfThreshold: 0.8,
			customSimThreshold:  0.8,
			expectedStatus:      AvailabilityStatusPartial,
		},
		{
			name: "Custom lower threshold - should be found",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.5, // Would be partial with default, but found with custom
				},
			},
			chunks:              []SemanticChunk{},
			customConfThreshold: 0.4,
			customSimThreshold:  0.4,
			expectedStatus:      AvailabilityStatusFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.DetermineAvailabilityWithThresholds(
				"Fuel Economy",
				tc.facts,
				tc.chunks,
				tc.customConfThreshold,
				tc.customSimThreshold,
			)
			assert.Equal(t, tc.expectedStatus, status.Status, "Status mismatch with custom thresholds")
		})
	}
}

func TestAvailabilityDetector_ConfidenceCalculation(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	tests := []struct {
		name           string
		facts           []SpecFact
		chunks          []SemanticChunk
		expectedMinConf float64
		expectedMaxConf float64
	}{
		{
			name: "Only facts - confidence should match fact confidence",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.85,
				},
			},
			chunks:          []SemanticChunk{},
			expectedMinConf: 0.84,
			expectedMaxConf: 0.86,
		},
		{
			name:  "Only chunks - confidence should match similarity",
			facts: []SpecFact{},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.2, // 0.8 similarity
				},
			},
			expectedMinConf: 0.79,
			expectedMaxConf: 0.81,
		},
		{
			name: "Both facts and chunks - weighted average",
			facts: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Confidence: 0.8, // Weight: 0.6
				},
			},
			chunks: []SemanticChunk{
				{
					ChunkType: storage.ChunkTypeSpecRow,
					Distance:  0.3, // 0.7 similarity, Weight: 0.4
				},
			},
			// Expected: (0.8 * 0.6) + (0.7 * 0.4) = 0.48 + 0.28 = 0.76
			expectedMinConf: 0.75,
			expectedMaxConf: 0.77,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.DetermineAvailability("Fuel Economy", tc.facts, tc.chunks)
			assert.GreaterOrEqual(t, status.Confidence, tc.expectedMinConf, "Confidence too low")
			assert.LessOrEqual(t, status.Confidence, tc.expectedMaxConf, "Confidence too high")
		})
	}
}

func TestAvailabilityDetector_DefaultThresholds(t *testing.T) {
	// Test with zero thresholds (should use defaults)
	detector := NewAvailabilityDetector(0, 0)

	status := detector.DetermineAvailability("Fuel Economy", []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.7, // Above default 0.6 threshold
		},
	}, []SemanticChunk{})

	assert.Equal(t, AvailabilityStatusFound, status.Status, "Should use default threshold of 0.6")
}

func TestAvailabilityDetector_MultipleFacts(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.5,
		},
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.5",
			Confidence: 0.9, // Higher confidence
		},
	}

	status := detector.DetermineAvailability("Fuel Economy", facts, []SemanticChunk{})
	
	// Should use the maximum confidence (0.9)
	assert.Equal(t, AvailabilityStatusFound, status.Status)
	assert.GreaterOrEqual(t, status.Confidence, 0.89)
	assert.Equal(t, 2, len(status.MatchedSpecs), "Should include all facts")
}

func TestAvailabilityDetector_MultipleChunks(t *testing.T) {
	detector := NewAvailabilityDetector(0.6, 0.5)

	chunks := []SemanticChunk{
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.4, // 0.6 similarity
		},
		{
			ChunkID:   uuid.New(),
			ChunkType: storage.ChunkTypeSpecRow,
			Distance:  0.2, // 0.8 similarity - higher
		},
	}

	status := detector.DetermineAvailability("Fuel Economy", []SpecFact{}, chunks)
	
	// Should use the maximum similarity (0.8)
	assert.Equal(t, AvailabilityStatusFound, status.Status)
	assert.GreaterOrEqual(t, status.Confidence, 0.79)
	assert.Equal(t, 2, len(status.MatchedChunks), "Should include all chunks")
}

