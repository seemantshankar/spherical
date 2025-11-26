package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spherical/pdf-extractor/internal/domain"
)

const (
	openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
	defaultModel  = "google/gemini-2.5-flash-preview-09-2025"
)

// Client handles communication with OpenRouter API
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// Message represents a chat message
type Message struct {
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

// ContentPart represents a part of message content (text or image)
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image URL in the message
type ImageURL struct {
	URL string `json:"url"`
}

// Request represents the API request structure
type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// Response represents the API response structure
type Response struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

// Choice represents a single completion choice
type Choice struct {
	Delta        Delta  `json:"delta"`
	Message      Delta  `json:"message"`
	FinishReason string `json:"finish_reason"`
}

// Delta represents a message delta in streaming response
type Delta struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

// NewClient creates a new LLM client
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = defaultModel
	}

	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// Extract processes an image and streams the extracted markdown
func (c *Client) Extract(ctx context.Context, imagePath string, resultCh chan<- string) error {
	// Build request
	req, err := c.buildRequest(imagePath)
	if err != nil {
		return domain.APIError("Failed to build request", err)
	}

	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return domain.APIError("Failed to marshal request", err)
	}

	// Send request with retry logic
	resp, err := c.retryWithBackoff(ctx, func() (*http.Response, error) {
		// Clone the request body for each retry
		reqBody := bytes.NewReader(body)
		req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, reqBody)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("HTTP-Referer", "https://github.com/spherical/pdf-extractor")
		req.Header.Set("X-Title", "PDF Specification Extractor")

		return c.httpClient.Do(req)
	})

	if err != nil {
		return domain.APIError("Failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return domain.APIError(fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(bodyBytes)), nil)
	}

	// Parse streaming response
	return c.parseStream(resp.Body, resultCh)
}

// buildRequest constructs the API request with the image
func (c *Client) buildRequest(imagePath string) (*Request, error) {
	// Read and encode image
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)
	imageURL := "data:image/jpeg;base64," + base64Image

	// Build message
	msg := Message{
		Role: "user",
		Content: []ContentPart{
			{
				Type: "text",
				Text: buildPrompt(),
			},
			{
				Type: "image_url",
				ImageURL: &ImageURL{
					URL: imageURL,
				},
			},
		},
	}

	return &Request{
		Model:    c.model,
		Messages: []Message{msg},
		Stream:   true,
	}, nil
}

