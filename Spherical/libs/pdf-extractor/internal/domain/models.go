package domain

import (
	"regexp"
	"strings"
	"time"
)

// Predefined list of supported domains for categorization
var SupportedDomains = []string{
	"Automobile",
	"Real Estate",
	"Luxury Watch",
	"Jewelry",
	"Electronics",
	"Fashion",
	"Furniture",
	"Art",
	"Collectibles",
	"Other",
}

// ISO 3166-1 alpha-2 country codes (common subset)
var ValidCountryCodes = map[string]string{
	"IN": "India",
	"US": "United States",
	"UK": "United Kingdom",
	"GB": "United Kingdom",
	"DE": "Germany",
	"FR": "France",
	"JP": "Japan",
	"CN": "China",
	"AU": "Australia",
	"CA": "Canada",
	"IT": "Italy",
	"ES": "Spain",
	"BR": "Brazil",
	"MX": "Mexico",
	"KR": "South Korea",
	"RU": "Russia",
	"AE": "United Arab Emirates",
	"SA": "Saudi Arabia",
	"SG": "Singapore",
	"TH": "Thailand",
	"MY": "Malaysia",
	"ID": "Indonesia",
	"NL": "Netherlands",
	"SE": "Sweden",
	"CH": "Switzerland",
	"AT": "Austria",
	"BE": "Belgium",
	"PL": "Poland",
	"ZA": "South Africa",
	"NZ": "New Zealand",
}

// Document represents the source PDF file being processed
type Document struct {
	FilePath   string
	TotalPages int
}

// PageImage represents a single converted PDF page
type PageImage struct {
	PageNumber int
	ImagePath  string // Path to temporary JPG file
	Width      int
	Height     int
}

// Specification represents a single product specification item
type Specification struct {
	Category string `json:"category"` // e.g. "Dimensions", "Power"
	Name     string `json:"name"`
	Value    string `json:"value"`
}

// ExtractionResult contains the structured data extracted from pages
type ExtractionResult struct {
	Specifications []Specification   `json:"specifications"`
	Features       []string          `json:"features"`
	USPs           []string          `json:"usps"`
	RawMarkdown    string            `json:"raw_markdown"` // Full markdown output from LLM
	Metadata       *DocumentMetadata `json:"metadata,omitempty"` // Document categorization metadata (FR-016)
}

// EventType represents the type of stream event
type EventType string

const (
	EventStart          EventType = "start"
	EventPageProcessing EventType = "page_processing"
	EventLLMStreaming   EventType = "llm_streaming" // Chunk of text
	EventPageComplete   EventType = "page_complete"
	EventError          EventType = "error"
	EventComplete       EventType = "complete"
)

// StreamEvent represents an event emitted during processing
type StreamEvent struct {
	Type       EventType   `json:"type"`
	PageNumber int         `json:"page_number,omitempty"`
	Payload    interface{} `json:"payload,omitempty"` // Text chunk or status message
	Timestamp  time.Time   `json:"timestamp"`
}

// ProcessingStats contains metadata about the extraction execution
type ProcessingStats struct {
	TotalTime       time.Duration
	PagesProcessed  int
	SuccessfulPages int
	FailedPages     int
	Errors          []error
}

// DocumentMetadata contains document categorization metadata (FR-016)
type DocumentMetadata struct {
	Domain      string  `json:"domain"`       // e.g., "Automobile", "Real Estate", "Luxury Watch"
	Subdomain   string  `json:"subdomain"`    // e.g., "Commercial", "Consumer"
	CountryCode string  `json:"country_code"` // ISO 3166-1 alpha-2 code, e.g., "IN", "US"
	ModelYear   int     `json:"model_year"`   // e.g., 2025, 2024
	Condition   string  `json:"condition"`    // e.g., "New", "Used", "Secondary Resale"
	Make        string  `json:"make"`         // e.g., "Toyota", "Volvo"
	Model       string  `json:"model"`        // e.g., "Camry", "XC90"
	Confidence  float64 `json:"confidence"`   // Overall confidence score (0.0-1.0)
}

