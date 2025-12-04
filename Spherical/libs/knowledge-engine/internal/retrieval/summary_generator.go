// Package retrieval provides natural language summary generation for retrieval results.
package retrieval

import (
	"fmt"
	"strings"
)

// SummaryGenerator generates natural language summaries from retrieval results.
type SummaryGenerator struct {
	// llmClient LLMClient // For future LLM integration
}

// NewSummaryGenerator creates a new summary generator.
func NewSummaryGenerator() *SummaryGenerator {
	return &SummaryGenerator{}
}

// GenerateSummary generates a concise natural language summary of the retrieval results.
func (sg *SummaryGenerator) GenerateSummary(
	specStatuses []SpecAvailabilityStatus,
	facts []SpecFact,
	chunks []SemanticChunk,
) string {
	if len(specStatuses) == 0 {
		return "No specifications requested."
	}

	var foundSpecs []string
	var unavailableSpecs []string
	var partialSpecs []string

	// Categorize specs by status
	for _, status := range specStatuses {
		switch status.Status {
		case AvailabilityStatusFound:
			// Format: "SpecName (value unit)" if we have facts
			if len(status.MatchedSpecs) > 0 {
				fact := status.MatchedSpecs[0]
				valueStr := fact.Value
				if fact.Unit != "" {
					valueStr = fmt.Sprintf("%s %s", fact.Value, fact.Unit)
				}
				foundSpecs = append(foundSpecs, fmt.Sprintf("%s (%s)", status.SpecName, valueStr))
			} else if len(status.MatchedChunks) > 0 {
				// Use chunk text if no structured facts
				foundSpecs = append(foundSpecs, status.SpecName)
			} else {
				foundSpecs = append(foundSpecs, status.SpecName)
			}
		case AvailabilityStatusUnavailable:
			unavailableSpecs = append(unavailableSpecs, status.SpecName)
		case AvailabilityStatusPartial:
			partialSpecs = append(partialSpecs, fmt.Sprintf("%s (low confidence)", status.SpecName))
		}
	}

	// Build summary
	var parts []string

	if len(foundSpecs) > 0 {
		parts = append(parts, fmt.Sprintf("Found: %s", strings.Join(foundSpecs, ", ")))
	}

	if len(unavailableSpecs) > 0 {
		parts = append(parts, fmt.Sprintf("Unavailable: %s", strings.Join(unavailableSpecs, ", ")))
	}

	if len(partialSpecs) > 0 {
		parts = append(parts, fmt.Sprintf("Partial: %s", strings.Join(partialSpecs, ", ")))
	}

	if len(parts) == 0 {
		return "No results found."
	}

	return strings.Join(parts, "\n")
}

// GenerateDetailedSummary generates a more detailed summary with confidence scores.
func (sg *SummaryGenerator) GenerateDetailedSummary(
	specStatuses []SpecAvailabilityStatus,
	overallConfidence float64,
) string {
	if len(specStatuses) == 0 {
		return "No specifications requested."
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Overall Confidence: %.1f%%", overallConfidence*100))
	parts = append(parts, "")

	for _, status := range specStatuses {
		statusLine := fmt.Sprintf("%s: %s (%.1f%% confidence)", status.SpecName, status.Status, status.Confidence*100)
		
		if len(status.MatchedSpecs) > 0 {
			fact := status.MatchedSpecs[0]
			valueStr := fact.Value
			if fact.Unit != "" {
				valueStr = fmt.Sprintf("%s %s", fact.Value, fact.Unit)
			}
			statusLine += fmt.Sprintf(" - Value: %s", valueStr)
		}
		
		if len(status.AlternativeNames) > 0 {
			maxAlt := 3
			if len(status.AlternativeNames) < maxAlt {
				maxAlt = len(status.AlternativeNames)
			}
			statusLine += fmt.Sprintf(" (also known as: %s)", strings.Join(status.AlternativeNames[:maxAlt], ", "))
		}
		
		parts = append(parts, statusLine)
	}

	return strings.Join(parts, "\n")
}