// buildPrompt creates the extraction prompt
func buildPrompt() string {
	return `You are a product specification extraction expert. Analyze this image from a product brochure or datasheet.

Extract and return ONLY the following information in Markdown format:

## Specifications
Create a table with ONLY technical product specifications. Use EXACTLY this 3-column format:
| Category | Specification | Value |
|----------|---------------|-------|
| Engine | Type | 1.2L Petrol |
| Engine | Power Output | 90 PS @ 6000 rpm |
| Dimensions | Length | 3655 mm |
| Fuel Efficiency | Petrol Variant | 24.43 km/l |

CRITICAL TABLE RULES:
- Each row must have EXACTLY 3 columns (Category | Specification | Value)
- NEVER create 4-column rows - if you have sub-categories, put them in the Specification column
- For multiple variants, create separate rows for each variant
- Example: "Maximum Power - Petrol 1.0L" goes in Specification column, "50.4 kW @ 5600 rpm" in Value column
- Keep tables simple and consistent
- Empty Category cells should be left empty (just |  |)

WHAT TO INCLUDE AS SPECIFICATIONS:
- Technical specifications (dimensions, weight, capacity, power, torque, etc.)
- Performance data (fuel efficiency, speed, acceleration, etc.)
- Features and equipment lists
- Variant information
- Safety features
- Exterior and interior color options (list all available colors)
- Trim levels and their associated features

WHAT TO EXCLUDE (DO NOT EXTRACT):
- Contact information (phone numbers, addresses, websites)
- Company names and branding
- Legal disclaimers and warranties
- Pricing information
- Dealer information
- Copyright notices
- Terms and conditions
- Footnote markers and references (remove all #, *, †, ‡, §, ¶, and similar footnote symbols from extracted text)
- Parenthetical footnote references like "(#)", "(*)", "(†)", etc.

NUMBER FORMATTING RULES (CRITICAL):
- Output plain numbers WITHOUT LaTeX formatting
- NEVER use $ signs for math mode (e.g., NO: $24.43^{\#}$)
- REMOVE all footnote markers (#, *, †, ‡, §, ¶) from numbers and text
- Example: "25.49 km/l" NOT "25.49 km/l*" or "25.49 km/l#"
- Example: "Floor Mat availability: In all Camry variants" NOT "Floor Mat availability: In all Camry variants (#)"
- For superscripts in original, just use plain text (remove superscript markers)
- Example: "335 litres" NOT "335^" or "$335^{\wedge}$"
- Keep measurements simple: "3655 mm" NOT "$3655$"

For complex tables:
- Break down into separate rows if needed
- For merged cells, create individual rows
- Maintain consistent 3-column structure throughout
- If data has multiple sub-values, combine them in the Value column

## Key Features
List all key features or highlights as bullet points.
- If no key features are present, DO NOT output this section at all
- Features are functional capabilities (e.g., "Adaptive Cruise Control", "Hill Assist")

## USPs (Unique Selling Points)
List ONLY true competitive advantages and unique selling propositions, written in persuasive marketing language.
- If no USPs are present, DO NOT output this section at all
- Treat USPs as the wow factors you would use to convince a buyer
- Reference the vehicle's segment/price point when deciding if something is truly special

WHAT ARE USPs:
- Best-in-class or segment-first stats (e.g., “Best-in-class fuel efficiency”, “First in segment with Level 2 ADAS”)
- Signature design elements or iconic styling cues (e.g., “Thor Hammer LED headlamps”, “Orrefors® crystal gear shifter”)
- Premium craftsmanship details that define the brand experience (e.g., “Hand-crafted Nappa leather with Swedish stitching patterns”)
- Innovative technologies or award-winning features not commonly available in the segment (e.g., “Pilot Assist semi-autonomous driving”, “Bowers & Wilkins 19-speaker studio audio”)
- Differentiators that justify the premium (e.g., “Air suspension with Four-C adaptive damping for limo-like comfort”)
- Iconic Swedish luxury cues (Crystal gear selector, Thor Hammer DRLs, Four-C air suspension, Bowers & Wilkins audio, panoramic roofs, massage seats, etc.)

WHAT ARE NOT USPs (put in Specifications or Key Features instead):
- Material or color options alone (unless they are iconic brand signatures; otherwise keep in Specs)
- Variant names (LX, VX, ZX, etc.)
- Standard equipment lists or safety basics (ABS, airbags, ISOFIX, etc.)
- Dimensions, measurements, or regular spec-sheet data
- Routine functional features that competitors also offer (ACC, Lane Keep Assist, Park Assist, etc.)

USP STYLE GUIDELINES:
- Use aspirational marketing phrasing (e.g., “Iconic Thor Hammer LED DRLs announce your arrival”)
- Combine feature + benefit (what it is + why it matters)
- Target 3-5 strong USPs if the content provides enough material; if premium cues like crystal gear knobs, Thor Hammer lighting, Bowers & Wilkins audio, panoramic roofs, Four-C air suspension, Nappa massage seats, etc. exist, you MUST output at least 2 USP bullets referencing them
- Proactively scan for signature design phrases (e.g., “Thor Hammer”, “Orrefors”, “Crystal”, “Signature”, “Iconic”, “Bowers”, “massage seats”, “air suspension”) and elevate them into USPs even if they originated in a table
- If the brochure highlights unique craftsmanship or technology cues, convert them into USPs even if they appeared under Specifications
- 1 bullet per USP, keep it punchy and persuasive, ending with a full sentence
- When you see “Crystal” and “gear” together, explicitly refer to the Orrefors-crafted crystal gear shifter and why it feels special
- When LED signature lighting like “LED Matrix headlights”, “Thor Hammer headlamps” or similar appear, describe them as the Thor’s Hammer signature lighting that defines Volvo’s design DNA
- When premium audio brands (e.g., “Bowers & Wilkins”) appear, highlight the concert-hall experience as a USP
- When air suspension, Four-C chassis, massage seats, panoramic roofs, or other halo comforts appear, highlight the serene Scandinavian experience they create

CRITICAL OUTPUT RULES:
- ONLY output sections that have actual content
- If a page has NO specifications, DO NOT output the ## Specifications section
- If a page has NO key features, DO NOT output the ## Key Features section  
- If a page has NO USPs, DO NOT output the ## USPs section
- NEVER output empty tables (header only with no data rows)
- NEVER output explanatory text like "(No features found)" or "(Not applicable)"
- If the entire page has no extractable content, output nothing (empty response)

FORMATTING RULES:
- Always add a blank line BEFORE section headers (##)
- Always add a blank line AFTER section headers
- Separate list items with single newlines
- Keep proper spacing between tables and text
- Example: After a table, add blank line, then ## Header, then blank line, then content

IMPORTANT:
- Output ONLY valid Markdown (NO LaTeX, NO math mode)
- STRICTLY maintain 3-column table format
- Test your tables: count pipes (|) - each row needs exactly 4 pipes
- Include ALL product specifications found
- EXCLUDE contact info, disclaimers, and legal text
- Use plain numbers without $ or ^ formatting
- Be precise and complete
- SKIP empty sections entirely`
}

