// Package ingest provides the brochure ingestion pipeline for the Knowledge Engine.
package ingest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// ParsedBrochure represents the extracted content from a brochure Markdown file.
type ParsedBrochure struct {
	Metadata     BrochureMetadata
	SpecValues   []ParsedSpec
	Features     []ParsedFeature
	USPs         []ParsedUSP
	RawChunks    []ParsedChunk
	SourcePages  map[int]string // page number -> content
	Errors       []ParseError
}

// BrochureMetadata holds YAML frontmatter metadata.
type BrochureMetadata struct {
	Title         string
	ProductName   string
	ModelYear     int
	Locale        string
	Market        string
	Trim          string
	ExtractedFrom string
	ExtractedAt   string
	Version       string
}

// ParsedSpec represents an extracted specification.
type ParsedSpec struct {
	Category          string
	Name              string
	Value             string
	Unit              string
	KeyFeatures       string // 4th column: Key Features
	VariantAvailability string // 5th column: Variant Availability
	Numeric           *float64
	Confidence        float64
	SourcePage        int
	SourceLine        int
	RawText           string
}

// ParsedFeature represents an extracted feature bullet.
type ParsedFeature struct {
	Body       string
	Tags       []string
	Priority   int
	SourcePage int
	SourceLine int
}

// ParsedUSP represents an extracted unique selling proposition.
type ParsedUSP struct {
	Body       string
	Tags       []string
	Priority   int
	SourcePage int
	SourceLine int
}

// ParsedChunk represents a text chunk for semantic indexing.
type ParsedChunk struct {
	Text       string
	ChunkType  storage.ChunkType
	SourcePage int
	StartLine  int
	EndLine    int
	Metadata   map[string]interface{}
}

// ParseError represents a parsing error or warning.
type ParseError struct {
	Line    int
	Column  int
	Message string
	Severity string // "error" or "warning"
}

// Parser handles Markdown parsing and content extraction.
type Parser struct {
	categoryAliases map[string]string
	unitNormalizer  *UnitNormalizer
	chunkSize       int
	chunkOverlap    int
}

// ParserConfig holds parser configuration.
type ParserConfig struct {
	ChunkSize    int
	ChunkOverlap int
}

// NewParser creates a new Markdown parser.
func NewParser(cfg ParserConfig) *Parser {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 512
	}
	if cfg.ChunkOverlap <= 0 {
		cfg.ChunkOverlap = 64
	}

	return &Parser{
		categoryAliases: defaultCategoryAliases(),
		unitNormalizer:  NewUnitNormalizer(),
		chunkSize:       cfg.ChunkSize,
		chunkOverlap:    cfg.ChunkOverlap,
	}
}

// Parse extracts structured content from Markdown.
func (p *Parser) Parse(content string) (*ParsedBrochure, error) {
	result := &ParsedBrochure{
		SourcePages: make(map[int]string),
	}

	// Parse YAML frontmatter
	metadata, remaining, err := p.parseMetadata(content)
	if err != nil {
		result.Errors = append(result.Errors, ParseError{
			Line:     1,
			Message:  fmt.Sprintf("metadata parse error: %v", err),
			Severity: "warning",
		})
	} else {
		result.Metadata = metadata
	}

	// Split by pages (if page markers exist)
	pages := p.splitByPages(remaining)
	for pageNum, pageContent := range pages {
		result.SourcePages[pageNum] = pageContent
	}

	// Parse tables for specs
	specs := p.parseSpecTables(remaining)
	result.SpecValues = specs

	// Parse feature lists
	features := p.parseFeatures(remaining)
	result.Features = features

	// Parse USPs
	usps := p.parseUSPs(remaining)
	result.USPs = usps

	// Generate chunks for semantic search
	chunks := p.generateChunks(remaining)
	result.RawChunks = chunks

	return result, nil
}

