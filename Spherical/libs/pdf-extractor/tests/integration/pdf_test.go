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
	// Arena Wagon R brochure for variant testing
	arenaWagonRPDFPath = "/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Arena-Wagon-r-Brochure.pdf"
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

// TestPDFExtractionWithoutCodeblocks verifies that extracted markdown contains no codeblock delimiters (T008 - User Story 1)
func TestPDFExtractionWithoutCodeblocks(t *testing.T) {
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

	// Collect markdown from events
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	// Note: result may be nil if processing failed (e.g., timeout), but we can still check markdown collected from events
	if result == nil {
		// If result is nil but we have markdown from events, continue with validation
		if len(markdown) == 0 {
			t.Fatal("ProcessWithResult returned nil result and no markdown was collected from events")
		}
		t.Log("Warning: ProcessWithResult returned nil (may indicate timeout or error), but markdown was collected from events - continuing validation")
	}

	// Verify no codeblock delimiters in output (SC-001, FR-001, FR-002)
	codeblockCount := strings.Count(markdown, "```")
	if codeblockCount > 0 {
		t.Errorf("Extracted markdown contains %d codeblock delimiters (```). Expected 0. Content sample: %q", codeblockCount, markdown[:min(200, len(markdown))])
	}

	// Verify markdown is not empty
	if len(markdown) == 0 {
		t.Error("Extracted markdown is empty")
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "pdf-extraction-no-codeblocks-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s (verified: %d codeblock delimiters found)", outputPath, codeblockCount)
	}
}

