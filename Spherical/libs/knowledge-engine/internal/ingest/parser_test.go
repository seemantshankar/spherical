package ingest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_Metadata(t *testing.T) {
	content := `---
title: Toyota Camry 2025 Brochure
product: Camry Hybrid
year: 2025
locale: en-IN
market: India
trim: XLE Hybrid
---

# Specifications

Some content here.
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(content)

	require.NoError(t, err)
	assert.Equal(t, "Toyota Camry 2025 Brochure", result.Metadata.Title)
	assert.Equal(t, "Camry Hybrid", result.Metadata.ProductName)
	assert.Equal(t, 2025, result.Metadata.ModelYear)
	assert.Equal(t, "en-IN", result.Metadata.Locale)
	assert.Equal(t, "India", result.Metadata.Market)
	assert.Equal(t, "XLE Hybrid", result.Metadata.Trim)
}

func TestParser_Parse_SpecTable(t *testing.T) {
	content := `
## Specifications

| Category | Specification | Value | Unit |
|----------|---------------|-------|------|
| Engine | Displacement | 2487 | cc |
| Engine | Power | 176 | hp |
| Fuel Efficiency | Combined | 25.49 | km/l |
| Dimensions | Length | 4885 | mm |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(content)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.SpecValues), 4)

	// Check first spec
	found := false
	for _, spec := range result.SpecValues {
		if spec.Name == "Displacement" {
			found = true
			assert.Equal(t, "Engine", spec.Category)
			assert.Equal(t, "2487", spec.Value)
			assert.Equal(t, "cc", spec.Unit)
			require.NotNil(t, spec.Numeric)
			assert.Equal(t, 2487.0, *spec.Numeric)
		}
	}
	assert.True(t, found, "Displacement spec not found")
}

func TestParser_Parse_Features(t *testing.T) {
	content := `
## Features

- Advanced safety with 8 airbags
- Premium leather interior
- 9-inch touchscreen display with navigation
- Wireless charging pad
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(content)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.Features), 4)

	// Check tags inference
	found := false
	for _, feature := range result.Features {
		if strings.Contains(feature.Body, "airbags") {
			found = true
			assert.Contains(t, feature.Tags, "safety")
		}
	}
	assert.True(t, found, "Airbag feature not found")
}

func TestParser_Parse_USPs(t *testing.T) {
	content := `
## Unique Selling Points

- Best-in-class fuel efficiency of 25.49 km/l
- Only hybrid in the segment with 5-star safety rating
- Industry-leading warranty coverage
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(content)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.USPs), 3)
}

func TestParser_Parse_GeneratesChunks(t *testing.T) {
	// Create a longer content to test chunking with multiple paragraphs
	var builder strings.Builder
	for i := 0; i < 50; i++ {
		builder.WriteString("This is a test paragraph with some content about specifications. ")
		builder.WriteString("It contains information about fuel efficiency and performance. ")
		builder.WriteString("\n\n") // Paragraph break
	}
	content := builder.String()

	parser := NewParser(ParserConfig{ChunkSize: 256, ChunkOverlap: 32})
	result, err := parser.Parse(content)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.RawChunks), 1, "Should generate at least one chunk")

	for _, chunk := range result.RawChunks {
		assert.NotEmpty(t, chunk.Text)
	}
}

func TestUnitNormalizer(t *testing.T) {
	normalizer := NewUnitNormalizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"kilometers per liter", "km/l"},
		{"kmpl", "km/l"},
		{"horsepower", "hp"},
		{"bhp", "hp"},
		{"millimeters", "mm"},
		{"mm", "mm"}, // Already normalized
		{"unknown unit", "unknown unit"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizer.Normalize(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestValidateParsedBrochure(t *testing.T) {
	t.Run("warns on no specs", func(t *testing.T) {
		parsed := &ParsedBrochure{
			SpecValues: []ParsedSpec{},
		}

		errors := ValidateParsedBrochure(parsed)
		found := false
		for _, err := range errors {
			if strings.Contains(err.Message, "no specifications") {
				found = true
			}
		}
		assert.True(t, found, "Should warn about missing specifications")
	})

	t.Run("warns on duplicate specs", func(t *testing.T) {
		parsed := &ParsedBrochure{
			SpecValues: []ParsedSpec{
				{Category: "Engine", Name: "Power", SourceLine: 1},
				{Category: "Engine", Name: "Power", SourceLine: 2},
			},
		}

		errors := ValidateParsedBrochure(parsed)
		found := false
		for _, err := range errors {
			if strings.Contains(err.Message, "duplicate") {
				found = true
			}
		}
		assert.True(t, found, "Should warn about duplicate specs")
	})

	t.Run("warns on missing product name", func(t *testing.T) {
		parsed := &ParsedBrochure{
			Metadata:   BrochureMetadata{Title: "Test"},
			SpecValues: []ParsedSpec{{Category: "Test", Name: "Test"}},
		}

		errors := ValidateParsedBrochure(parsed)
		found := false
		for _, err := range errors {
			if strings.Contains(err.Message, "product name") {
				found = true
			}
		}
		assert.True(t, found, "Should warn about missing product name")
	})
}