// parseMetadata extracts YAML frontmatter.
func (p *Parser) parseMetadata(content string) (BrochureMetadata, string, error) {
	meta := BrochureMetadata{}

	// Check for YAML frontmatter
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return meta, content, nil
	}

	// Find end of frontmatter
	lines := strings.Split(content, "\n")
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return meta, content, fmt.Errorf("unclosed YAML frontmatter")
	}

	// Parse simple key-value pairs
	for i := 1; i < endIdx; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

		switch strings.ToLower(key) {
		case "title":
			meta.Title = value
		case "product", "product_name", "productname":
			meta.ProductName = value
		case "year", "model_year", "modelyear":
			if y, err := strconv.Atoi(value); err == nil {
				meta.ModelYear = y
			}
		case "locale":
			meta.Locale = value
		case "market":
			meta.Market = value
		case "trim":
			meta.Trim = value
		case "extracted_from", "source":
			meta.ExtractedFrom = value
		case "extracted_at":
			meta.ExtractedAt = value
		case "version":
			meta.Version = value
		}
	}

	// Return content after frontmatter
	remaining := strings.Join(lines[endIdx+1:], "\n")
	return meta, remaining, nil
}

// splitByPages splits content by page markers.
func (p *Parser) splitByPages(content string) map[int]string {
	pages := make(map[int]string)
	
	// Look for page markers like "<!-- PAGE 1 -->" or "## Page 1"
	pageMarkerRe := regexp.MustCompile(`(?i)(?:<!--\s*PAGE\s*(\d+)\s*-->|##\s*Page\s*(\d+))`)
	
	matches := pageMarkerRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		pages[1] = content
		return pages
	}

	for i, match := range matches {
		// Determine page number
		var pageNum int
		if match[2] != -1 {
			pageNum, _ = strconv.Atoi(content[match[2]:match[3]])
		} else if match[4] != -1 {
			pageNum, _ = strconv.Atoi(content[match[4]:match[5]])
		} else {
			pageNum = i + 1
		}

		// Extract content until next marker or end
		start := match[1]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(content)
		}

		pages[pageNum] = strings.TrimSpace(content[start:end])
	}

	return pages
}

// parseSpecTables extracts specifications from Markdown tables.
func (p *Parser) parseSpecTables(content string) []ParsedSpec {
	var specs []ParsedSpec

	// Match 5-column tables: | Category | Specification | Value | Key Features | Variant Availability |
	tableRow5Re := regexp.MustCompile(`\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|`)
	// Match 4-column tables: | Category | Specification | Value | Unit |
	tableRow4Re := regexp.MustCompile(`\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|`)
	// Match 3-column tables: | Category | Specification | Value |
	tableRow3Re := regexp.MustCompile(`\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|`)

	lines := strings.Split(content, "\n")
	currentCategory := ""
	lineNum := 0

	for _, line := range lines {
		lineNum++
		line = strings.TrimSpace(line)

		// Skip header/separator rows
		if strings.Contains(line, "---") || strings.Contains(line, "===") {
			continue
		}

		var category, name, value, unit, keyFeatures, variantAvailability string

		// Try 5-column format first (new format with Variant Availability)
		matches5 := tableRow5Re.FindStringSubmatch(line)
		if len(matches5) >= 6 {
			category = strings.TrimSpace(matches5[1])
			name = strings.TrimSpace(matches5[2])
			value = strings.TrimSpace(matches5[3])
			keyFeatures = strings.TrimSpace(matches5[4])
			variantAvailability = strings.TrimSpace(matches5[5])
			unit = "" // Unit extracted from value if numeric
		} else {
			// Try 4-column format (legacy: Category | Specification | Value | Unit)
			matches4 := tableRow4Re.FindStringSubmatch(line)
			if len(matches4) >= 5 {
				category = strings.TrimSpace(matches4[1])
				name = strings.TrimSpace(matches4[2])
				value = strings.TrimSpace(matches4[3])
				unit = strings.TrimSpace(matches4[4])
				keyFeatures = ""
				variantAvailability = ""
			} else {
				// Try 3-column format (legacy: Category | Specification | Value)
				matches3 := tableRow3Re.FindStringSubmatch(line)
				if len(matches3) < 4 {
					continue
				}
				category = strings.TrimSpace(matches3[1])
				name = strings.TrimSpace(matches3[2])
				value = strings.TrimSpace(matches3[3])
				unit = "" // No unit column in 3-column format
				keyFeatures = ""
				variantAvailability = ""
			}
		}

		// Skip header rows (check for 5-column, 4-column, or 3-column headers)
		if strings.EqualFold(category, "category") || 
		   strings.EqualFold(name, "specification") ||
		   strings.EqualFold(name, "spec") ||
		   strings.EqualFold(value, "value") ||
		   strings.EqualFold(keyFeatures, "key features") ||
		   strings.EqualFold(variantAvailability, "variant availability") {
			continue
		}

		// Handle merged cells (category might be empty for continuation)
		if category != "" {
			currentCategory = p.normalizeCategory(category)
		}

		if name == "" || value == "" {
			continue
		}

		// Extract unit from value if embedded (e.g., "25.49 km/l")
		if unit == "" {
			value, unit = p.extractUnitFromValue(value)
		}

		spec := ParsedSpec{
			Category:           currentCategory,
			Name:               name,
			Value:              value,
			Unit:               p.unitNormalizer.Normalize(unit),
			KeyFeatures:        keyFeatures,
			VariantAvailability: variantAvailability,
			Confidence:         1.0,
			SourceLine:         lineNum,
			RawText:            line,
		}

		// Try to parse numeric value
		if num, err := p.parseNumericValue(value); err == nil {
			spec.Numeric = &num
		}

		specs = append(specs, spec)
	}

	return specs
}

