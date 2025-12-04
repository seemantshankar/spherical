package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpecNormalizer_NormalizeSpecName(t *testing.T) {
	normalizer := NewSpecNormalizer()

	tests := []struct {
		name           string
		input          string
		expectedCanonical string
		shouldHaveAlternatives bool
	}{
		// Fuel Economy synonyms
		{"Fuel Economy - direct", "Fuel Economy", "Fuel Economy", true},
		{"Fuel Economy - mileage", "Mileage", "Fuel Economy", true},
		{"Fuel Economy - fuel consumption", "Fuel Consumption", "Fuel Economy", true},
		{"Fuel Economy - fuel efficiency", "Fuel Efficiency", "Fuel Economy", true},
		{"Fuel Economy - km/l", "km/l", "Fuel Economy", true},
		{"Fuel Economy - kmpl", "kmpl", "Fuel Economy", true},
		{"Fuel Economy - mpg", "mpg", "Fuel Economy", true},

		// Engine Torque synonyms
		{"Engine Torque - direct", "Engine Torque", "Engine Torque", true},
		{"Engine Torque - torque", "Torque", "Engine Torque", true},
		{"Engine Torque - maximum torque", "Maximum Torque", "Engine Torque", true},
		{"Engine Torque - peak torque", "Peak Torque", "Engine Torque", true},

		// Ground Clearance synonyms
		{"Ground Clearance - direct", "Ground Clearance", "Ground Clearance", true},
		{"Ground Clearance - height", "Ground Clearance Height", "Ground Clearance", true},
		{"Ground Clearance - minimum", "Minimum Ground Clearance", "Ground Clearance", true},

		// Interior Comfort synonyms
		{"Interior Comfort - direct", "Interior Comfort", "Interior Comfort", true},
		{"Interior Comfort - comfort features", "Comfort Features", "Interior Comfort", true},
		{"Interior Comfort - seating comfort", "Seating Comfort", "Interior Comfort", true},

		// Case insensitivity
		{"Case insensitive - lowercase", "fuel economy", "Fuel Economy", true},
		{"Case insensitive - mixed", "FuEl EcOnOmY", "Fuel Economy", true},
		{"Case insensitive - uppercase", "FUEL ECONOMY", "Fuel Economy", true},

		// Unknown spec (should normalize to title case)
		{"Unknown spec", "Custom Spec Name", "Custom Spec Name", false},
		{"Unknown spec - lowercase", "custom spec name", "Custom Spec Name", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			canonical, alternatives := normalizer.NormalizeSpecName(tc.input)
			assert.Equal(t, tc.expectedCanonical, canonical, "Canonical name mismatch")
			if tc.shouldHaveAlternatives {
				assert.Greater(t, len(alternatives), 0, "Should have alternatives for known spec")
			}
		})
	}
}

func TestSpecNormalizer_FindCanonicalCategory(t *testing.T) {
	normalizer := NewSpecNormalizer()

	tests := []struct {
		name           string
		specName       string
		expectedCategory string
	}{
		// Fuel Efficiency category
		{"Fuel Economy", "Fuel Economy", "Fuel Efficiency"},
		{"Mileage", "Mileage", "Fuel Efficiency"},
		{"Fuel Consumption", "Fuel Consumption", "Fuel Efficiency"},
		{"Fuel Tank Capacity", "Fuel Tank Capacity", "Fuel Efficiency"},

		// Engine category
		{"Engine Torque", "Engine Torque", "Engine"},
		{"Torque", "Torque", "Engine"},
		{"Engine Specifications", "Engine Specifications", "Engine"},
		{"Engine Specs", "Engine Specs", "Engine"},

		// Dimensions category
		{"Ground Clearance", "Ground Clearance", "Dimensions"},
		{"Ground Clearance Height", "Ground Clearance Height", "Dimensions"},
		{"Length", "Length", "Dimensions"},
		{"Width", "Width", "Dimensions"},
		{"Height", "Height", "Dimensions"},
		{"Boot Space", "Boot Space", "Dimensions"},

		// Comfort category
		{"Interior Comfort", "Interior Comfort", "Comfort"},
		{"Comfort Features", "Comfort Features", "Comfort"},
		{"Seating Capacity", "Seating Capacity", "Comfort"},

		// Safety category
		{"Airbags", "Airbags", "Safety"},
		{"ABS", "ABS", "Safety"},
		{"Parking Sensors", "Parking Sensors", "Safety"},

		// Technology category
		{"Audio", "Audio", "Technology"},
		{"Navigation", "Navigation", "Technology"},
		{"GPS", "GPS", "Technology"},

		// Exterior category
		{"Headlights", "Headlights", "Exterior"},
		{"Sunroof", "Sunroof", "Exterior"},
		{"Colors", "Colors", "Exterior"},

		// Wheels category
		{"Alloy Wheels", "Alloy Wheels", "Wheels"},
		{"Wheel Size", "Wheel Size", "Wheels"},

		// Suspension category
		{"Suspension", "Suspension", "Suspension"},

		// Unknown spec (should default to General)
		{"Unknown Spec", "Unknown Spec", "General"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			category := normalizer.FindCanonicalCategory(tc.specName)
			assert.Equal(t, tc.expectedCategory, category, "Category mismatch for: %s", tc.specName)
		})
	}
}

func TestSpecNormalizer_EdgeCases(t *testing.T) {
	normalizer := NewSpecNormalizer()

	tests := []struct {
		name  string
		input string
	}{
		{"Empty string", ""},
		{"Whitespace only", "   "},
		{"Multiple spaces", "Fuel   Economy"},
		{"Leading/trailing spaces", "  Fuel Economy  "},
		{"Special characters", "Fuel Economy (City)"},
		{"Numbers", "Fuel Economy 2024"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			canonical, alternatives := normalizer.NormalizeSpecName(tc.input)
			// Should not panic and should return something (even if empty string)
			// Empty string is valid, just check it doesn't panic
			_ = canonical
			_ = alternatives
		})
	}
}

func TestSpecNormalizer_PartialMatching(t *testing.T) {
	normalizer := NewSpecNormalizer()

	tests := []struct {
		name           string
		input          string
		expectedCategory string
	}{
		{"Contains fuel economy", "What is the fuel economy?", "Fuel Efficiency"},
		{"Contains ground clearance", "Tell me about ground clearance", "Dimensions"},
		{"Contains engine torque", "I need engine torque info", "Engine"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			category := normalizer.FindCanonicalCategory(tc.input)
			assert.Equal(t, tc.expectedCategory, category, "Category mismatch for partial match: %s", tc.input)
		})
	}
}

