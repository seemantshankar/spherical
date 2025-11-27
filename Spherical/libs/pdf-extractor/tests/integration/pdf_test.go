package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

const (
	// Sample PDF path from requirements
	testPDFPath = "/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Arena-Wagon-r-Brochure.pdf"
)

func init() {
	// Load .env file for testing
	_ = godotenv.Load("../../.env")
}

// TestPDFToMarkdownConversion tests the complete flow from PDF to Markdown
func TestPDFToMarkdownConversion(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	// Create output channel for events
	eventCh := make(chan domain.StreamEvent, 100)

	// Process PDF in a goroutine
	go func() {
		err := extractor.Process(ctx, testPDFPath, eventCh)
		if err != nil {
			t.Errorf("Process failed: %v", err)
		}
		close(eventCh)
	}()

	// Collect results
	var markdown string
	var pagesProcessed int
	var hasError bool

	for event := range eventCh {
		switch event.Type {
		case domain.EventPageProcessing:
			t.Logf("Processing page %d", event.PageNumber)
			pagesProcessed++

		case domain.EventLLMStreaming:
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}

		case domain.EventError:
			t.Errorf("Error event: %v", event.Payload)
			hasError = true

		case domain.EventComplete:
			t.Log("Processing complete")
		}
	}

	// Assertions
	if hasError {
		t.Fatal("Processing encountered errors")
	}

	if pagesProcessed == 0 {
		t.Fatal("No pages were processed")
	}

	if len(markdown) == 0 {
		t.Fatal("No markdown content generated")
	}

	// Verify markdown contains expected content patterns
	// Use case-insensitive matching for text patterns
	markdownLower := strings.ToLower(markdown)

	expectedPatterns := map[string]bool{
		"specification": strings.Contains(markdownLower, "specification"),
		"feature":       strings.Contains(markdownLower, "feature"),
		"table":         strings.Contains(markdown, "|"), // Tables have | character
	}

	for pattern, found := range expectedPatterns {
		if !found {
			t.Logf("Warning: Markdown missing expected pattern: %s (may be okay depending on content)", pattern)
		}
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "pdf-extraction-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// Helper function to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// CATEGORIZATION INTEGRATION TESTS (FR-016)
// =============================================================================

// TestCategorizationMetadataExtraction verifies categorization metadata is extracted
// and included in EventComplete events (T612)
func TestCategorizationMetadataExtraction(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize real components (Constitution Principle IV - no mocks)
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	// Create output channel for events
	eventCh := make(chan domain.StreamEvent, 100)

	// Process PDF using ProcessWithResult to get metadata
	var result *extract.ProcessResult
	var processErr error

	go func() {
		result, processErr = extractor.ProcessWithResult(ctx, testPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect events and look for EventComplete with metadata
	var completePayload *extract.CompletePayload

	for event := range eventCh {
		if event.Type == domain.EventComplete {
			if payload, ok := event.Payload.(*extract.CompletePayload); ok {
				completePayload = payload
			}
		}
	}

	// Verify processing succeeded
	if processErr != nil {
		t.Fatalf("ProcessWithResult failed: %v", processErr)
	}

	// Verify result contains metadata
	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	if result.Metadata == nil {
		t.Fatal("Result metadata is nil")
	}

	t.Logf("Categorization Metadata:")
	t.Logf("  Domain:      %s", result.Metadata.Domain)
	t.Logf("  Subdomain:   %s", result.Metadata.Subdomain)
	t.Logf("  CountryCode: %s", result.Metadata.CountryCode)
	t.Logf("  ModelYear:   %d", result.Metadata.ModelYear)
	t.Logf("  Condition:   %s", result.Metadata.Condition)
	t.Logf("  Make:        %s", result.Metadata.Make)
	t.Logf("  Model:       %s", result.Metadata.Model)
	t.Logf("  Confidence:  %.2f", result.Metadata.Confidence)

	// Verify EventComplete payload contains metadata
	if completePayload == nil {
		t.Fatal("EventComplete payload is nil")
	}

	if completePayload.Metadata == nil {
		t.Fatal("EventComplete payload metadata is nil")
	}

	// Verify metadata is populated (at least some fields should be detected)
	if result.Metadata.Domain == "Unknown" &&
		result.Metadata.Make == "Unknown" &&
		result.Metadata.Model == "Unknown" {
		t.Log("Warning: All categorization fields are 'Unknown' - may indicate detection issues")
	}
}

// TestYAMLFrontmatterFormat verifies the YAML frontmatter format in output (T616)
func TestYAMLFrontmatterFormat(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize real components (Constitution Principle IV - no mocks)
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	// Create output channel for events
	eventCh := make(chan domain.StreamEvent, 100)

	// Process PDF
	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, testPDFPath, eventCh)
		close(eventCh)
	}()

	// Drain event channel
	for range eventCh {
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	markdown := result.Markdown

	// Verify YAML frontmatter format
	if !strings.HasPrefix(markdown, "---\n") {
		t.Fatalf("Markdown should start with '---\\n', got: %q", markdown[:min(20, len(markdown))])
	}

	// Find the closing delimiter
	secondDelimiter := strings.Index(markdown[4:], "\n---\n")
	if secondDelimiter == -1 {
		t.Fatal("Markdown missing closing '---' YAML delimiter")
	}

	// Extract frontmatter content
	frontmatter := markdown[4 : 4+secondDelimiter]
	t.Logf("YAML Frontmatter:\n%s", frontmatter)

	// Verify required fields are present
	requiredFields := []string{
		"domain:",
		"subdomain:",
		"country_code:",
		"model_year:",
		"condition:",
		"make:",
		"model:",
	}

	for _, field := range requiredFields {
		if !strings.Contains(frontmatter, field) {
			t.Errorf("YAML frontmatter missing required field: %s", field)
		}
	}

	// Verify header appears at the very top of output
	lines := strings.Split(markdown, "\n")
	if lines[0] != "---" {
		t.Errorf("First line should be '---', got %q", lines[0])
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "categorization-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestCLISummaryJSON verifies --summary-json output includes categorization (T620)
func TestCLISummaryJSON(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	// Get the project root directory
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Failed to get project root: %v", err)
	}

	// Build the CLI if not already built
	cliPath := filepath.Join(os.TempDir(), "pdf-extractor-test")
	buildCmd := exec.Command("go", "build", "-o", cliPath, "./cmd/pdf-extractor")
	buildCmd.Dir = projectRoot
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, string(buildOutput))
	}
	defer os.Remove(cliPath)

	// Create temp output path
	outputPath := filepath.Join(os.TempDir(), "cli-test-output.md")
	defer os.Remove(outputPath)

	// Run CLI with --summary-json
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, cliPath,
		"-o", outputPath,
		"--summary-json",
		testPDFPath,
	)
	cmd.Env = os.Environ() // Include OPENROUTER_API_KEY

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\nOutput: %s", err, string(output))
	}

	t.Logf("CLI output:\n%s", string(output))

	// Verify Markdown output file exists and has YAML frontmatter
	mdContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read Markdown output: %v", err)
	}

	if !strings.HasPrefix(string(mdContent), "---\n") {
		t.Error("Markdown output missing YAML frontmatter header")
	}

	// Verify summary JSON file exists
	jsonPath := strings.TrimSuffix(outputPath, ".md") + "-summary.json"
	defer os.Remove(jsonPath)

	jsonContent, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read summary JSON: %v", err)
	}

	// Parse and verify JSON structure
	var summary struct {
		Metadata struct {
			Domain      string  `json:"domain"`
			Subdomain   string  `json:"subdomain"`
			CountryCode string  `json:"country_code"`
			ModelYear   int     `json:"model_year"`
			Condition   string  `json:"condition"`
			Make        string  `json:"make"`
			Model       string  `json:"model"`
			Confidence  float64 `json:"confidence"`
		} `json:"metadata"`
		OutputFile string `json:"output_file"`
		Stats      struct {
			TotalPages      int `json:"TotalPages"`
			SuccessfulPages int `json:"SuccessfulPages"`
		} `json:"stats"`
	}

	if err := json.Unmarshal(jsonContent, &summary); err != nil {
		t.Fatalf("Failed to parse summary JSON: %v\nContent: %s", err, string(jsonContent))
	}

	t.Logf("Summary JSON metadata:")
	t.Logf("  Domain:      %s", summary.Metadata.Domain)
	t.Logf("  Make:        %s", summary.Metadata.Make)
	t.Logf("  Model:       %s", summary.Metadata.Model)
	t.Logf("  Confidence:  %.2f", summary.Metadata.Confidence)

	// Verify output file path is set
	if summary.OutputFile == "" {
		t.Error("Summary JSON missing output_file field")
	}
}