// extractUnitFromValue extracts unit from value if embedded.
func (p *Parser) extractUnitFromValue(value string) (string, string) {
	// Common patterns: "25.49 km/l", "176 hp", "221 Nm", "4885 mm"
	// Note: Only extract units when preceded by a number or space+number
	unitPatterns := []string{
		"km/l", "kmpl", "mpg", "hp", "bhp", "kW", "Nm", "kg-m",
		"mm", "cm", "kg", "cc", "rpm", "PS",
		"stars", "count", "passengers", "inches",
	}
	
	// Single-char units that need more careful matching (only after numbers)
	singleCharUnits := []string{"L", "m"}

	value = strings.TrimSpace(value)
	
	// Check for space-separated unit first (safer matching)
	if idx := strings.LastIndex(value, " "); idx > 0 {
		potentialUnit := strings.TrimSpace(value[idx+1:])
		numPart := strings.TrimSpace(value[:idx])
		
		// Check multi-char units
		for _, unit := range unitPatterns {
			if strings.EqualFold(potentialUnit, unit) {
				return numPart, potentialUnit
			}
		}
		
		// Check single-char units only if preceded by a number
		if len(numPart) > 0 && isNumericString(numPart) {
			for _, unit := range singleCharUnits {
				if strings.EqualFold(potentialUnit, unit) {
					return numPart, potentialUnit
				}
			}
		}
	}
	
	// Check for directly attached units (e.g., "25.49km/l")
	for _, unit := range unitPatterns {
		if strings.HasSuffix(strings.ToLower(value), strings.ToLower(unit)) {
			numPart := strings.TrimSpace(value[:len(value)-len(unit)])
			// Only extract if what remains looks like a number
			if isNumericString(numPart) {
				return numPart, unit
			}
		}
	}

	return value, ""
}

// isNumericString checks if a string looks like a number (int or float)
func isNumericString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Allow numbers with commas (e.g., "1,234") and decimals
	for i, r := range s {
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '.' || r == ',' {
			continue
		}
		if r == '-' && i == 0 {
			continue
		}
		return false
	}
	return true
}

// parseFeatures extracts feature bullets.
func (p *Parser) parseFeatures(content string) []ParsedFeature {
	var features []ParsedFeature

	// Look for feature sections
	featureSectionRe := regexp.MustCompile(`(?i)##\s*(?:Features?|Key Features?|Highlights?)\s*\n((?:[-*]\s*.+\n?)+)`)
	
	matches := featureSectionRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		bullets := p.parseBulletList(match[1])
		for i, bullet := range bullets {
			features = append(features, ParsedFeature{
				Body:     bullet,
				Tags:     p.inferTags(bullet),
				Priority: i + 1,
			})
		}
	}

	return features
}