// parseStream parses the Server-Sent Events stream
func (c *Client) parseStream(body io.Reader, resultCh chan<- string) error {
	parser := NewStreamParser(body)
	err := parser.ParseAll(resultCh)
	if err != nil {
		return domain.APIError("Failed to parse stream", err)
	}
	return nil
}

// CategorizationResponse represents the LLM's categorization response
type CategorizationResponse struct {
	Domain           string  `json:"domain"`
	DomainConfidence float64 `json:"domain_confidence"`

	Subdomain           string  `json:"subdomain"`
	SubdomainConfidence float64 `json:"subdomain_confidence"`

	CountryCode           string  `json:"country_code"`
	CountryCodeConfidence float64 `json:"country_code_confidence"`

	ModelYear           int     `json:"model_year"`
	ModelYearConfidence float64 `json:"model_year_confidence"`

	Condition           string  `json:"condition"`
	ConditionConfidence float64 `json:"condition_confidence"`

	Make           string  `json:"make"`
	MakeConfidence float64 `json:"make_confidence"`

	Model           string  `json:"model"`
	ModelConfidence float64 `json:"model_confidence"`
}

// ConfidenceThreshold is the minimum confidence (70%) for categorization fields (FR-016)
const ConfidenceThreshold = 0.70

// DetectCategorization analyzes page images and extracts document metadata (FR-016)
// It implements sequential page fallback: cover page → first page → subsequent pages
func (c *Client) DetectCategorization(ctx context.Context, pageImages []domain.PageImage) (*domain.DocumentMetadata, error) {
	if len(pageImages) == 0 {
		return domain.NewDocumentMetadata(), nil
	}

	// Sequential page fallback: try each page until we get clear categorization
	for _, pageImage := range pageImages {
		metadata, err := c.detectCategorizationFromPage(ctx, pageImage.ImagePath)
		if err != nil {
			// Log error and continue to next page
			continue
		}

		// Check if we got meaningful categorization (at least one field is valid)
		if metadata.IsValid() && metadata.Confidence >= ConfidenceThreshold {
			return metadata, nil
		}
	}

	// All pages failed or returned low confidence - return default metadata
	return domain.NewDocumentMetadata(), nil
}

