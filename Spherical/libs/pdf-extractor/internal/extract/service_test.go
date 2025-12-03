package extract

import (
	"strings"
	"testing"

	"github.com/spherical/pdf-extractor/internal/domain"
)

// Tests for formatCategorizationHeader (FR-016)

func TestFormatCategorizationHeader(t *testing.T) {
	tests := []struct {
		name     string
		metadata *domain.DocumentMetadata
		want     []string // Strings that should appear in output
	}{
		{
			name: "full metadata",
			metadata: &domain.DocumentMetadata{
				Domain:      "Automobile",
				Subdomain:   "Sedan",
				CountryCode: "IN",
				ModelYear:   2025,
				Condition:   "New",
				Make:        "Toyota",
				Model:       "Camry",
			},
			want: []string{
				"---",
				"domain: Automobile",
				"subdomain: Sedan",
				"country_code: IN",
				"model_year: 2025",
				"condition: New",
				"make: Toyota",
				"model: Camry",
			},
		},
		{
			name:     "default metadata (unknown)",
			metadata: domain.NewDocumentMetadata(),
			want: []string{
				"---",
				"domain: Unknown",
				"subdomain: Unknown",
				"country_code: Unknown",
				"model_year: Unknown", // 0 should be rendered as Unknown
				"condition: Unknown",
				"make: Unknown",
				"model: Unknown",
			},
		},
		{
			name: "partial metadata",
			metadata: &domain.DocumentMetadata{
				Domain:      "Real Estate",
				Subdomain:   "Unknown",
				CountryCode: "US",
				ModelYear:   0,
				Condition:   "Unknown",
				Make:        "Unknown",
				Model:       "Unknown",
			},
			want: []string{
				"domain: Real Estate",
				"country_code: US",
				"model_year: Unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCategorizationHeader(tt.metadata)

			// Check that it starts and ends with ---
			if !strings.HasPrefix(result, "---\n") {
				t.Errorf("Header should start with '---\\n', got: %q", result[:20])
			}

			// Check all expected strings appear
			for _, expected := range tt.want {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected header to contain %q, got:\n%s", expected, result)
				}
			}
		})
	}
}

func TestFormatCategorizationHeader_YAMLFormat(t *testing.T) {
	metadata := &domain.DocumentMetadata{
		Domain:      "Automobile",
		Subdomain:   "SUV",
		CountryCode: "DE",
		ModelYear:   2024,
		Condition:   "New",
		Make:        "BMW",
		Model:       "X5",
	}

	result := formatCategorizationHeader(metadata)

	// Verify YAML frontmatter delimiters
	lines := strings.Split(result, "\n")
	if lines[0] != "---" {
		t.Errorf("First line should be '---', got %q", lines[0])
	}

	// Find the closing delimiter
	foundClosing := false
	for i, line := range lines {
		if i > 0 && line == "---" {
			foundClosing = true
			break
		}
	}
	if !foundClosing {
		t.Error("Header should have closing '---' delimiter")
	}

	// Verify header ends with two newlines (for proper markdown separation)
	if !strings.HasSuffix(result, "---\n\n") {
		t.Error("Header should end with '---\\n\\n'")
	}
}

func TestFormatCategorizationHeader_FieldOrder(t *testing.T) {
	metadata := &domain.DocumentMetadata{
		Domain:      "Luxury Watch",
		Subdomain:   "Sport",
		CountryCode: "CH",
		ModelYear:   2023,
		Condition:   "New",
		Make:        "Rolex",
		Model:       "Submariner",
	}

	result := formatCategorizationHeader(metadata)

	// Verify field order
	expectedOrder := []string{
		"domain:",
		"subdomain:",
		"country_code:",
		"model_year:",
		"condition:",
		"make:",
		"model:",
	}

	lastIndex := -1
	for _, field := range expectedOrder {
		idx := strings.Index(result, field)
		if idx == -1 {
			t.Errorf("Field %q not found in header", field)
			continue
		}
		if idx <= lastIndex {
			t.Errorf("Field %q appears out of order (at %d, previous was at %d)", field, idx, lastIndex)
		}
		lastIndex = idx
	}
}

// Tests for cleanCodeblocks function (User Story 1 - T005, T006, T007)

func TestCleanCodeblocks_CodeblockWrappedMarkdown(t *testing.T) {
	// T005: Test cleanCodeblocks with codeblock-wrapped markdown
	input := "```markdown\n## Specifications\n| Category | Specification | Value |\n|----------|---------------|-------|\n| Engine | Type | 1.2L Petrol |\n```"
	expected := "## Specifications\n| Category | Specification | Value |\n|----------|---------------|-------|\n| Engine | Type | 1.2L Petrol |"
	
	result := cleanCodeblocks(input)
	if result != expected {
		t.Errorf("cleanCodeblocks() = %q, want %q", result, expected)
	}
	
	// Verify no codeblock delimiters remain
	if strings.Contains(result, "```") {
		t.Errorf("cleanCodeblocks() still contains codeblock delimiters: %q", result)
	}
}