// parseUSPs extracts unique selling propositions.
func (p *Parser) parseUSPs(content string) []ParsedUSP {
	var usps []ParsedUSP

	// Look for USP sections
	uspSectionRe := regexp.MustCompile(`(?i)##\s*(?:USPs?|Unique Selling (?:Points?|Propositions?)|Why (?:Buy|Choose))\s*\n((?:[-*]\s*.+\n?)+)`)
	
	matches := uspSectionRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		bullets := p.parseBulletList(match[1])
		for i, bullet := range bullets {
			usps = append(usps, ParsedUSP{
				Body:     bullet,
				Tags:     p.inferTags(bullet),
				Priority: i + 1,
			})
		}
	}

	return usps
}

// generateChunks creates text chunks for semantic indexing.
func (p *Parser) generateChunks(content string) []ParsedChunk {
	var chunks []ParsedChunk

	// Remove markdown formatting for chunking
	cleanContent := p.cleanMarkdown(content)
	
	// Split into sentences/paragraphs
	paragraphs := strings.Split(cleanContent, "\n\n")
	
	var currentChunk strings.Builder
	var currentLines []int
	startLine := 1

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Check if adding this paragraph exceeds chunk size
		if currentChunk.Len()+len(para) > p.chunkSize && currentChunk.Len() > 0 {
			// Save current chunk
			chunks = append(chunks, ParsedChunk{
				Text:      currentChunk.String(),
				ChunkType: storage.ChunkTypeGlobal,
				StartLine: startLine,
				EndLine:   startLine + len(currentLines),
				Metadata:  make(map[string]interface{}),
			})

			// Start new chunk with overlap
			overlapText := p.getOverlapText(currentChunk.String(), p.chunkOverlap)
			currentChunk.Reset()
			currentChunk.WriteString(overlapText)
			currentLines = nil
			startLine += len(currentLines)
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
		currentLines = append(currentLines, 1)
	}

	// Add final chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, ParsedChunk{
			Text:      currentChunk.String(),
			ChunkType: storage.ChunkTypeGlobal,
			StartLine: startLine,
			EndLine:   startLine + len(currentLines),
			Metadata:  make(map[string]interface{}),
		})
	}

	return chunks
}

// Helper methods

func (p *Parser) normalizeCategory(category string) string {
	normalized := strings.ToLower(strings.TrimSpace(category))
	if alias, ok := p.categoryAliases[normalized]; ok {
		return alias
	}
	return category
}

func (p *Parser) parseNumericValue(value string) (float64, error) {
	// Remove common formatting
	cleaned := strings.ReplaceAll(value, ",", "")
	cleaned = strings.TrimSpace(cleaned)
	
	// Try to parse as float
	return strconv.ParseFloat(cleaned, 64)
}

func (p *Parser) parseBulletList(content string) []string {
	var bullets []string
	lines := strings.Split(content, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
			bullet := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "-"), "*"))
			if bullet != "" {
				bullets = append(bullets, bullet)
			}
		}
	}
	
	return bullets
}

func (p *Parser) inferTags(text string) []string {
	var tags []string
	text = strings.ToLower(text)

	tagKeywords := map[string][]string{
		"safety":     {"airbag", "brake", "abs", "safety", "collision", "crash"},
		"comfort":    {"seat", "climate", "ac", "air conditioning", "leather", "comfort"},
		"technology": {"display", "screen", "bluetooth", "usb", "navigation", "gps", "sensor"},
		"performance": {"engine", "horsepower", "torque", "acceleration", "speed"},
		"efficiency": {"fuel", "mileage", "hybrid", "electric", "economy"},
		"exterior":   {"wheel", "headlight", "grille", "body", "paint"},
	}

	for tag, keywords := range tagKeywords {
		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				tags = append(tags, tag)
				break
			}
		}
	}

	return tags
}