// detectCategorizationFromPage analyzes a single page for categorization
func (c *Client) detectCategorizationFromPage(ctx context.Context, imagePath string) (*domain.DocumentMetadata, error) {
	// Build categorization request
	req, err := c.buildCategorizationRequest(imagePath)
	if err != nil {
		return nil, domain.APIError("Failed to build categorization request", err)
	}

	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return nil, domain.APIError("Failed to marshal request", err)
	}

	// Send request with retry logic
	resp, err := c.retryWithBackoff(ctx, func() (*http.Response, error) {
		reqBody := bytes.NewReader(body)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, reqBody)
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("HTTP-Referer", "https://github.com/spherical/pdf-extractor")
		httpReq.Header.Set("X-Title", "PDF Specification Extractor")

		return c.httpClient.Do(httpReq)
	})

	if err != nil {
		return nil, domain.APIError("Failed to send categorization request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, domain.APIError(fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(bodyBytes)), nil)
	}

	// Parse non-streaming response
	return c.parseCategorizationResponse(resp.Body)
}

// buildCategorizationRequest constructs the API request for categorization
func (c *Client) buildCategorizationRequest(imagePath string) (*Request, error) {
	// Read and encode image
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)
	imageURL := "data:image/jpeg;base64," + base64Image

	// Build message
	msg := Message{
		Role: "user",
		Content: []ContentPart{
			{
				Type: "text",
				Text: buildCategorizationPrompt(),
			},
			{
				Type: "image_url",
				ImageURL: &ImageURL{
					URL: imageURL,
				},
			},
		},
	}

	return &Request{
		Model:    c.model,
		Messages: []Message{msg},
		Stream:   false, // Non-streaming for categorization
	}, nil
}

// buildCategorizationPrompt creates the categorization detection prompt (T605)
func buildCategorizationPrompt() string {
	return `You are a document categorization expert. Analyze this image from a document (likely a cover page or title page).

Extract document metadata and return ONLY a valid JSON object with the following structure:

{
  "domain": "Automobile|Real Estate|Luxury Watch|Jewelry|Electronics|Fashion|Furniture|Art|Collectibles|Other",
  "domain_confidence": 0.0-1.0,
  "subdomain": "string (e.g., Commercial, Consumer, Residential, Sports, Sedan, SUV)",
  "subdomain_confidence": 0.0-1.0,
  "country_code": "ISO 3166-1 alpha-2 code (e.g., US, UK, IN, DE, JP)",
  "country_code_confidence": 0.0-1.0,
  "model_year": integer (e.g., 2025, 2024) or 0 if not found,
  "model_year_confidence": 0.0-1.0,
  "condition": "New|Used|Secondary Resale|Certified Pre-Owned|Refurbished",
  "condition_confidence": 0.0-1.0,
  "make": "string (manufacturer/brand name, e.g., Toyota, Rolex, Apple)",
  "make_confidence": 0.0-1.0,
  "model": "string (specific model name, e.g., Camry, Submariner, iPhone)",
  "model_confidence": 0.0-1.0
}

DETECTION RULES:
1. Domain Detection:
   - Look for product type indicators: vehicle images → Automobile, property photos → Real Estate, watch → Luxury Watch
   - Use visual cues and text context to determine domain

2. Subdomain/Type Detection (IMPORTANT - this indicates the product type):
   - For Automobile: Sedan, SUV, Hatchback, Truck, MPV, Crossover, Coupe, Convertible, Wagon, Van, Commercial Vehicle, Sports Car, Luxury, Electric Vehicle (EV), Hybrid
   - For Real Estate: Residential, Commercial, Industrial, Land, Apartment, Villa, Office Space
   - For Electronics: Smartphone, Laptop, Tablet, Wearable, Home Appliance
   - Use context clues from the document to determine the specific product type/category

3. Country Code Detection:
   - Look for country names, language, currency symbols, phone formats, regional pricing
   - Check for "Available in [Country]", distributor information, regulatory marks
   - Default to "Unknown" if not determinable
   - Use ISO 3166-1 alpha-2 codes (US, UK, IN, DE, FR, JP, etc.)

4. Model Year Detection (CRITICAL - try hard to find this):
   - Look for explicit year mentions: "2025 Model", "MY2024", "2025 Edition", "All-New 2025"
   - Check titles/headers for years like "The 2025 [Model Name]"
   - Look for copyright years (© 2025) - this often indicates current model year
   - Check for "New for 2025", "Introducing the 2025", "Launch Year"
   - Look at publication dates - brochures are usually for current/next model year
   - For automobiles: Check for model year in specs tables
   - Common patterns: "FY2025", "MY25", "2025 MY", "Model Year 2025"
   - If document appears to be for a current/upcoming product and shows recent copyright, use that year
   - Return 0 ONLY if absolutely no year indication is found

5. Condition Detection:
   - Default to "New" for marketing brochures/spec sheets/official brand documents
   - Look for "Used", "Pre-owned", "Certified", "Second-hand" keywords
   - "Secondary Resale" for auction/resale documents
   - "Certified Pre-Owned" or "CPO" for manufacturer-certified used vehicles

6. Make & Model Detection:
   - Make: Brand/manufacturer name (Toyota, BMW, Rolex, Maruti Suzuki, etc.)
   - Model: Specific product name (Camry, X5, Submariner, Wagon R, etc.)
   - Look in titles, headers, logos, prominent text
   - For automobiles: The largest text on cover is usually Make + Model

CONFIDENCE SCORING:
- 0.9-1.0: Explicitly stated in document
- 0.7-0.89: Strongly implied or clearly visible
- 0.5-0.69: Inferred from context
- 0.0-0.49: Uncertain/guessed

OUTPUT RULES:
- Return ONLY valid JSON, no markdown formatting
- No explanations or additional text
- Use "Unknown" for string fields if not found
- Use 0 for model_year if not found
- Always include confidence scores for each field`
}