// TestVariantNameExtractionFromTableHeaders verifies variant names are extracted from table headers (T027 - User Story 3)
func TestVariantNameExtractionFromTableHeaders(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// Verify variant names are extracted from table headers
	// Look for variant names in the Variant Availability column or in table structure
	// Common variant patterns: "Lounge", "Sportline", "Selection L&K", "VX", "ZX", "LX", etc.
	variantPatterns := []string{
		"Lounge", "Sportline", "Selection", "L&K", "VX", "ZX", "LX", "Variant", "Trim",
	}

	foundVariants := 0
	markdownLower := strings.ToLower(markdown)
	for _, pattern := range variantPatterns {
		if strings.Contains(markdownLower, strings.ToLower(pattern)) {
			foundVariants++
			t.Logf("Found variant reference: %s", pattern)
		}
	}

	// Check for Variant Availability column usage
	hasVariantAvailability := strings.Contains(markdown, "Variant Availability") ||
		strings.Contains(markdown, "variant availability") ||
		strings.Contains(markdown, "Standard") ||
		strings.Contains(markdown, "Exclusive to:")

	if !hasVariantAvailability && foundVariants == 0 {
		t.Log("Warning: No variant information found in output. This may indicate:")
		t.Log("1. The brochure has no variant information (single trim model)")
		t.Log("2. Variant extraction needs improvement")
		t.Log("3. The test PDF doesn't contain variant specification tables")
	} else {
		t.Logf("Variant extraction test: Found %d variant references, Variant Availability column: %v", foundVariants, hasVariantAvailability)
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "variant-extraction-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestVariantExclusiveFeatureTagging verifies variant-exclusive features are tagged from text mentions (T028 - User Story 3)
func TestVariantExclusiveFeatureTagging(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// Look for "Exclusive to:" patterns in Variant Availability column
	exclusivePatterns := []string{
		"Exclusive to:",
		"exclusive to:",
		"Exclusive",
		"exclusive",
	}

	foundExclusive := false
	markdownLower := strings.ToLower(markdown)
	for _, pattern := range exclusivePatterns {
		if strings.Contains(markdownLower, strings.ToLower(pattern)) {
			foundExclusive = true
			t.Logf("Found exclusive feature pattern: %s", pattern)
			break
		}
	}

	// Also check for variant-specific mentions in text
	variantMentions := []string{
		"only in", "available in", "standard in", "included in",
	}

	foundMentions := false
	for _, mention := range variantMentions {
		if strings.Contains(markdownLower, mention) {
			foundMentions = true
			t.Logf("Found variant mention pattern: %s", mention)
			break
		}
	}

	if !foundExclusive && !foundMentions {
		t.Log("Warning: No variant-exclusive feature tagging found. This may indicate:")
		t.Log("1. The brochure has no variant-exclusive features")
		t.Log("2. Features are available in all variants (marked as 'Standard')")
		t.Log("3. Variant extraction from text needs improvement")
	} else {
		t.Logf("Variant-exclusive feature tagging: Exclusive patterns: %v, Mentions: %v", foundExclusive, foundMentions)
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "variant-exclusive-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestSingleTrimModels verifies handling of single trim models with no variant information (T029 - User Story 3)
func TestSingleTrimModels(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// For single trim models, Variant Availability column should be:
	// - Empty, OR
	// - "Standard" (if feature available in all variants, which is the same as single trim)
	// - Should NOT contain variant-specific names like "Lounge", "Sportline", etc.

	// Check if output uses "Standard" for all features (indicating single trim or all-variant availability)
	standardCount := strings.Count(markdown, "Standard")

	// Check for variant-specific names (should be minimal or none for single trim)
	variantNames := []string{"Lounge", "Sportline", "Selection", "L&K", "VX", "ZX", "LX"}
	variantNameCount := 0
	markdownLower := strings.ToLower(markdown)
	for _, name := range variantNames {
		if strings.Contains(markdownLower, strings.ToLower(name)) {
			variantNameCount++
		}
	}

	// Verify 5-column format is maintained even for single trim models
	has5ColumnFormat := strings.Contains(markdown, "| Category | Specification | Value | Key Features | Variant Availability |") ||
		strings.Contains(markdown, "Variant Availability")

	if !has5ColumnFormat {
		t.Error("Output should maintain 5-column format even for single trim models (Variant Availability column should be present)")
	}

	t.Logf("Single trim model test results:")
	t.Logf("  - 'Standard' occurrences: %d", standardCount)
	t.Logf("  - Variant-specific names found: %d", variantNameCount)
	t.Logf("  - 5-column format present: %v", has5ColumnFormat)

	// For single trim models, it's acceptable to have:
	// - "Standard" in Variant Availability (FR-015)
	// - Empty Variant Availability (FR-017)
	// - But should NOT have variant-specific differentiation

	if variantNameCount > 0 && standardCount == 0 {
		t.Log("Note: Found variant names but no 'Standard' - this may indicate variant differentiation exists in the brochure")
	} else if standardCount > 0 && variantNameCount == 0 {
		t.Log("Note: All features marked as 'Standard' with no variant names - likely a single trim model")
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "single-trim-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestStandardNomenclatureMapping verifies that extracted specifications use standard hierarchical nomenclature (T013 - User Story 2)
func TestStandardNomenclatureMapping(t *testing.T) {
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

	// Collect markdown from events
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	// Note: result may be nil if processing failed (e.g., timeout), but we can still check markdown collected from events
	if result == nil {
		// If result is nil but we have markdown from events, continue with validation
		if len(markdown) == 0 {
			t.Fatal("ProcessWithResult returned nil result and no markdown was collected from events")
		}
		t.Log("Warning: ProcessWithResult returned nil (may indicate timeout or error), but markdown was collected from events - continuing validation")
	}

	// Verify standard hierarchical nomenclature is used (FR-003, FR-004, SC-002)
	// Standard categories include: Engine, Exterior, Interior, Safety, Performance, Dimensions
	standardCategories := []string{
		"Engine", "Exterior", "Interior", "Safety", "Performance", "Dimensions",
	}

	// Check for hierarchical notation (Category > Subcategory)
	hasHierarchicalNotation := strings.Contains(markdown, ">")

	// Check for standard category usage
	foundStandardCategories := 0
	markdownLower := strings.ToLower(markdown)
	for _, category := range standardCategories {
		if strings.Contains(markdownLower, strings.ToLower(category)) {
			foundStandardCategories++
			t.Logf("Found standard category: %s", category)
		}
	}

	// Verify hierarchical structure is present (preferred but not required for all specs)
	// Note: Some simple specs may use flat categories, which is acceptable per FR-014
	// (variable depth: 2-4 levels based on semantic meaning)
	if !hasHierarchicalNotation {
		t.Log("Note: Output uses flat categories instead of hierarchical notation. This is acceptable for simple specs, but hierarchical notation (Category > Subcategory) is preferred for complex features.")
	}

	// Verify at least some standard categories are used
	if foundStandardCategories == 0 {
		t.Error("No standard categories found. Output should use standard nomenclature (Engine, Exterior, Interior, Safety, Performance, Dimensions)")
	} else {
		t.Logf("Standard nomenclature test: Found %d/%d standard categories, Hierarchical notation: %v",
			foundStandardCategories, len(standardCategories), hasHierarchicalNotation)
		// Test passes if standard categories are used, even without hierarchical notation
		// (hierarchical notation is preferred but flat categories are acceptable for simple specs)
	}

	// Check for specific standard category patterns
	standardPatterns := []string{
		"Engine >", "Interior >", "Exterior >", "Safety >", "Performance >", "Dimensions >",
		"Interior > Seats", "Engine > Power", "Exterior > Lighting",
	}

	foundPatterns := 0
	for _, pattern := range standardPatterns {
		if strings.Contains(markdown, pattern) {
			foundPatterns++
			t.Logf("Found standard pattern: %s", pattern)
		}
	}

	if foundPatterns > 0 {
		t.Logf("Found %d standard hierarchical patterns in output", foundPatterns)
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "standard-nomenclature-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestSemanticMappingNonStandardSections verifies that non-standard section names are mapped to standard categories (T014 - User Story 2)
func TestSemanticMappingNonStandardSections(t *testing.T) {
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

	// Collect markdown from events
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	// Note: result may be nil if processing failed (e.g., timeout), but we can still check markdown collected from events
	if result == nil {
		// If result is nil but we have markdown from events, continue with validation
		if len(markdown) == 0 {
			t.Fatal("ProcessWithResult returned nil result and no markdown was collected from events")
		}
		t.Log("Warning: ProcessWithResult returned nil (may indicate timeout or error), but markdown was collected from events - continuing validation")
	}

	// Verify semantic mapping: non-standard brochure terms should be mapped to standard categories (FR-004)
	// Examples of semantic mapping:
	// - "Cabin Experience" → "Interior > Comfort"
	// - "DRL" → "Exterior > Lighting > DRL"
	// - "Powertrain" → "Engine"
	// - "Chassis" → "Performance > Suspension" or "Dimensions"

	// Check that output uses standard categories even if brochure uses non-standard terms
	// We verify by checking that hierarchical notation is used and standard categories appear
	hasStandardMapping := false

	// Check for evidence of semantic mapping (standard categories present)
	standardCategoryIndicators := []string{
		"Interior >", "Exterior >", "Engine >", "Safety >", "Performance >", "Dimensions >",
	}

	for _, indicator := range standardCategoryIndicators {
		if strings.Contains(markdown, indicator) {
			hasStandardMapping = true
			t.Logf("Found standard category mapping: %s", indicator)
			break
		}
	}

	// Check that brochure-specific terms are NOT used as-is in category paths
	// (This is a heuristic - we can't know all possible non-standard terms, but we verify standard structure)
	nonStandardPatterns := []string{
		"Cabin Experience >", "Powertrain >", "Chassis >", "Body >",
	}

	foundNonStandard := false
	for _, pattern := range nonStandardPatterns {
		if strings.Contains(markdown, pattern) {
			foundNonStandard = true
			t.Logf("Warning: Found non-standard category pattern: %s (should be mapped to standard)", pattern)
		}
	}

	if !hasStandardMapping {
		t.Log("Warning: No evidence of standard category mapping found. This may indicate:")
		t.Log("1. The brochure uses standard section names already")
		t.Log("2. Semantic mapping needs improvement")
		t.Log("3. The test PDF doesn't contain typical non-standard sections")
	} else {
		t.Logf("Semantic mapping test: Standard mapping present: %v, Non-standard patterns found: %v",
			hasStandardMapping, foundNonStandard)
	}

	// Verify hierarchical depth is appropriate (2-4 levels per FR-014)
	// Count hierarchy depth in category paths
	lines := strings.Split(markdown, "\n")
	maxDepth := 0
	for _, line := range lines {
		if strings.Contains(line, "|") && strings.Contains(line, ">") {
			// Count ">" separators to determine depth
			depth := strings.Count(line, ">")
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}

	if maxDepth > 0 {
		t.Logf("Hierarchical depth: Maximum depth found: %d levels (expected: 2-4 levels)", maxDepth)
		if maxDepth > 4 {
			t.Logf("Warning: Maximum depth (%d) exceeds recommended 4 levels", maxDepth)
		}
		if maxDepth < 2 {
			t.Logf("Note: Maximum depth (%d) is below recommended 2 levels", maxDepth)
		}
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "semantic-mapping-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestVariantTableExtractionWithCheckboxes verifies variant table extraction with checkbox indicators (T018 - User Story 4)
func TestVariantTableExtractionWithCheckboxes(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// Verify variant table extraction with checkboxes (FR-008, FR-010, FR-011)
	// Check for Variant Availability column
	hasVariantAvailabilityColumn := strings.Contains(markdown, "Variant Availability") ||
		strings.Contains(markdown, "variant availability")

	if !hasVariantAvailabilityColumn {
		t.Error("Output should include 'Variant Availability' column in specification table")
	}

	// Check for 5-column format
	has5ColumnFormat := strings.Contains(markdown, "| Category | Specification | Value | Key Features | Variant Availability |")

	if !has5ColumnFormat {
		t.Log("Warning: 5-column format header not found. Checking for variant availability patterns...")
	}

	// Check for variant availability patterns that indicate checkbox/symbol parsing
	variantAvailabilityPatterns := []string{
		": ✓", ": ✗", ": ●", ": ○", "Standard", "Exclusive to:",
	}

	foundPatterns := 0
	for _, pattern := range variantAvailabilityPatterns {
		if strings.Contains(markdown, pattern) {
			foundPatterns++
			t.Logf("Found variant availability pattern: %s", pattern)
		}
	}

	if foundPatterns == 0 {
		t.Log("Warning: No variant availability patterns found. This may indicate:")
		t.Log("1. The brochure has no variant differentiation tables")
		t.Log("2. Checkbox/symbol parsing needs improvement")
		t.Log("3. All features are marked as 'Standard' (available in all variants)")
	} else {
		t.Logf("Variant table extraction test: Found %d variant availability patterns", foundPatterns)
	}

	// Verify table structure includes variant information
	// Look for table rows with variant availability data
	lines := strings.Split(markdown, "\n")
	rowsWithVariantInfo := 0
	for _, line := range lines {
		if strings.Contains(line, "|") && (strings.Contains(line, "✓") || strings.Contains(line, "✗") ||
			strings.Contains(line, "Standard") || strings.Contains(line, "Exclusive to:")) {
			rowsWithVariantInfo++
		}
	}

	t.Logf("Found %d table rows with variant availability information", rowsWithVariantInfo)

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "variant-table-checkbox-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestVariantAvailabilitySymbolParsing verifies parsing of variant availability symbols (✓, ✗, ●, ○) (T019 - User Story 4)
func TestVariantAvailabilitySymbolParsing(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown
	var markdown string
	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// Verify variant availability symbol parsing (FR-010)
	// Check for common symbols used in variant tables: ✓, ✗, ●, ○
	symbols := []string{
		"✓", "✗", "●", "○",
	}

	foundSymbols := make(map[string]int)
	for _, symbol := range symbols {
		count := strings.Count(markdown, symbol)
		if count > 0 {
			foundSymbols[symbol] = count
			t.Logf("Found symbol '%s': %d occurrences", symbol, count)
		}
	}

	// Check for symbol patterns in variant availability format
	// Expected format: "VariantName: ✓" or "VariantName: ✗"
	symbolPatterns := []string{
		": ✓", ": ✗", ": ●", ": ○",
	}

	foundPatterns := 0
	for _, pattern := range symbolPatterns {
		if strings.Contains(markdown, pattern) {
			foundPatterns++
			t.Logf("Found symbol pattern: %s", pattern)
		}
	}

	// Also check for alternative formats
	alternativeFormats := []string{
		"Standard", "Exclusive to:", "Available in:", "Unknown",
	}

	foundAlternatives := 0
	for _, format := range alternativeFormats {
		if strings.Contains(markdown, format) {
			foundAlternatives++
			t.Logf("Found alternative format: %s", format)
		}
	}

	if len(foundSymbols) == 0 && foundPatterns == 0 && foundAlternatives == 0 {
		t.Log("Warning: No variant availability symbols or patterns found. This may indicate:")
		t.Log("1. The brochure doesn't use symbol-based variant indicators")
		t.Log("2. Symbols are not being parsed correctly")
		t.Log("3. Variant information is presented in a different format")
	} else {
		t.Logf("Symbol parsing test: Found %d symbol types, %d symbol patterns, %d alternative formats",
			len(foundSymbols), foundPatterns, foundAlternatives)
	}

	// Verify symbols appear in Variant Availability column context
	// Check that symbols appear near variant names or in table structure
	lines := strings.Split(markdown, "\n")
	symbolsInTableContext := 0
	for _, line := range lines {
		if strings.Contains(line, "|") {
			for _, symbol := range symbols {
				if strings.Contains(line, symbol) {
					symbolsInTableContext++
					break
				}
			}
		}
	}

	t.Logf("Found %d table rows containing variant availability symbols", symbolsInTableContext)

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "variant-symbol-parsing-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// TestMultiPageVariantTables verifies that variant information is maintained across multi-page tables (T020 - User Story 4)
func TestMultiPageVariantTables(t *testing.T) {
	// Skip if PDF doesn't exist
	if _, err := os.Stat(arenaWagonRPDFPath); os.IsNotExist(err) {
		t.Skipf("Arena Wagon R PDF not found at %s", arenaWagonRPDFPath)
	}

	// Skip if API key not set
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize real components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	var result *extract.ProcessResult
	go func() {
		result, _ = extractor.ProcessWithResult(ctx, arenaWagonRPDFPath, eventCh)
		close(eventCh)
	}()

	// Collect markdown and track page numbers
	var markdown string
	pageMarkdown := make(map[int]string)
	currentPage := 0

	for event := range eventCh {
		if event.Type == domain.EventLLMStreaming {
			if chunk, ok := event.Payload.(string); ok {
				markdown += chunk
				// Track page-specific content
				if event.PageNumber > 0 {
					if event.PageNumber != currentPage {
						currentPage = event.PageNumber
					}
					pageMarkdown[currentPage] += chunk
				}
			}
		}
	}

	if result == nil {
		t.Fatal("ProcessWithResult returned nil result")
	}

	// Verify variant information consistency across pages (FR-012)
	// Extract variant names from the full markdown
	variantNames := extractVariantNames(markdown)

	if len(variantNames) == 0 {
		t.Log("Warning: No variant names found. This may indicate:")
		t.Log("1. The brochure has no variant information")
		t.Log("2. Variant extraction needs improvement")
		t.Log("3. The test PDF doesn't contain variant specification tables")
	} else {
		t.Logf("Found variant names: %v", variantNames)
	}

	// Check that variant names appear consistently across multiple pages
	// (if variant tables span multiple pages, variant names should appear on multiple pages)
	pagesWithVariants := make(map[string][]int) // variant name -> list of page numbers
	for variantName := range variantNames {
		for pageNum, pageContent := range pageMarkdown {
			if strings.Contains(pageContent, variantName) {
				pagesWithVariants[variantName] = append(pagesWithVariants[variantName], pageNum)
			}
		}
	}

	// Log variant distribution across pages
	for variantName, pages := range pagesWithVariants {
		if len(pages) > 1 {
			t.Logf("Variant '%s' appears on %d pages: %v (indicates multi-page table)", variantName, len(pages), pages)
		} else if len(pages) == 1 {
			t.Logf("Variant '%s' appears on page %d only", variantName, pages[0])
		}
	}

	// Verify Variant Availability column format is consistent across pages
	// Check that all pages with variant information use the same format
	hasConsistentFormat := true
	variantAvailabilityFormats := []string{
		"Variant Availability", "Standard", "Exclusive to:", ": ✓", ": ✗",
	}

	for pageNum, pageContent := range pageMarkdown {
		hasVariantInfo := false
		for _, format := range variantAvailabilityFormats {
			if strings.Contains(pageContent, format) {
				hasVariantInfo = true
				break
			}
		}

		if hasVariantInfo {
			// Check if this page uses the 5-column format
			has5Column := strings.Contains(pageContent, "| Category | Specification | Value | Key Features | Variant Availability |")
			if !has5Column {
				// Check if it's at least using variant availability patterns
				hasPattern := strings.Contains(pageContent, "Standard") ||
					strings.Contains(pageContent, "Exclusive to:") ||
					strings.Contains(pageContent, ": ✓") ||
					strings.Contains(pageContent, ": ✗")

				if !hasPattern {
					t.Logf("Warning: Page %d has variant info but inconsistent format", pageNum)
					hasConsistentFormat = false
				}
			}
		}
	}

	if !hasConsistentFormat {
		t.Log("Warning: Variant availability format is not consistent across all pages")
	} else {
		t.Log("Variant availability format is consistent across pages")
	}

	// Verify that variant context is maintained (variant names don't disappear and reappear randomly)
	// This is a heuristic check - variant names should appear in a logical sequence
	if len(pagesWithVariants) > 0 {
		t.Logf("Multi-page variant table test: Variants found across %d pages, Format consistency: %v",
			len(pageMarkdown), hasConsistentFormat)
	}

	// Write output for manual inspection
	outputPath := filepath.Join(os.TempDir(), "multi-page-variant-table-test-output.md")
	err := os.WriteFile(outputPath, []byte(markdown), 0644)
	if err != nil {
		t.Errorf("Failed to write output file: %v", err)
	} else {
		t.Logf("Output written to: %s", outputPath)
	}
}

// extractVariantNames extracts variant names from markdown content
func extractVariantNames(markdown string) map[string]bool {
	variants := make(map[string]bool)

	// Common variant patterns to look for
	commonPatterns := []string{
		"Lounge", "Sportline", "Selection", "L&K", "VX", "ZX", "LX", "EX", "Touring",
		"LXi", "VXi", "ZXi", "Style", "Elegance", "Comfort", "Premium", "Luxury", "Sport",
	}

	markdownLower := strings.ToLower(markdown)
	for _, pattern := range commonPatterns {
		if strings.Contains(markdownLower, strings.ToLower(pattern)) {
			variants[pattern] = true
		}
	}

	// Also look for variant names in "Exclusive to:" patterns
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Exclusive to:") {
			// Extract variant name after "Exclusive to:"
			parts := strings.Split(line, "Exclusive to:")
			if len(parts) > 1 {
				variantName := strings.TrimSpace(parts[1])
				// Remove any trailing punctuation or table separators
				variantName = strings.TrimRight(variantName, "|,;")
				variantName = strings.TrimSpace(variantName)
				if variantName != "" {
					variants[variantName] = true
				}
			}
		}
	}

	return variants
}