func (p *Parser) cleanMarkdown(content string) string {
	// Remove headers
	content = regexp.MustCompile(`#+\s*`).ReplaceAllString(content, "")
	// Remove bold/italic
	content = regexp.MustCompile(`\*+([^*]+)\*+`).ReplaceAllString(content, "$1")
	// Remove links
	content = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(content, "$1")
	// Remove images
	content = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`).ReplaceAllString(content, "")
	// Remove HTML comments
	content = regexp.MustCompile(`<!--.*?-->`).ReplaceAllString(content, "")
	
	return content
}

func (p *Parser) getOverlapText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	
	// Try to break at word boundary
	overlap := text[len(text)-maxLen:]
	if idx := strings.Index(overlap, " "); idx > 0 {
		overlap = overlap[idx+1:]
	}
	
	return overlap
}

func defaultCategoryAliases() map[string]string {
	return map[string]string{
		"engine specs":     "Engine",
		"engine":           "Engine",
		"fuel economy":     "Fuel Efficiency",
		"fuel efficiency":  "Fuel Efficiency",
		"mileage":          "Fuel Efficiency",
		"transmission":     "Transmission",
		"gearbox":          "Transmission",
		"dimensions":       "Dimensions",
		"size":             "Dimensions",
		"weight":           "Weight",
		"mass":             "Weight",
		"safety":           "Safety",
		"security":         "Safety",
		"comfort":          "Comfort",
		"interior":         "Comfort",
		"technology":       "Technology",
		"tech":             "Technology",
		"infotainment":     "Technology",
		"exterior":         "Exterior",
		"design":           "Exterior",
		"warranty":         "Warranty",
	}
}

// UnitNormalizer normalizes measurement units.
type UnitNormalizer struct {
	aliases map[string]string
}

// NewUnitNormalizer creates a new unit normalizer.
func NewUnitNormalizer() *UnitNormalizer {
	return &UnitNormalizer{
		aliases: map[string]string{
			"kilometers per liter": "km/l",
			"km/litre":             "km/l",
			"kmpl":                 "km/l",
			"miles per gallon":     "mpg",
			"horsepower":           "hp",
			"horse power":          "hp",
			"bhp":                  "hp",
			"kilowatt":             "kW",
			"kilowatts":            "kW",
			"newton meter":         "Nm",
			"newton-meter":         "Nm",
			"newton metres":        "Nm",
			"millimeter":           "mm",
			"millimeters":          "mm",
			"millimetre":           "mm",
			"millimetres":          "mm",
			"centimeter":           "cm",
			"centimeters":          "cm",
			"centimetre":           "cm",
			"centimetres":          "cm",
			"meter":                "m",
			"meters":               "m",
			"metre":                "m",
			"metres":               "m",
			"kilogram":             "kg",
			"kilograms":            "kg",
			"liter":                "L",
			"liters":               "L",
			"litre":                "L",
			"litres":               "L",
			"cc":                   "cc",
			"cubic centimeters":    "cc",
		},
	}
}

// Normalize converts a unit to its canonical form.
func (n *UnitNormalizer) Normalize(unit string) string {
	normalized := strings.ToLower(strings.TrimSpace(unit))
	if canonical, ok := n.aliases[normalized]; ok {
		return canonical
	}
	return unit
}

// ValidateParsedBrochure checks the parsed content for issues.
func ValidateParsedBrochure(parsed *ParsedBrochure) []ParseError {
	var errors []ParseError

	// Check for minimum content
	if len(parsed.SpecValues) == 0 {
		errors = append(errors, ParseError{
			Message:  "no specifications found",
			Severity: "warning",
		})
	}

	// Check for duplicate specs
	specKeys := make(map[string]bool)
	for _, spec := range parsed.SpecValues {
		key := fmt.Sprintf("%s:%s", spec.Category, spec.Name)
		if specKeys[key] {
			errors = append(errors, ParseError{
				Line:     spec.SourceLine,
				Message:  fmt.Sprintf("duplicate specification: %s", key),
				Severity: "warning",
			})
		}
		specKeys[key] = true
	}

	// Validate metadata
	if parsed.Metadata.ProductName == "" {
		errors = append(errors, ParseError{
			Message:  "missing product name in metadata",
			Severity: "warning",
		})
	}

	return errors
}

// GenerateSpecID creates a deterministic ID for a spec value.
func GenerateSpecID(tenantID, productID uuid.UUID, category, name string) uuid.UUID {
	// Use UUID v5 for deterministic generation
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // DNS namespace
	data := fmt.Sprintf("%s:%s:%s:%s", tenantID, productID, category, name)
	return uuid.NewSHA1(namespace, []byte(data))
}