// parseCategorizationResponse parses the LLM response for categorization
func (c *Client) parseCategorizationResponse(body io.Reader) (*domain.DocumentMetadata, error) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, domain.APIError("Failed to read response body", err)
	}

	// Parse the API response wrapper
	var apiResp Response
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, domain.APIError("Failed to parse API response", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, domain.APIError("No choices in API response", nil)
	}

	// Get the content from the response
	content := apiResp.Choices[0].Message.Content
	if content == "" {
		content = apiResp.Choices[0].Delta.Content
	}

	// Parse the JSON content from the LLM response
	catResp, err := parseCategorizationJSON(content)
	if err != nil {
		// Fallback: try to extract metadata heuristically
		return extractCategorizationHeuristically(content), nil
	}

	// Convert to DocumentMetadata with confidence threshold logic (T607)
	return applyConfidenceThreshold(catResp), nil
}

// parseCategorizationJSON extracts and parses JSON from LLM response
func parseCategorizationJSON(content string) (*CategorizationResponse, error) {
	// Try to find JSON in the response (LLM might include extra text)
	content = strings.TrimSpace(content)

	// Remove markdown code blocks if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Find JSON object boundaries
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")

	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	jsonContent := content[start : end+1]

	var catResp CategorizationResponse
	if err := json.Unmarshal([]byte(jsonContent), &catResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Debug: log what we parsed for model year
	if catResp.ModelYear > 0 || catResp.ModelYearConfidence > 0 {
		// Model year was found by LLM
		domain.DefaultLogger.WithPrefix("llm").Info("LLM returned ModelYear=%d (confidence=%.2f)", catResp.ModelYear, catResp.ModelYearConfidence)
	}

	return &catResp, nil
}

// applyConfidenceThreshold applies the >70% confidence threshold to categorization fields (T607)
func applyConfidenceThreshold(catResp *CategorizationResponse) *domain.DocumentMetadata {
	metadata := domain.NewDocumentMetadata()

	// Apply threshold to each field
	if catResp.DomainConfidence >= ConfidenceThreshold {
		metadata.Domain = domain.NormalizeDomain(catResp.Domain)
	}

	if catResp.SubdomainConfidence >= ConfidenceThreshold && catResp.Subdomain != "" {
		metadata.Subdomain = catResp.Subdomain
	}

	if catResp.CountryCodeConfidence >= ConfidenceThreshold {
		metadata.CountryCode = domain.NormalizeCountryCode(catResp.CountryCode)
	}

	// Model Year - use explicit year if found with sufficient confidence
	if catResp.ModelYear > 0 && catResp.ModelYearConfidence >= ConfidenceThreshold && domain.ValidateModelYear(catResp.ModelYear) {
		metadata.ModelYear = catResp.ModelYear
	}

	if catResp.ConditionConfidence >= ConfidenceThreshold && catResp.Condition != "" {
		metadata.Condition = domain.NormalizeCondition(catResp.Condition)
	}

	if catResp.MakeConfidence >= ConfidenceThreshold && catResp.Make != "" {
		metadata.Make = catResp.Make
	}

	if catResp.ModelConfidence >= ConfidenceThreshold && catResp.Model != "" {
		metadata.Model = catResp.Model
	}

	// Infer Model Year from current date if not detected and document is for a "New" product
	// This is common for non-US market brochures that don't explicitly state the model year
	if metadata.ModelYear == 0 && (metadata.Condition == "New" || catResp.Condition == "New") {
		currentYear := time.Now().Year()
		// Use current year for new products (brochures are typically for current/upcoming model year)
		metadata.ModelYear = currentYear
		domain.DefaultLogger.WithPrefix("llm").Info("Inferred ModelYear=%d from current date (new product brochure)", currentYear)
	}

	// Calculate overall confidence as average of non-zero confidences
	confidences := []float64{
		catResp.DomainConfidence,
		catResp.SubdomainConfidence,
		catResp.CountryCodeConfidence,
		catResp.ModelYearConfidence,
		catResp.ConditionConfidence,
		catResp.MakeConfidence,
		catResp.ModelConfidence,
	}
	var sum float64
	var count int
	for _, conf := range confidences {
		if conf > 0 {
			sum += conf
			count++
		}
	}
	if count > 0 {
		metadata.Confidence = sum / float64(count)
	}

	return metadata
}

// extractCategorizationHeuristically attempts to extract metadata from non-JSON response
// This is a fallback when LLM doesn't provide proper JSON (T607 heuristic fallback)
func extractCategorizationHeuristically(content string) *domain.DocumentMetadata {
	metadata := domain.NewDocumentMetadata()
	content = strings.ToLower(content)

	// Heuristic domain detection
	domainPatterns := map[string][]string{
		"Automobile":   {"car", "vehicle", "sedan", "suv", "truck", "automotive", "motor"},
		"Real Estate":  {"property", "house", "apartment", "real estate", "residential", "commercial"},
		"Luxury Watch": {"watch", "timepiece", "chronograph", "rolex", "omega"},
		"Jewelry":      {"jewelry", "jewellery", "diamond", "gold", "necklace", "ring"},
		"Electronics":  {"electronic", "phone", "computer", "laptop", "tablet", "device"},
	}

	for domainName, patterns := range domainPatterns {
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				metadata.Domain = domainName
				metadata.Confidence = 0.5 // Heuristic confidence is lower
				break
			}
		}
		if metadata.Domain != "Unknown" {
			break
		}
	}

	// Heuristic year detection
	yearRegex := regexp.MustCompile(`\b(20[0-9]{2}|19[0-9]{2})\b`)
	if matches := yearRegex.FindStringSubmatch(content); len(matches) > 0 {
		if year, err := strconv.Atoi(matches[1]); err == nil && domain.ValidateModelYear(year) {
			metadata.ModelYear = year
		}
	}

	// Heuristic condition detection
	if strings.Contains(content, "used") || strings.Contains(content, "pre-owned") {
		metadata.Condition = "Used"
	} else if strings.Contains(content, "new") || strings.Contains(content, "brochure") {
		metadata.Condition = "New"
	}

	return metadata
}

