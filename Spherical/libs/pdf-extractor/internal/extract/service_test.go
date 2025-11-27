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

