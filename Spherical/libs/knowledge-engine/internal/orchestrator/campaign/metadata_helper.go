package campaign

import (
	"fmt"
	"strconv"
	"strings"

	pdfextractor "github.com/spherical/pdf-extractor/pkg/extractor"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
)

// MetadataField represents a single metadata field that can be prompted.
type MetadataField struct {
	Key         string
	Label       string
	Value       string
	Required    bool
	Prompt      func(label string) (string, error)
	Validate    func(value string) error
}

// CompleteMetadata prompts the user to fill in any missing or "Unknown" metadata fields.
func CompleteMetadata(metadata *pdfextractor.DocumentMetadata) (*pdfextractor.DocumentMetadata, error) {
	if metadata == nil {
		metadata = &pdfextractor.DocumentMetadata{
			Domain:      "Unknown",
			Subdomain:   "Unknown",
			CountryCode: "Unknown",
			ModelYear:   0,
			Condition:   "Unknown",
			Make:        "Unknown",
			Model:       "Unknown",
		}
	}

	ui.Section("Document Information")
	ui.Info("Review the detected information and provide any missing details:")

	completed := *metadata // Copy the metadata

	// Define field mappings with prompts
	fields := []MetadataField{
		{
			Key:      "domain",
			Label:    "Product Category",
			Value:    completed.Domain,
			Required: true,
			Prompt:   func(label string) (string, error) { return ui.PromptRequired(fmt.Sprintf("%s (e.g., Automobile, Real Estate, Luxury Watch)", label)) },
			Validate: func(v string) error {
				v = strings.TrimSpace(v)
				if v == "" || v == "Unknown" {
					return fmt.Errorf("category is required")
				}
				return nil
			},
		},
		{
			Key:      "subdomain",
			Label:    "Product Sub-Category",
			Value:    completed.Subdomain,
			Required: false,
			Prompt:   func(label string) (string, error) { return ui.Prompt(fmt.Sprintf("%s (e.g., Commercial Vehicle, Consumer, press Enter to skip)", label)) },
			Validate: func(v string) error { return nil },
		},
		{
			Key:      "make",
			Label:    "Product Make",
			Value:    completed.Make,
			Required: true,
			Prompt:   func(label string) (string, error) { return ui.PromptRequired(fmt.Sprintf("%s (e.g., Toyota, Volvo)", label)) },
			Validate: func(v string) error {
				v = strings.TrimSpace(v)
				if v == "" || v == "Unknown" {
					return fmt.Errorf("make is required")
				}
				return nil
			},
		},
		{
			Key:      "model",
			Label:    "Product Model",
			Value:    completed.Model,
			Required: true,
			Prompt:   func(label string) (string, error) { return ui.PromptRequired(fmt.Sprintf("%s (e.g., Camry Hybrid, XC90)", label)) },
			Validate: func(v string) error {
				v = strings.TrimSpace(v)
				if v == "" || v == "Unknown" {
					return fmt.Errorf("model is required")
				}
				return nil
			},
		},
		{
			Key:      "condition",
			Label:    "Product Condition",
			Value:    completed.Condition,
			Required: false,
			Prompt:   func(label string) (string, error) {
				options := []string{"New", "Used", "Secondary Resale", "Other"}
				choice, err := ui.PromptChoice(fmt.Sprintf("%s (press Enter to skip)", label), options)
				if err != nil {
					return ui.Prompt(fmt.Sprintf("%s (e.g., New, Used, press Enter to skip)", label))
				}
				if choice >= 0 && choice < len(options) {
					return options[choice], nil
				}
				return ui.Prompt(fmt.Sprintf("%s (e.g., New, Used, press Enter to skip)", label))
			},
			Validate: func(v string) error { return nil },
		},
		{
			Key:      "model_year",
			Label:    "Product Year",
			Value:    formatYear(completed.ModelYear),
			Required: false,
			Prompt:   func(label string) (string, error) { return ui.Prompt(fmt.Sprintf("%s (e.g., 2025, press Enter to skip)", label)) },
			Validate: func(v string) error {
				if v == "" {
					return nil // Optional
				}
				year, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return fmt.Errorf("invalid year: %w", err)
				}
				if year < 1900 || year > 2100 {
					return fmt.Errorf("year must be between 1900 and 2100")
				}
				return nil
			},
		},
		{
			Key:      "country_code",
			Label:    "Product Region",
			Value:    formatCountryCode(completed.CountryCode),
			Required: false,
			Prompt:   func(label string) (string, error) { return ui.Prompt(fmt.Sprintf("%s (e.g., en-IN, en-US, press Enter to skip)", label)) },
			Validate: func(v string) error {
				// Basic validation - can be enhanced with ISO 3166-1 validation
				return nil
			},
		},
	}

	ui.Newline()
	
	// Display current values and prompt for missing/unknown fields
	for i, field := range fields {
		isUnknown := (field.Value == "Unknown" || field.Value == "" || (field.Key == "model_year" && field.Value == "0"))
		isRequired := field.Required

		if isUnknown || (isRequired && strings.TrimSpace(field.Value) == "") {
			// Display current detected value if available
			if field.Value != "Unknown" && field.Value != "" && field.Key != "model_year" {
				ui.Info("Detected %s: %s", field.Label, field.Value)
			} else {
				ui.Info("Detected %s: Not available", field.Label)
			}

			// Prompt for value
			var value string
			var err error

			for {
				value, err = field.Prompt(field.Label)
				if err != nil {
					return nil, fmt.Errorf("prompt for %s: %w", field.Key, err)
				}

				value = strings.TrimSpace(value)
				
				// Skip optional fields if empty
				if !isRequired && value == "" {
					break
				}

				// Validate value
				if err := field.Validate(value); err != nil {
					ui.Error("Invalid value: %v", err)
					continue // Re-prompt
				}

				if value != "" {
					break
				}
			}

			// Update the completed metadata
			if value != "" {
				switch field.Key {
				case "domain":
					completed.Domain = value
				case "subdomain":
					completed.Subdomain = value
				case "make":
					completed.Make = value
				case "model":
					completed.Model = value
				case "condition":
					completed.Condition = value
				case "model_year":
					if year, err := strconv.Atoi(value); err == nil {
						completed.ModelYear = year
					}
				case "country_code":
					completed.CountryCode = value
				}
			}
		} else {
			// Display confirmed value
			displayValue := field.Value
			if field.Key == "model_year" && displayValue == "0" {
				displayValue = "Not specified"
			}
			ui.Success("✓ %s: %s", field.Label, displayValue)
		}

		// Add spacing between fields except for the last one
		if i < len(fields)-1 {
			ui.Newline()
		}
	}

	ui.Newline()
	ui.Success("Document information complete!")

	return &completed, nil
}