// FieldConfidence tracks confidence for individual categorization fields
type FieldConfidence struct {
	Domain      float64 `json:"domain"`
	Subdomain   float64 `json:"subdomain"`
	CountryCode float64 `json:"country_code"`
	ModelYear   float64 `json:"model_year"`
	Condition   float64 `json:"condition"`
	Make        float64 `json:"make"`
	Model       float64 `json:"model"`
}

// NewDocumentMetadata creates a new DocumentMetadata with default "Unknown" values
func NewDocumentMetadata() *DocumentMetadata {
	return &DocumentMetadata{
		Domain:      "Unknown",
		Subdomain:   "Unknown",
		CountryCode: "Unknown",
		ModelYear:   0,
		Condition:   "Unknown",
		Make:        "Unknown",
		Model:       "Unknown",
		Confidence:  0.0,
	}
}

// IsValid checks if the metadata has at least some valid fields
func (m *DocumentMetadata) IsValid() bool {
	return m.Domain != "Unknown" || m.Make != "Unknown" || m.Model != "Unknown"
}

// ValidateCountryCode checks if a country code is valid ISO 3166-1 alpha-2
func ValidateCountryCode(code string) bool {
	if code == "" || code == "Unknown" {
		return true // Unknown is acceptable
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	_, exists := ValidCountryCodes[code]
	return exists
}

// NormalizeCountryCode normalizes a country code to uppercase
func NormalizeCountryCode(code string) string {
	if code == "" {
		return "Unknown"
	}
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if _, exists := ValidCountryCodes[normalized]; exists {
		return normalized
	}
	return "Unknown"
}

// ValidateModelYear checks if a model year is within reasonable range (1900-2100)
func ValidateModelYear(year int) bool {
	return year == 0 || (year >= 1900 && year <= 2100)
}

// ValidateDomain checks if a domain is in the predefined list
func ValidateDomain(domain string) bool {
	if domain == "" || domain == "Unknown" {
		return true
	}
	domain = strings.TrimSpace(domain)
	for _, d := range SupportedDomains {
		if strings.EqualFold(d, domain) {
			return true
		}
	}
	return false
}

// NormalizeDomain normalizes a domain string to match predefined list
func NormalizeDomain(domain string) string {
	if domain == "" {
		return "Unknown"
	}
	domain = strings.TrimSpace(domain)
	for _, d := range SupportedDomains {
		if strings.EqualFold(d, domain) {
			return d
		}
	}
	return domain // Return as-is if not in predefined list
}

// ValidateSubdomain checks if a subdomain follows common conventions
func ValidateSubdomain(subdomain string) bool {
	if subdomain == "" || subdomain == "Unknown" {
		return true
	}
	// Subdomain should be alphanumeric with spaces, no special characters
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9\s\-]+$`, subdomain)
	return matched
}

// ValidateCondition checks if a condition follows common conventions
func ValidateCondition(condition string) bool {
	if condition == "" || condition == "Unknown" {
		return true
	}
	// Common condition values
	validConditions := []string{"New", "Used", "Secondary Resale", "Certified Pre-Owned", "Refurbished"}
	condition = strings.TrimSpace(condition)
	for _, c := range validConditions {
		if strings.EqualFold(c, condition) {
			return true
		}
	}
	// Also accept free-form if it's alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9\s\-]+$`, condition)
	return matched
}

// NormalizeCondition normalizes a condition string
func NormalizeCondition(condition string) string {
	if condition == "" {
		return "Unknown"
	}
	condition = strings.TrimSpace(condition)
	// Normalize common conditions
	switch strings.ToLower(condition) {
	case "new":
		return "New"
	case "used":
		return "Used"
	case "secondary resale", "resale":
		return "Secondary Resale"
	case "certified pre-owned", "cpo":
		return "Certified Pre-Owned"
	case "refurbished":
		return "Refurbished"
	default:
		return condition
	}
}