// DetectCategorizationWithMajorityVote analyzes multiple pages and uses majority vote for conflicts (T611)
func (c *Client) DetectCategorizationWithMajorityVote(ctx context.Context, pageImages []domain.PageImage) (*domain.DocumentMetadata, error) {
	if len(pageImages) == 0 {
		return domain.NewDocumentMetadata(), nil
	}

	// If only one page, use direct detection
	if len(pageImages) == 1 {
		return c.DetectCategorization(ctx, pageImages)
	}

	// Collect metadata from multiple pages
	var allMetadata []*domain.DocumentMetadata
	for _, pageImage := range pageImages {
		metadata, err := c.detectCategorizationFromPage(ctx, pageImage.ImagePath)
		if err != nil {
			continue
		}
		if metadata.IsValid() {
			allMetadata = append(allMetadata, metadata)
		}
	}

	if len(allMetadata) == 0 {
		return domain.NewDocumentMetadata(), nil
	}

	if len(allMetadata) == 1 {
		return allMetadata[0], nil
	}

	// Apply majority vote
	return majorityVote(allMetadata), nil
}

// majorityVote selects the most common value for each field from multiple metadata results
func majorityVote(metadataList []*domain.DocumentMetadata) *domain.DocumentMetadata {
	result := domain.NewDocumentMetadata()

	// Count occurrences for each field
	domainCounts := make(map[string]int)
	subdomainCounts := make(map[string]int)
	countryCodeCounts := make(map[string]int)
	modelYearCounts := make(map[int]int)
	conditionCounts := make(map[string]int)
	makeCounts := make(map[string]int)
	modelCounts := make(map[string]int)
	var totalConfidence float64

	for _, m := range metadataList {
		if m.Domain != "Unknown" {
			domainCounts[m.Domain]++
		}
		if m.Subdomain != "Unknown" {
			subdomainCounts[m.Subdomain]++
		}
		if m.CountryCode != "Unknown" {
			countryCodeCounts[m.CountryCode]++
		}
		if m.ModelYear != 0 {
			modelYearCounts[m.ModelYear]++
		}
		if m.Condition != "Unknown" {
			conditionCounts[m.Condition]++
		}
		if m.Make != "Unknown" {
			makeCounts[m.Make]++
		}
		if m.Model != "Unknown" {
			modelCounts[m.Model]++
		}
		totalConfidence += m.Confidence
	}

	// Select majority for each field
	result.Domain = selectMajority(domainCounts, "Unknown")
	result.Subdomain = selectMajority(subdomainCounts, "Unknown")
	result.CountryCode = selectMajority(countryCodeCounts, "Unknown")
	result.ModelYear = selectMajorityInt(modelYearCounts, 0)
	result.Condition = selectMajority(conditionCounts, "Unknown")
	result.Make = selectMajority(makeCounts, "Unknown")
	result.Model = selectMajority(modelCounts, "Unknown")
	result.Confidence = totalConfidence / float64(len(metadataList))

	return result
}

// selectMajority returns the string value with the highest count
func selectMajority(counts map[string]int, defaultValue string) string {
	if len(counts) == 0 {
		return defaultValue
	}

	var maxCount int
	var maxValue string
	for value, count := range counts {
		if count > maxCount {
			maxCount = count
			maxValue = value
		}
	}
	return maxValue
}

// selectMajorityInt returns the int value with the highest count
func selectMajorityInt(counts map[int]int, defaultValue int) int {
	if len(counts) == 0 {
		return defaultValue
	}

	var maxCount int
	var maxValue int
	for value, count := range counts {
		if count > maxCount {
			maxCount = count
			maxValue = value
		}
	}
	return maxValue
}
