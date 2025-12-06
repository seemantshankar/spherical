package retrieval

import (
	"testing"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestNewSummaryGenerator(t *testing.T) {
	sg := NewSummaryGenerator()
	assert.NotNil(t, sg)
}

func TestSummaryGenerator_GenerateSummary_FoundOnly(t *testing.T) {
	sg := NewSummaryGenerator()

	statuses := []SpecAvailabilityStatus{
		{
			SpecName: "Fuel Economy",
			Status:   AvailabilityStatusFound,
			Confidence: 0.9,
			MatchedSpecs: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Unit:       "km/l",
					Confidence: 0.9,
				},
			},
		},
		{
			SpecName: "Engine Torque",
			Status:   AvailabilityStatusFound,
			Confidence: 0.85,
			MatchedSpecs: []SpecFact{
				{
					Category:   "Engine",
					Name:       "Engine Torque",
					Value:      "221",
					Unit:       "Nm",
					Confidence: 0.85,
				},
			},
		},
	}

	summary := sg.GenerateSummary(statuses, []SpecFact{}, []SemanticChunk{})
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Found:")
	assert.Contains(t, summary, "Fuel Economy")
	assert.Contains(t, summary, "25.49")
	assert.Contains(t, summary, "km/l")
}

func TestSummaryGenerator_GenerateSummary_UnavailableOnly(t *testing.T) {
	sg := NewSummaryGenerator()

	statuses := []SpecAvailabilityStatus{
		{
			SpecName:   "Ground Clearance",
			Status:     AvailabilityStatusUnavailable,
			Confidence: 0.0,
		},
		{
			SpecName:   "NonExistent Spec",
			Status:     AvailabilityStatusUnavailable,
			Confidence: 0.0,
		},
	}

	summary := sg.GenerateSummary(statuses, []SpecFact{}, []SemanticChunk{})
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Unavailable:")
	assert.Contains(t, summary, "Ground Clearance")
}

func TestSummaryGenerator_GenerateSummary_Mixed(t *testing.T) {
	sg := NewSummaryGenerator()

	statuses := []SpecAvailabilityStatus{
		{
			SpecName: "Fuel Economy",
			Status:   AvailabilityStatusFound,
			Confidence: 0.9,
			MatchedSpecs: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Unit:       "km/l",
					Confidence: 0.9,
				},
			},
		},
		{
			SpecName:   "Ground Clearance",
			Status:     AvailabilityStatusUnavailable,
			Confidence: 0.0,
		},
		{
			SpecName: "Engine Torque",
			Status:   AvailabilityStatusPartial,
			Confidence: 0.5,
			MatchedChunks: []SemanticChunk{
				{
					ChunkID:   uuid.New(),
					ChunkType: storage.ChunkTypeSpecRow,
					Text:      "Engine Torque: 221 Nm",
					Distance:  0.5,
				},
			},
		},
	}

	summary := sg.GenerateSummary(statuses, []SpecFact{}, []SemanticChunk{})
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Found:")
	assert.Contains(t, summary, "Unavailable:")
	assert.Contains(t, summary, "Partial:")
	assert.Contains(t, summary, "low confidence")
}

func TestSummaryGenerator_GenerateSummary_Empty(t *testing.T) {
	sg := NewSummaryGenerator()

	summary := sg.GenerateSummary([]SpecAvailabilityStatus{}, []SpecFact{}, []SemanticChunk{})
	assert.Equal(t, "No specifications requested.", summary)
}

func TestSummaryGenerator_GenerateSummary_ChunksOnly(t *testing.T) {
	sg := NewSummaryGenerator()

	statuses := []SpecAvailabilityStatus{
		{
			SpecName: "Fuel Economy",
			Status:   AvailabilityStatusFound,
			Confidence: 0.8,
			MatchedChunks: []SemanticChunk{
				{
					ChunkID:   uuid.New(),
					ChunkType: storage.ChunkTypeSpecRow,
					Text:      "Fuel Economy: 25.49 km/l",
					Distance:  0.2,
				},
			},
		},
	}

	summary := sg.GenerateSummary(statuses, []SpecFact{}, []SemanticChunk{})
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Found:")
	assert.Contains(t, summary, "Fuel Economy")
}

func TestSummaryGenerator_GenerateDetailedSummary(t *testing.T) {
	sg := NewSummaryGenerator()

	statuses := []SpecAvailabilityStatus{
		{
			SpecName: "Fuel Economy",
			Status:   AvailabilityStatusFound,
			Confidence: 0.9,
			AlternativeNames: []string{"Mileage", "Fuel Consumption"},
			MatchedSpecs: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Unit:       "km/l",
					Confidence: 0.9,
				},
			},
		},
		{
			SpecName:   "Ground Clearance",
			Status:     AvailabilityStatusUnavailable,
			Confidence: 0.0,
		},
	}

	summary := sg.GenerateDetailedSummary(statuses, 0.45)
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Overall Confidence:")
	assert.Contains(t, summary, "Fuel Economy")
	assert.Contains(t, summary, "found")
	assert.Contains(t, summary, "Ground Clearance")
	assert.Contains(t, summary, "unavailable")
	assert.Contains(t, summary, "also known as")
}

func TestSummaryGenerator_GenerateDetailedSummary_Empty(t *testing.T) {
	sg := NewSummaryGenerator()

	summary := sg.GenerateDetailedSummary([]SpecAvailabilityStatus{}, 0.0)
	assert.Equal(t, "No specifications requested.", summary)
}

func TestSummaryGenerator_GenerateSummary_NoMatchedData(t *testing.T) {
	sg := NewSummaryGenerator()

	// Test with found status but no matched specs or chunks
	statuses := []SpecAvailabilityStatus{
		{
			SpecName:     "Fuel Economy",
			Status:       AvailabilityStatusFound,
			Confidence:   0.9,
			MatchedSpecs: []SpecFact{},
			MatchedChunks: []SemanticChunk{},
		},
	}

	summary := sg.GenerateSummary(statuses, []SpecFact{}, []SemanticChunk{})
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Found:")
	assert.Contains(t, summary, "Fuel Economy")
}

func TestSummaryGenerator_GenerateDetailedSummary_ManyAlternatives(t *testing.T) {
	sg := NewSummaryGenerator()

	// Test with many alternative names (should limit to 3)
	statuses := []SpecAvailabilityStatus{
		{
			SpecName:        "Fuel Economy",
			Status:          AvailabilityStatusFound,
			Confidence:      0.9,
			AlternativeNames: []string{"Mileage", "Fuel Consumption", "Fuel Efficiency", "km/l", "kmpl", "mpg"},
			MatchedSpecs: []SpecFact{
				{
					Category:   "Fuel Efficiency",
					Name:       "Fuel Economy",
					Value:      "25.49",
					Unit:       "km/l",
					Confidence: 0.9,
				},
			},
		},
	}

	summary := sg.GenerateDetailedSummary(statuses, 0.9)
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "also known as")
	// Should only show first 3 alternatives
}

