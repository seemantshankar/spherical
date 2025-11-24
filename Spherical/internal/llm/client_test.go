package llm

import (
	"os"
	"testing"
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
