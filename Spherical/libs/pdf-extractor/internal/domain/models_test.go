package domain

import "testing"

// Tests for DocumentMetadata and validation functions (FR-016)

func TestNewDocumentMetadata(t *testing.T) {
	metadata := NewDocumentMetadata()

	if metadata.Domain != "Unknown" {
		t.Errorf("Expected Domain to be 'Unknown', got '%s'", metadata.Domain)
	}
	if metadata.Make != "Unknown" {
		t.Errorf("Expected Make to be 'Unknown', got '%s'", metadata.Make)
	}
	if metadata.Model != "Unknown" {
		t.Errorf("Expected Model to be 'Unknown', got '%s'", metadata.Model)
	}
	if metadata.Confidence != 0.0 {
		t.Errorf("Expected Confidence to be 0.0, got %f", metadata.Confidence)
	}
}

func TestDocumentMetadata_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		metadata *DocumentMetadata
		want     bool
	}{
		{
			name:     "all unknown",
			metadata: NewDocumentMetadata(),
			want:     false,
		},
		{
			name: "domain set",
			metadata: &DocumentMetadata{
				Domain: "Automobile",
				Make:   "Unknown",
				Model:  "Unknown",
			},
			want: true,
		},
		{
			name: "make set",
			metadata: &DocumentMetadata{
				Domain: "Unknown",
				Make:   "Toyota",
				Model:  "Unknown",
			},
			want: true,
		},
		{
			name: "model set",
			metadata: &DocumentMetadata{
				Domain: "Unknown",
				Make:   "Unknown",
				Model:  "Camry",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metadata.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCountryCode(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"US", true},
		{"UK", true},
		{"IN", true},
		{"DE", true},
		{"JP", true},
		{"Unknown", true},
		{"", true},
		{"XX", false}, // Invalid code
		{"usa", false},
		{"INVALID", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := ValidateCountryCode(tt.code); got != tt.want {
				t.Errorf("ValidateCountryCode(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestNormalizeCountryCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"us", "US"},
		{"US", "US"},
		{"  in  ", "IN"},
		{"uk", "UK"},
		{"", "Unknown"},
		{"invalid", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeCountryCode(tt.input); got != tt.want {
				t.Errorf("NormalizeCountryCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateModelYear(t *testing.T) {
	tests := []struct {
		year int
		want bool
	}{
		{0, true},    // Unknown/not set
		{1900, true}, // Min valid
		{2025, true}, // Current
		{2100, true}, // Max valid
		{1899, false},
		{2101, false},
		{-1, false},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.year)), func(t *testing.T) {
			if got := ValidateModelYear(tt.year); got != tt.want {
				t.Errorf("ValidateModelYear(%d) = %v, want %v", tt.year, got, tt.want)
			}
		})
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"Automobile", true},
		{"automobile", true}, // Case insensitive
		{"Real Estate", true},
		{"Luxury Watch", true},
		{"Jewelry", true},
		{"Electronics", true},
		{"Unknown", true},
		{"", true},
		{"Invalid Domain", false},
		{"Random", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			if got := ValidateDomain(tt.domain); got != tt.want {
				t.Errorf("ValidateDomain(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"automobile", "Automobile"},
		{"AUTOMOBILE", "Automobile"},
		{"Real Estate", "Real Estate"},
		{"", "Unknown"},
		{"Random", "Random"}, // Non-standard returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeDomain(tt.input); got != tt.want {
				t.Errorf("NormalizeDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		subdomain string
		want      bool
	}{
		{"Sedan", true},
		{"SUV", true},
		{"Commercial", true},
		{"Unknown", true},
		{"", true},
		{"Residential-Commercial", true},
		{"Test@123", false}, // Special characters
		{"Sub$domain", false},
	}

	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			if got := ValidateSubdomain(tt.subdomain); got != tt.want {
				t.Errorf("ValidateSubdomain(%q) = %v, want %v", tt.subdomain, got, tt.want)
			}
		})
	}
}

func TestValidateCondition(t *testing.T) {
	tests := []struct {
		condition string
		want      bool
	}{
		{"New", true},
		{"Used", true},
		{"Secondary Resale", true},
		{"Certified Pre-Owned", true},
		{"Refurbished", true},
		{"Unknown", true},
		{"", true},
		{"Custom Condition", true}, // Alphanumeric custom values accepted
		{"Bad@Condition", false},
	}

	for _, tt := range tests {
		t.Run(tt.condition, func(t *testing.T) {
			if got := ValidateCondition(tt.condition); got != tt.want {
				t.Errorf("ValidateCondition(%q) = %v, want %v", tt.condition, got, tt.want)
			}
		})
	}
}

func TestNormalizeCondition(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"new", "New"},
		{"NEW", "New"},
		{"used", "Used"},
		{"secondary resale", "Secondary Resale"},
		{"resale", "Secondary Resale"},
		{"cpo", "Certified Pre-Owned"},
		{"certified pre-owned", "Certified Pre-Owned"},
		{"refurbished", "Refurbished"},
		{"", "Unknown"},
		{"Custom", "Custom"}, // Non-standard returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeCondition(tt.input); got != tt.want {
				t.Errorf("NormalizeCondition(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

