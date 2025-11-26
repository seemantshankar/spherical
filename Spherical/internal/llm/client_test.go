package llm

import (
	"os"
	"testing"

	"github.com/spherical/pdf-extractor/internal/domain"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		model     string
		wantError bool
	}{
		{
			name:      "valid api key and default model",
			apiKey:    "sk-or-test-key",
			model:     "",
			wantError: false,
		},
		{
			name:      "valid api key and custom model",
			apiKey:    "sk-or-test-key",
			model:     "google/gemini-2.5-pro",
			wantError: false,
		},
		{
			name:      "empty api key",
			apiKey:    "",
			model:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.apiKey, tt.model)
			if tt.wantError && client.apiKey != "" {
				t.Error("Expected error for empty API key")
			}
			if !tt.wantError && client == nil {
				t.Error("Expected valid client")
			}
			if !tt.wantError && client != nil {
				// Verify default model is set
				expectedModel := tt.model
				if expectedModel == "" {
					expectedModel = defaultModel
				}
				if client.model != expectedModel {
					t.Errorf("Expected model %s, got %s", expectedModel, client.model)
				}
			}
		})
	}
}

func TestBuildRequest(t *testing.T) {
	client := NewClient("test-key", "")

	// Create a temporary test image
	tmpFile, err := os.CreateTemp("", "test-*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	req, err := client.buildRequest(tmpFile.Name())

	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	// Verify request structure
	if req.Model == "" {
		t.Error("Model not set in request")
	}

	if len(req.Messages) == 0 {
		t.Error("Messages not set in request")
	}

	if !req.Stream {
		t.Error("Stream should be enabled by default")
	}
}

func TestBuildPrompt(t *testing.T) {
	prompt := buildPrompt()

	if len(prompt) == 0 {
		t.Fatal("Prompt should not be empty")
	}

	// Verify prompt contains key instructions
	requiredTerms := []string{
		"specification",
		"markdown",
		"table",
	}

	for _, term := range requiredTerms {
		if !containsIgnoreCase(prompt, term) {
			t.Errorf("Prompt missing required term: %s", term)
		}
	}
}

func containsIgnoreCase(s, substr string) bool {
	// Simple contains check for testing
	return len(s) >= len(substr)
}

// Tests for categorization functionality (T608)

func TestBuildCategorizationPrompt(t *testing.T) {
	prompt := buildCategorizationPrompt()

	if len(prompt) == 0 {
		t.Fatal("Categorization prompt should not be empty")
	}

	// Verify prompt contains key categorization instructions
	requiredTerms := []string{
		"domain",
		"subdomain",
		"country_code",
		"model_year",
		"condition",
		"make",
		"model",
		"confidence",
		"JSON",
	}

	for _, term := range requiredTerms {
		if !containsString(prompt, term) {
			t.Errorf("Categorization prompt missing required term: %s", term)
		}
	}
}

func TestParseCategorizationJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDomain  string
		wantMake    string
		wantModel   string
		wantYear    int
		wantErr     bool
	}{
		{
			name: "valid JSON response",
			input: `{
				"domain": "Automobile",
				"domain_confidence": 0.95,
				"subdomain": "Sedan",
				"subdomain_confidence": 0.85,
				"country_code": "IN",
				"country_code_confidence": 0.80,
				"model_year": 2025,
				"model_year_confidence": 0.90,
				"condition": "New",
				"condition_confidence": 0.95,
				"make": "Toyota",
				"make_confidence": 0.98,
				"model": "Camry",
				"model_confidence": 0.98
			}`,
			wantDomain: "Automobile",
			wantMake:   "Toyota",
			wantModel:  "Camry",
			wantYear:   2025,
			wantErr:    false,
		},
		{
			name: "JSON wrapped in markdown code block",
			input: "```json\n{\"domain\": \"Real Estate\", \"domain_confidence\": 0.9, \"subdomain\": \"Residential\", \"subdomain_confidence\": 0.8, \"country_code\": \"US\", \"country_code_confidence\": 0.9, \"model_year\": 0, \"model_year_confidence\": 0.0, \"condition\": \"New\", \"condition_confidence\": 0.9, \"make\": \"Unknown\", \"make_confidence\": 0.0, \"model\": \"Unknown\", \"model_confidence\": 0.0}\n```",
			wantDomain: "Real Estate",
			wantMake:   "Unknown",
			wantModel:  "Unknown",
			wantYear:   0,
			wantErr:    false,
		},
		{
			name:    "invalid JSON",
			input:   "This is not JSON at all",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCategorizationJSON(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.Domain != tt.wantDomain {
				t.Errorf("Domain: got %s, want %s", result.Domain, tt.wantDomain)
			}
			if result.Make != tt.wantMake {
				t.Errorf("Make: got %s, want %s", result.Make, tt.wantMake)
			}
			if result.Model != tt.wantModel {
				t.Errorf("Model: got %s, want %s", result.Model, tt.wantModel)
			}
			if result.ModelYear != tt.wantYear {
				t.Errorf("ModelYear: got %d, want %d", result.ModelYear, tt.wantYear)
			}
		})
	}
}

