package extract

import (
	"fmt"
	"strings"
)

// Deduplicator handles removal of redundant specifications
type Deduplicator struct {
	seenSpecs map[string]bool
}

// NewDeduplicator creates a new Deduplicator
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		seenSpecs: make(map[string]bool),
	}
}

// DeduplicateMarkdown removes redundant table rows from the markdown
func (d *Deduplicator) DeduplicateMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect table start/end
		if strings.HasPrefix(trimmed, "|") {

			// Check if it's a separator line
			if strings.Contains(trimmed, "---") {
				result = append(result, line)
				continue
			}

			// Parse row
			parts := strings.Split(trimmed, "|")
			if len(parts) >= 4 { // Expecting at least empty start, Cat, Spec, Val, ...
				// Extract key components (Category, Specification, Value)
				// Assuming format: | Category | Specification | Value | ... |
				category := strings.TrimSpace(parts[1])
				spec := strings.TrimSpace(parts[2])
				value := strings.TrimSpace(parts[3])

				// Skip header row
				if category == "Category" && spec == "Specification" && value == "Value" {
					result = append(result, line)
					continue
				}

				// Create unique key
				key := fmt.Sprintf("%s|%s|%s", category, spec, value)

				// Check for duplicate
				if d.seenSpecs[key] {
					// Duplicate found, skip this line
					continue
				}

				// Mark as seen
				d.seenSpecs[key] = true
			}

			result = append(result, line)
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
