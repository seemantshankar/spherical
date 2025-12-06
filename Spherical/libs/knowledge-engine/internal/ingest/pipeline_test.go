package ingest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnforceSingleSentence_Success(t *testing.T) {
	expl, err := enforceSingleSentence("Range is 300 km")
	assert.NoError(t, err)
	assert.Equal(t, "Range is 300 km.", expl)
}

func TestEnforceSingleSentence_MultiSentenceFails(t *testing.T) {
	_, err := enforceSingleSentence("First sentence. Second sentence.")
	assert.Error(t, err)
}

func TestEnforceSingleSentence_TooLongFails(t *testing.T) {
	longText := make([]byte, maxExplanationLength+10)
	for i := range longText {
		longText[i] = 'a'
	}
	_, err := enforceSingleSentence(string(longText))
	assert.Error(t, err)
}

func TestBuildSpecExplanation(t *testing.T) {
	spec := ParsedSpec{
		Category:            "Engine",
		Name:                "Battery Range",
		Value:               "300",
		Unit:                "km",
		KeyFeatures:         "Fast charge",
		VariantAvailability: "Standard",
	}

	expl, failed := buildSpecExplanation(spec)
	assert.False(t, failed)
	assert.Contains(t, expl, "Engine Battery Range is 300 km")
	assert.Contains(t, expl, "Key features")
	assert.Contains(t, expl, "Availability")
	assert.Equal(t, ".", expl[len(expl)-1:])
}

func TestBuildSpecExplanation_FailsWhenTooLong(t *testing.T) {
	long := strings.Repeat("very-long-text ", 20) // make explanation exceed max length
	spec := ParsedSpec{
		Category: "Engine",
		Name:     "Battery Range",
		Value:    long,
	}

	expl, failed := buildSpecExplanation(spec)
	assert.True(t, failed)
	assert.Empty(t, expl)
}