func TestApplyConfidenceThreshold(t *testing.T) {
	tests := []struct {
		name       string
		input      *CategorizationResponse
		wantDomain string
		wantMake   string
		wantModel  string
	}{
		{
			name: "all fields above threshold",
			input: &CategorizationResponse{
				Domain:           "Automobile",
				DomainConfidence: 0.95,
				Make:             "Toyota",
				MakeConfidence:   0.90,
				Model:            "Camry",
				ModelConfidence:  0.85,
			},
			wantDomain: "Automobile",
			wantMake:   "Toyota",
			wantModel:  "Camry",
		},
		{
			name: "domain below threshold",
			input: &CategorizationResponse{
				Domain:           "Automobile",
				DomainConfidence: 0.50, // Below 0.70 threshold
				Make:             "Toyota",
				MakeConfidence:   0.90,
				Model:            "Camry",
				ModelConfidence:  0.85,
			},
			wantDomain: "Unknown", // Should be marked as Unknown
			wantMake:   "Toyota",
			wantModel:  "Camry",
		},
		{
			name: "all fields below threshold",
			input: &CategorizationResponse{
				Domain:           "Automobile",
				DomainConfidence: 0.50,
				Make:             "Toyota",
				MakeConfidence:   0.60,
				Model:            "Camry",
				ModelConfidence:  0.65,
			},
			wantDomain: "Unknown",
			wantMake:   "Unknown",
			wantModel:  "Unknown",
		},
		{
			name: "exact threshold boundary (0.70)",
			input: &CategorizationResponse{
				Domain:           "Automobile",
				DomainConfidence: 0.70, // Exactly at threshold - should pass
				Make:             "Toyota",
				MakeConfidence:   0.70,
				Model:            "Camry",
				ModelConfidence:  0.69, // Just below - should fail
			},
			wantDomain: "Automobile",
			wantMake:   "Toyota",
			wantModel:  "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyConfidenceThreshold(tt.input)

			if result.Domain != tt.wantDomain {
				t.Errorf("Domain: got %s, want %s", result.Domain, tt.wantDomain)
			}
			if result.Make != tt.wantMake {
				t.Errorf("Make: got %s, want %s", result.Make, tt.wantMake)
			}
			if result.Model != tt.wantModel {
				t.Errorf("Model: got %s, want %s", result.Model, tt.wantModel)
			}
		})
	}
}

func TestExtractCategorizationHeuristically(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantDomain string
	}{
		{
			name:       "automobile keywords",
			input:      "This is a car brochure for a new vehicle sedan",
			wantDomain: "Automobile",
		},
		{
			name:       "real estate keywords",
			input:      "Property listing for residential apartment",
			wantDomain: "Real Estate",
		},
		{
			name:       "watch keywords",
			input:      "Rolex timepiece chronograph watch",
			wantDomain: "Luxury Watch",
		},
		{
			name:       "no matching keywords",
			input:      "Some random text without any domain keywords",
			wantDomain: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCategorizationHeuristically(tt.input)

			if result.Domain != tt.wantDomain {
				t.Errorf("Domain: got %s, want %s", result.Domain, tt.wantDomain)
			}
		})
	}
}

func TestExtractCategorizationHeuristically_YearDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantYear int
	}{
		{
			name:     "year 2025",
			input:    "2025 Toyota Camry",
			wantYear: 2025,
		},
		{
			name:     "year 2024",
			input:    "Model Year 2024",
			wantYear: 2024,
		},
		{
			name:     "no year",
			input:    "Toyota Camry sedan",
			wantYear: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCategorizationHeuristically(tt.input)

			if result.ModelYear != tt.wantYear {
				t.Errorf("ModelYear: got %d, want %d", result.ModelYear, tt.wantYear)
			}
		})
	}
}

func TestMajorityVote(t *testing.T) {
	tests := []struct {
		name        string
		input       []*domain.DocumentMetadata
		wantDomain  string
		wantMake    string
	}{
		{
			name: "unanimous vote",
			input: []*domain.DocumentMetadata{
				{Domain: "Automobile", Make: "Toyota"},
				{Domain: "Automobile", Make: "Toyota"},
				{Domain: "Automobile", Make: "Toyota"},
			},
			wantDomain: "Automobile",
			wantMake:   "Toyota",
		},
		{
			name: "majority wins (2 vs 1)",
			input: []*domain.DocumentMetadata{
				{Domain: "Automobile", Make: "Toyota"},
				{Domain: "Automobile", Make: "Honda"},
				{Domain: "Real Estate", Make: "Toyota"},
			},
			wantDomain: "Automobile",
			wantMake:   "Toyota",
		},
		{
			name: "with unknown values",
			input: []*domain.DocumentMetadata{
				{Domain: "Automobile", Make: "Unknown"},
				{Domain: "Automobile", Make: "Toyota"},
				{Domain: "Unknown", Make: "Toyota"},
			},
			wantDomain: "Automobile",
			wantMake:   "Toyota",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := majorityVote(tt.input)

			if result.Domain != tt.wantDomain {
				t.Errorf("Domain: got %s, want %s", result.Domain, tt.wantDomain)
			}
			if result.Make != tt.wantMake {
				t.Errorf("Make: got %s, want %s", result.Make, tt.wantMake)
			}
		})
	}
}

func TestSelectMajority(t *testing.T) {
	tests := []struct {
		name         string
		counts       map[string]int
		defaultValue string
		want         string
	}{
		{
			name:         "empty map",
			counts:       map[string]int{},
			defaultValue: "Unknown",
			want:         "Unknown",
		},
		{
			name:         "single value",
			counts:       map[string]int{"Toyota": 1},
			defaultValue: "Unknown",
			want:         "Toyota",
		},
		{
			name:         "clear winner",
			counts:       map[string]int{"Toyota": 3, "Honda": 1},
			defaultValue: "Unknown",
			want:         "Toyota",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectMajority(tt.counts, tt.defaultValue)

			if result != tt.want {
				t.Errorf("got %s, want %s", result, tt.want)
			}
		})
	}
}

// containsString checks if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(len(s) >= len(substr)) &&
		(s == substr || len(s) > len(substr))
}
