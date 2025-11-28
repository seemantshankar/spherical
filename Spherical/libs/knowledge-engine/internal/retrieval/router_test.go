package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntentClassifier_Classify(t *testing.T) {
	classifier := NewIntentClassifier()

	tests := []struct {
		question       string
		expectedIntent Intent
		minConfidence  float64
	}{
		// Spec lookup patterns
		{"What is the fuel efficiency of the Camry?", IntentSpecLookup, 0.4},
		{"How much horsepower does it have?", IntentSpecLookup, 0.4},
		{"What's the engine displacement?", IntentSpecLookup, 0.4},
		{"Tell me about the mileage", IntentSpecLookup, 0.4},

		// Comparison patterns
		{"How does Camry compare to Accord?", IntentComparison, 0.8},
		{"Compare the fuel efficiency", IntentComparison, 0.8},
		{"Camry vs Accord", IntentComparison, 0.8},
		{"Which is better, Camry or Accord?", IntentComparison, 0.8},

		// USP patterns
		{"What makes this car unique?", IntentUSPLookup, 0.7},
		{"Why should I buy this?", IntentUSPLookup, 0.7},
		{"What's the best feature?", IntentUSPLookup, 0.7},

		// FAQ patterns
		{"How do I connect my phone?", IntentFAQ, 0.6},
		{"Can I charge wirelessly?", IntentFAQ, 0.6},

		// Unknown - no clear pattern
		{"Tell me more", IntentUnknown, 0.0},
		{"Interesting", IntentUnknown, 0.0},
		{"Hello there", IntentUnknown, 0.0},
	}

	for _, tc := range tests {
		t.Run(tc.question, func(t *testing.T) {
			intent, confidence := classifier.Classify(tc.question)
			assert.Equal(t, tc.expectedIntent, intent, "Intent mismatch for: %s", tc.question)
			assert.GreaterOrEqual(t, confidence, tc.minConfidence, 
				"Confidence too low for: %s (got %f)", tc.question, confidence)
		})
	}
}

func TestVectorFilters_Matching(t *testing.T) {
	// This tests the matchesFilters helper indirectly through the adapter

	t.Run("matches by tenant", func(t *testing.T) {
		// Setup would involve creating entries and filters
		// For now this is a placeholder for when the adapter is more complete
	})
}

