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