// formatYear formats a year integer as a string.
func formatYear(year int) string {
	if year == 0 {
		return "0"
	}
	return strconv.Itoa(year)
}

// formatCountryCode formats country code, handling both formats (e.g., "IN" or "en-IN").
func formatCountryCode(code string) string {
	if code == "" || code == "Unknown" {
		return "Unknown"
	}
	return code
}

// BuildProductName constructs a product name from metadata.
func BuildProductName(metadata *pdfextractor.DocumentMetadata) string {
	parts := []string{}
	
	if metadata.Make != "Unknown" && metadata.Make != "" {
		parts = append(parts, metadata.Make)
	}
	if metadata.Model != "Unknown" && metadata.Model != "" {
		parts = append(parts, metadata.Model)
	}
	if metadata.ModelYear > 0 {
		parts = append(parts, fmt.Sprintf("%d", metadata.ModelYear))
	}
	
	if len(parts) == 0 {
		return "Unknown Product"
	}
	
	return strings.Join(parts, " ")
}

// BuildLocaleFromCountryCode extracts locale information from country code.
func BuildLocaleFromCountryCode(countryCode string) string {
	// Handle formats like "en-IN", "IN", "US", etc.
	parts := strings.Split(countryCode, "-")
	if len(parts) == 2 {
		// Format: "en-IN" -> use as is for locale
		return countryCode
	} else if len(parts) == 1 {
		// Format: "IN", "US" -> convert to locale format (simplified)
		// In a production system, you'd have a proper mapping
		return fmt.Sprintf("en-%s", strings.ToUpper(parts[0]))
	}
	
	// Default fallback
	return "en-US"
}