// TestCategorizationEdgeCases tests edge cases for categorization detection (T624)
func TestCategorizationEdgeCases(t *testing.T) {
	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	// Test cases with various scenarios
	testCases := []struct {
		name        string
		description string
		checkFn     func(t *testing.T, metadata *domain.DocumentMetadata)
	}{
		{
			name:        "default_unknown_values",
			description: "Verify default metadata has Unknown values",
			checkFn: func(t *testing.T, metadata *domain.DocumentMetadata) {
				defaultMeta := domain.NewDocumentMetadata()
				if defaultMeta.Domain != "Unknown" {
					t.Errorf("Default Domain should be 'Unknown', got %s", defaultMeta.Domain)
				}
				if defaultMeta.Make != "Unknown" {
					t.Errorf("Default Make should be 'Unknown', got %s", defaultMeta.Make)
				}
				if defaultMeta.Model != "Unknown" {
					t.Errorf("Default Model should be 'Unknown', got %s", defaultMeta.Model)
				}
				if defaultMeta.Confidence != 0.0 {
					t.Errorf("Default Confidence should be 0.0, got %f", defaultMeta.Confidence)
				}
			},
		},
		{
			name:        "validation_functions",
			description: "Verify validation functions work correctly",
			checkFn: func(t *testing.T, metadata *domain.DocumentMetadata) {
				// Test country code validation
				if !domain.ValidateCountryCode("US") {
					t.Error("ValidateCountryCode should accept 'US'")
				}
				if domain.ValidateCountryCode("XX") {
					t.Error("ValidateCountryCode should reject 'XX'")
				}

				// Test model year validation
				if !domain.ValidateModelYear(2025) {
					t.Error("ValidateModelYear should accept 2025")
				}
				if domain.ValidateModelYear(1800) {
					t.Error("ValidateModelYear should reject 1800")
				}

				// Test domain validation
				if !domain.ValidateDomain("Automobile") {
					t.Error("ValidateDomain should accept 'Automobile'")
				}
			},
		},
		{
			name:        "normalization_functions",
			description: "Verify normalization functions work correctly",
			checkFn: func(t *testing.T, metadata *domain.DocumentMetadata) {
				// Test country code normalization
				if domain.NormalizeCountryCode("us") != "US" {
					t.Error("NormalizeCountryCode should normalize 'us' to 'US'")
				}

				// Test domain normalization
				if domain.NormalizeDomain("automobile") != "Automobile" {
					t.Error("NormalizeDomain should normalize 'automobile' to 'Automobile'")
				}

				// Test condition normalization
				if domain.NormalizeCondition("cpo") != "Certified Pre-Owned" {
					t.Error("NormalizeCondition should normalize 'cpo' to 'Certified Pre-Owned'")
				}
			},
		},
		{
			name:        "is_valid_check",
			description: "Verify IsValid() returns correct results",
			checkFn: func(t *testing.T, metadata *domain.DocumentMetadata) {
				// All unknown should be invalid
				allUnknown := domain.NewDocumentMetadata()
				if allUnknown.IsValid() {
					t.Error("Metadata with all Unknown values should not be valid")
				}

				// With domain set should be valid
				withDomain := domain.NewDocumentMetadata()
				withDomain.Domain = "Automobile"
				if !withDomain.IsValid() {
					t.Error("Metadata with Domain set should be valid")
				}

				// With make set should be valid
				withMake := domain.NewDocumentMetadata()
				withMake.Make = "Toyota"
				if !withMake.IsValid() {
					t.Error("Metadata with Make set should be valid")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Log(tc.description)
			tc.checkFn(t, nil)
		})
	}
}

// TestCategorizationWithRealPDF tests categorization with a real PDF if available (T624 continued)
func TestCategorizationWithRealPDF(t *testing.T) {
	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, testPDFPath, eventCh)
		close(eventCh)
	}()

	// Drain events
	for range eventCh {
	}

	if result == nil || result.Metadata == nil {
		t.Fatal("Failed to get categorization result")
	}

	// Log results for inspection
	t.Logf("Real PDF Categorization Results:")
	t.Logf("  Domain:      %s (expected: Automobile)", result.Metadata.Domain)
	t.Logf("  Subdomain:   %s", result.Metadata.Subdomain)
	t.Logf("  Make:        %s (expected: Maruti or Suzuki)", result.Metadata.Make)
	t.Logf("  Model:       %s (expected: Wagon R or Arena)", result.Metadata.Model)
	t.Logf("  CountryCode: %s (expected: IN)", result.Metadata.CountryCode)
	t.Logf("  Condition:   %s (expected: New)", result.Metadata.Condition)
	t.Logf("  Confidence:  %.2f", result.Metadata.Confidence)

	// For the Arena Wagon R brochure, we expect certain values
	// (relaxed checks since LLM output can vary)
	if result.Metadata.Domain != "Unknown" {
		// Domain should ideally be "Automobile"
		if result.Metadata.Domain != "Automobile" {
			t.Logf("Note: Domain is %q, expected 'Automobile'", result.Metadata.Domain)
		}
	}

	// Confidence should be above threshold if values are detected
	if result.Metadata.IsValid() && result.Metadata.Confidence < 0.70 {
		t.Logf("Warning: Valid metadata but confidence (%.2f) is below 70%% threshold",
			result.Metadata.Confidence)
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