func TestCleanCodeblocks_EmptyCodeblocks(t *testing.T) {
	// T006: Test cleanCodeblocks with empty codeblocks
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty codeblock",
			input:    "```markdown\n```",
			expected: "",
		},
		{
			name:     "empty codeblock with whitespace",
			input:    "```markdown\n\n```",
			expected: "",
		},
		{
			name:     "empty codeblock no language",
			input:    "```\n```",
			expected: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanCodeblocks(tt.input)
			if result != tt.expected {
				t.Errorf("cleanCodeblocks() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCleanCodeblocks_NestedCodeblocks(t *testing.T) {
	// T007: Test cleanCodeblocks with nested codeblocks
	// FR-018: For nested codeblocks, remove outer delimiters, preserve inner content
	input := "```markdown\n```markdown\n## Specifications\n| Category | Specification | Value |\n```\n```"
	
	result := cleanCodeblocks(input)
	// For nested codeblocks, we remove the outer delimiters first
	// The result should contain the inner content (which may still have codeblock delimiters)
	if !strings.Contains(result, "## Specifications") {
		t.Errorf("cleanCodeblocks() removed inner content: %q", result)
	}
	// Verify that at least one layer of codeblocks was removed
	if strings.Count(result, "```") >= strings.Count(input, "```") {
		t.Errorf("cleanCodeblocks() did not remove any codeblock delimiters from nested input")
	}
}

func TestCleanCodeblocks_CodeblocksInTableCells(t *testing.T) {
	// T012: Test edge case handling for codeblocks in table cells
	// FR-018: Remove delimiters, preserve cell content
	input := "## Specifications\n| Category | Specification | Value | Key Features |\n|----------|---------------|-------|--------------|\n| Engine | Type | ```1.2L Petrol``` | High efficiency |\n| Dimensions | Length | 3655 mm | Compact design |"
	// The regex should remove ```1.2L Petrol``` and leave "1.2L Petrol"
	result := cleanCodeblocks(input)
	
	// Verify codeblock delimiters removed from table cell
	if strings.Contains(result, "```") {
		t.Errorf("cleanCodeblocks() still contains codeblock delimiters in table cell: %q", result)
	}
	// Verify content is preserved
	if !strings.Contains(result, "1.2L Petrol") {
		t.Errorf("cleanCodeblocks() removed content from table cell: %q", result)
	}
	// Verify table structure is preserved
	if !strings.Contains(result, "| Engine | Type |") {
		t.Errorf("cleanCodeblocks() broke table structure: %q", result)
	}
}

func TestCleanCodeblocks_NoCodeblocks(t *testing.T) {
	// Test that content without codeblocks is unchanged
	input := "## Specifications\n| Category | Specification | Value |\n|----------|---------------|-------|\n| Engine | Type | 1.2L Petrol |"
	expected := input
	
	result := cleanCodeblocks(input)
	if result != expected {
		t.Errorf("cleanCodeblocks() modified content without codeblocks: got %q, want %q", result, expected)
	}
}

func TestCleanCodeblocks_Idempotent(t *testing.T) {
	// FR-019: Test that cleanCodeblocks is idempotent (safe to run multiple times)
	input := "```markdown\n## Specifications\n| Category | Specification | Value |\n```"
	
	// Run once
	result1 := cleanCodeblocks(input)
	
	// Run again on the result
	result2 := cleanCodeblocks(result1)
	
	if result1 != result2 {
		t.Errorf("cleanCodeblocks() is not idempotent: first run = %q, second run = %q", result1, result2)
	}
}

func TestCleanCodeblocks_MultipleCodeblocks(t *testing.T) {
	// Test handling multiple codeblocks in the same content
	input := "```markdown\n## Section 1\n```\nSome text\n```markdown\n## Section 2\n```"
	result := cleanCodeblocks(input)
	
	// Verify both codeblocks are removed
	if strings.Contains(result, "```") {
		t.Errorf("cleanCodeblocks() still contains codeblock delimiters: %q", result)
	}
	// Verify content is preserved
	if !strings.Contains(result, "## Section 1") {
		t.Errorf("cleanCodeblocks() removed Section 1: %q", result)
	}
	if !strings.Contains(result, "## Section 2") {
		t.Errorf("cleanCodeblocks() removed Section 2: %q", result)
	}
	if !strings.Contains(result, "Some text") {
		t.Errorf("cleanCodeblocks() removed text between codeblocks: %q", result)
	}
}

