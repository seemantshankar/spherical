// Package main provides an interactive CLI demo for the Knowledge Engine.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

type KnowledgeBase struct {
	db         *sql.DB
	tenantID   uuid.UUID
	productID  uuid.UUID
	campaignID uuid.UUID
	embedder   embedding.Embedder
}

type scoredSpec struct {
	Category   string
	Name       string
	Value      string
	Unit       string
	Confidence float64
	Score      int // Relevance score
}

func main() {
	printBanner()

	ctx := context.Background()
	_ = observability.NewLogger(observability.LogConfig{
		Level:       "info",
		Format:      "console",
		ServiceName: "knowledge-demo",
	})

	// Initialize database
	dbPath := filepath.Join(os.TempDir(), "knowledge_demo.db")
	fmt.Printf("%sInitializing database at: %s%s\n", colorCyan, dbPath, colorReset)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("%sError: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := runMigrations(db); err != nil {
		fmt.Printf("%sError running migrations: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}

	// Create embedder
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	var embedder embedding.Embedder
	if apiKey != "" {
		client, err := embedding.NewClient(embedding.Config{
			APIKey:  apiKey,
			Model:   "google/gemini-embedding-001",
			BaseURL: "https://openrouter.ai/api/v1",
		})
		if err == nil {
			embedder = client
			fmt.Printf("%s✓ Using OpenRouter embeddings%s\n", colorGreen, colorReset)
		}
	}
	if embedder == nil {
		embedder = embedding.NewMockClient(768)
		fmt.Printf("%s⚠ Using mock embeddings (set OPENROUTER_API_KEY for real embeddings)%s\n", colorYellow, colorReset)
	}

	kb := &KnowledgeBase{
		db:       db,
		embedder: embedder,
	}

	// Check if we need to ingest data
	if !kb.hasData() {
		fmt.Println("\n" + colorYellow + "No data found. Let's ingest the Toyota Camry brochure!" + colorReset)
		if err := kb.ingestBrochure(ctx); err != nil {
			fmt.Printf("%sError ingesting brochure: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
	} else {
		fmt.Printf("%s✓ Found existing data%s\n", colorGreen, colorReset)
		kb.loadExistingIDs()
	}

	// Print stats
	kb.printStats()

	// Interactive query loop
	fmt.Println("\n" + colorBold + "Interactive Query Mode" + colorReset)
	fmt.Println("Type your questions about the Toyota Camry. Type 'quit' to exit.\n")
	fmt.Println(colorCyan + "Example queries:" + colorReset)
	fmt.Println("  - What is the fuel efficiency?")
	fmt.Println("  - How many airbags does it have?")
	fmt.Println("  - Tell me about the safety features")
	fmt.Println("  - What are the dimensions?")
	fmt.Println("  - Does it have Apple CarPlay?")
	fmt.Println("")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(colorBold + "🚗 Query> " + colorReset)
		if !scanner.Scan() {
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}
		if strings.ToLower(query) == "quit" || strings.ToLower(query) == "exit" {
			fmt.Println("\n" + colorCyan + "Goodbye! 👋" + colorReset)
			break
		}

		// Special commands
		if strings.HasPrefix(query, "/") {
			kb.handleCommand(query)
			continue
		}

		// Run query
		kb.runQuery(ctx, query)
	}
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   🚗  Knowledge Engine Interactive Demo                       ║
║                                                               ║
║   Query the Toyota Camry Knowledge Base                       ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
`
	fmt.Println(colorCyan + banner + colorReset)
}

func (kb *KnowledgeBase) hasData() bool {
	var count int
	err := kb.db.QueryRow("SELECT COUNT(*) FROM spec_values").Scan(&count)
	return err == nil && count > 0
}

func (kb *KnowledgeBase) loadExistingIDs() {
	kb.db.QueryRow("SELECT id FROM tenants LIMIT 1").Scan(&kb.tenantID)
	kb.db.QueryRow("SELECT id FROM products LIMIT 1").Scan(&kb.productID)
	kb.db.QueryRow("SELECT id FROM campaign_variants LIMIT 1").Scan(&kb.campaignID)
}

func (kb *KnowledgeBase) ingestBrochure(ctx context.Context) error {
	// Find brochure
	brochurePaths := []string{
		"../../e-brochure-camry-hybrid-specs.md",
		"../../../e-brochure-camry-hybrid-specs.md",
		"../../../../e-brochure-camry-hybrid-specs.md",
		"e-brochure-camry-hybrid-specs.md",
	}

	var brochureContent []byte
	var err error
	var foundPath string

	for _, p := range brochurePaths {
		brochureContent, err = os.ReadFile(p)
		if err == nil {
			foundPath = p
			break
		}
	}

	if brochureContent == nil {
		return fmt.Errorf("brochure not found")
	}

	fmt.Printf("%sLoading brochure from: %s%s\n", colorCyan, foundPath, colorReset)

	// Parse
	fmt.Print("Parsing brochure... ")
	parser := ingest.NewParser(ingest.ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(string(brochureContent))
	if err != nil {
		return err
	}
	fmt.Printf("%s✓%s (%d specs, %d features, %d USPs)\n", colorGreen, colorReset,
		len(result.SpecValues), len(result.Features), len(result.USPs))

	// Create tenant, product, campaign
	kb.tenantID = uuid.New()
	kb.productID = uuid.New()
	kb.campaignID = uuid.New()

	kb.db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", kb.tenantID.String(), "Toyota India")
	kb.db.Exec("INSERT INTO products (id, tenant_id, name) VALUES (?, ?, ?)",
		kb.productID.String(), kb.tenantID.String(), "Camry Hybrid 2025")
	kb.db.Exec("INSERT INTO campaign_variants (id, product_id, tenant_id, locale, trim, status) VALUES (?, ?, ?, ?, ?, ?)",
		kb.campaignID.String(), kb.productID.String(), kb.tenantID.String(), "en-IN", "XLE Hybrid", "published")

	// Store specs
	fmt.Print("Storing specifications... ")
	categoryCache := make(map[string]uuid.UUID)
	for _, spec := range result.SpecValues {
		categoryID, ok := categoryCache[spec.Category]
		if !ok {
			categoryID = uuid.New()
			kb.db.Exec("INSERT OR IGNORE INTO spec_categories (id, name) VALUES (?, ?)",
				categoryID.String(), spec.Category)
			categoryCache[spec.Category] = categoryID
		}

		specItemID := uuid.New()
		kb.db.Exec("INSERT INTO spec_items (id, category_id, display_name, unit) VALUES (?, ?, ?, ?)",
			specItemID.String(), categoryID.String(), spec.Name, spec.Unit)

		specValueID := uuid.New()
		kb.db.Exec(`INSERT INTO spec_values (id, tenant_id, product_id, campaign_variant_id, spec_item_id, 
			value_text, unit, confidence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			specValueID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
			specItemID.String(), spec.Value, spec.Unit, spec.Confidence)
	}
	fmt.Printf("%s✓%s\n", colorGreen, colorReset)

	// Store features and USPs with embeddings
	totalChunks := len(result.Features) + len(result.USPs)
	fmt.Printf("Storing %d knowledge chunks with embeddings... ", totalChunks)

	chunkCount := 0
	for _, feature := range result.Features {
		chunkID := uuid.New()
		var embVector []byte
		if kb.embedder != nil {
			if emb, err := kb.embedder.EmbedSingle(ctx, feature.Body); err == nil {
				embVector, _ = json.Marshal(emb)
			}
		}
		metadata, _ := json.Marshal(map[string]interface{}{"tags": feature.Tags})
		kb.db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, metadata, embedding_vector, embedding_model) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
			"feature_block", feature.Body, string(metadata), embVector, kb.embedder.Model())
		chunkCount++
		if chunkCount%20 == 0 {
			fmt.Printf("\rStoring %d knowledge chunks with embeddings... %d/%d", totalChunks, chunkCount, totalChunks)
		}
	}

	for _, usp := range result.USPs {
		chunkID := uuid.New()
		var embVector []byte
		if kb.embedder != nil {
			if emb, err := kb.embedder.EmbedSingle(ctx, usp.Body); err == nil {
				embVector, _ = json.Marshal(emb)
			}
		}
		kb.db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, embedding_vector, embedding_model) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
			"usp", usp.Body, embVector, kb.embedder.Model())
		chunkCount++
	}

	fmt.Printf("\r%s✓%s Stored %d knowledge chunks                    \n", colorGreen, colorReset, chunkCount)
	return nil
}

func (kb *KnowledgeBase) printStats() {
	var specCount, chunkCount int
	kb.db.QueryRow("SELECT COUNT(*) FROM spec_values WHERE tenant_id = ?", kb.tenantID.String()).Scan(&specCount)
	kb.db.QueryRow("SELECT COUNT(*) FROM knowledge_chunks WHERE tenant_id = ?", kb.tenantID.String()).Scan(&chunkCount)

	fmt.Printf("\n%s📊 Knowledge Base Stats:%s\n", colorBold, colorReset)
	fmt.Printf("   Specifications: %d\n", specCount)
	fmt.Printf("   Knowledge Chunks: %d\n", chunkCount)
}

func (kb *KnowledgeBase) runQuery(ctx context.Context, query string) {
	start := time.Now()

	keywords := extractKeywords(query)

	// Also search for compound terms (e.g., "android auto", "apple carplay")
	compoundTerms := extractCompoundTerms(query)

	// Filter out keywords that are parts of compound terms to avoid false positives
	// e.g., if "android auto" is detected, don't search for just "auto"
	filteredKeywords := filterKeywordsByCompound(keywords, compoundTerms)

	// Search specs with relevance scoring
	specMap := make(map[string]*scoredSpec) // Use key to deduplicate

	// First, search for compound terms (higher priority)
	for _, term := range compoundTerms {
		rows, err := kb.db.Query(`
			SELECT DISTINCT sc.name, si.display_name, sv.value_text, COALESCE(sv.unit, ''), sv.confidence
			FROM spec_values sv
			JOIN spec_items si ON sv.spec_item_id = si.id
			JOIN spec_categories sc ON si.category_id = sc.id
			WHERE sv.tenant_id = ? AND sv.campaign_variant_id = ?
			  AND (LOWER(si.display_name) LIKE ? OR LOWER(sc.name) LIKE ? OR LOWER(sv.value_text) LIKE ?)
			ORDER BY sv.confidence DESC
			LIMIT 20
		`, kb.tenantID.String(), kb.campaignID.String(), "%"+term+"%", "%"+term+"%", "%"+term+"%")
		if err != nil {
			continue
		}
		for rows.Next() {
			var s struct {
				Category   string
				Name       string
				Value      string
				Unit       string
				Confidence float64
			}
			if rows.Scan(&s.Category, &s.Name, &s.Value, &s.Unit, &s.Confidence) == nil {
				key := s.Category + "|" + s.Name + "|" + s.Value
				if existing, ok := specMap[key]; ok {
					existing.Score += 5 // Compound term matches = much higher score
				} else {
					specMap[key] = &scoredSpec{
						Category:   s.Category,
						Name:       s.Name,
						Value:      s.Value,
						Unit:       s.Unit,
						Confidence: s.Confidence,
						Score:      5, // High score for compound term match
					}
				}
			}
		}
		rows.Close()
	}

	// Then search for individual keywords
	for _, keyword := range filteredKeywords {
		// Skip very common words that cause noise
		if keyword == "features" || keyword == "feature" {
			continue
		}
		rows, err := kb.db.Query(`
			SELECT DISTINCT sc.name, si.display_name, sv.value_text, COALESCE(sv.unit, ''), sv.confidence
			FROM spec_values sv
			JOIN spec_items si ON sv.spec_item_id = si.id
			JOIN spec_categories sc ON si.category_id = sc.id
			WHERE sv.tenant_id = ? AND sv.campaign_variant_id = ?
			  AND (LOWER(si.display_name) LIKE ? OR LOWER(sc.name) LIKE ? OR LOWER(sv.value_text) LIKE ?)
			ORDER BY sv.confidence DESC
			LIMIT 15
		`, kb.tenantID.String(), kb.campaignID.String(), "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		if err != nil {
			continue
		}
		for rows.Next() {
			var s struct {
				Category   string
				Name       string
				Value      string
				Unit       string
				Confidence float64
			}
			if rows.Scan(&s.Category, &s.Name, &s.Value, &s.Unit, &s.Confidence) == nil {
				key := s.Category + "|" + s.Name + "|" + s.Value
				if existing, ok := specMap[key]; ok {
					existing.Score++ // Multiple keyword matches = higher score
				} else {
					score := 1
					// Boost score if matches category (more relevant)
					if strings.Contains(strings.ToLower(s.Category), strings.ToLower(keyword)) {
						score += 2
					}
					// Boost score if matches name (very relevant)
					if strings.Contains(strings.ToLower(s.Name), strings.ToLower(keyword)) {
						score += 2
					}
					specMap[key] = &scoredSpec{
						Category:   s.Category,
						Name:       s.Name,
						Value:      s.Value,
						Unit:       s.Unit,
						Confidence: s.Confidence,
						Score:      score,
					}
				}
			}
		}
		rows.Close()
	}

	// Convert to slice and sort by score
	var specs []scoredSpec
	for _, s := range specMap {
		specs = append(specs, *s)
	}
	
	// Sort by score (descending), then by confidence
	for i := 0; i < len(specs)-1; i++ {
		for j := i + 1; j < len(specs); j++ {
			if specs[i].Score < specs[j].Score || 
			   (specs[i].Score == specs[j].Score && specs[i].Confidence < specs[j].Confidence) {
				specs[i], specs[j] = specs[j], specs[i]
			}
		}
	}
	
	// Limit to top 10 most relevant
	if len(specs) > 10 {
		specs = specs[:10]
	}

	// Search chunks - first try compound terms, then individual keywords
	var chunks []struct {
		Type string
		Text string
	}

	// Try compound terms first (higher priority)
	for _, term := range compoundTerms {
		rows, err := kb.db.Query(`
			SELECT DISTINCT chunk_type, text
			FROM knowledge_chunks
			WHERE tenant_id = ? AND campaign_variant_id = ?
			  AND LOWER(text) LIKE ?
			LIMIT 5
		`, kb.tenantID.String(), kb.campaignID.String(), "%"+term+"%")
		if err != nil {
			continue
		}
		for rows.Next() {
			var c struct {
				Type string
				Text string
			}
			if rows.Scan(&c.Type, &c.Text) == nil {
				chunks = append(chunks, c)
			}
		}
		rows.Close()
	}

	// Then try individual keywords
	for _, keyword := range keywords {
		rows, err := kb.db.Query(`
			SELECT DISTINCT chunk_type, text
			FROM knowledge_chunks
			WHERE tenant_id = ? AND campaign_variant_id = ?
			  AND LOWER(text) LIKE ?
			LIMIT 3
		`, kb.tenantID.String(), kb.campaignID.String(), "%"+keyword+"%")
		if err != nil {
			continue
		}
		for rows.Next() {
			var c struct {
				Type string
				Text string
			}
			if rows.Scan(&c.Type, &c.Text) == nil {
				chunks = append(chunks, c)
			}
		}
		rows.Close()
	}

	elapsed := time.Since(start)

	// Print results
	fmt.Printf("\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("%sResults for:%s %s\n", colorBold, colorReset, query)
	fmt.Printf("%s(found in %v)%s\n", colorPurple, elapsed, colorReset)
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)

	// Filter out irrelevant categories for specific queries
	filteredSpecs := filterRelevantSpecs(specs, query, filteredKeywords)
	
	if len(filteredSpecs) > 0 {
		fmt.Printf("\n%s📋 Specifications:%s\n", colorGreen, colorReset)
		for _, s := range filteredSpecs {
			unit := ""
			if s.Unit != "" {
				unit = " " + s.Unit
			}
			fmt.Printf("   • %s%s%s > %s: %s%s%s%s\n",
				colorYellow, s.Category, colorReset,
				s.Name, colorBold, s.Value, unit, colorReset)
		}
	}

	if len(chunks) > 0 {
		fmt.Printf("\n%s💡 Related Information:%s\n", colorBlue, colorReset)
		seen := make(map[string]bool)
		for _, c := range chunks {
			if seen[c.Text] {
				continue
			}
			seen[c.Text] = true
			typeIcon := "📝"
			if c.Type == "usp" {
				typeIcon = "⭐"
			}
			fmt.Printf("   %s %s\n", typeIcon, c.Text)
		}
	}

	if len(specs) == 0 && len(chunks) == 0 {
		fmt.Printf("\n%s❌ No results found for this query.%s\n", colorRed, colorReset)
		fmt.Println("   Try rephrasing your question or use more specific terms.")
	}

	fmt.Println()
}

func (kb *KnowledgeBase) handleCommand(cmd string) {
	switch strings.ToLower(cmd) {
	case "/stats":
		kb.printStats()
	case "/categories":
		rows, _ := kb.db.Query("SELECT DISTINCT name FROM spec_categories ORDER BY name")
		fmt.Printf("\n%s📂 Categories:%s\n", colorGreen, colorReset)
		for rows.Next() {
			var name string
			rows.Scan(&name)
			fmt.Printf("   • %s\n", name)
		}
		rows.Close()
		fmt.Println()
	case "/help":
		fmt.Println("\n" + colorCyan + "Available commands:" + colorReset)
		fmt.Println("   /stats      - Show database statistics")
		fmt.Println("   /categories - List all specification categories")
		fmt.Println("   /help       - Show this help message")
		fmt.Println("   quit        - Exit the demo")
		fmt.Println()
	default:
		fmt.Printf("%sUnknown command: %s%s\n", colorRed, cmd, colorReset)
		fmt.Println("Type /help for available commands")
	}
}

func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"what": true, "is": true, "the": true, "a": true, "an": true,
		"how": true, "many": true, "does": true, "it": true, "have": true,
		"tell": true, "me": true, "about": true, "of": true, "are": true,
		"why": true, "should": true, "i": true, "buy": true, "over": true,
		"can": true, "you": true, "please": true, "show": true, "this": true,
		"come": true, "in": true, "support": true, "car": true, "camry": true,
		"toyota": true, "vehicle": true, "available": true,
	}

	// British to American spelling normalization
	spellingMap := map[string]string{
		"colour":  "color",
		"colours": "colors",
		"favour":  "favor",
		"favours": "favors",
		"metre":   "meter",
		"metres":  "meters",
		"litre":   "liter",
		"litres":  "liters",
	}

	words := strings.Fields(strings.ToLower(query))
	keywordSet := make(map[string]bool)

	for _, word := range words {
		word = strings.Trim(word, "?.,!")
		if len(word) > 2 && !stopWords[word] {
			// Normalize British spelling
			if americanSpelling, ok := spellingMap[word]; ok {
				word = americanSpelling
			}

			keywordSet[word] = true

			// Add singular form if word ends in 's' (simple stemming)
			if strings.HasSuffix(word, "s") && len(word) > 3 {
				singular := word[:len(word)-1]
				keywordSet[singular] = true
			}
			// Add plural form if word doesn't end in 's'
			if !strings.HasSuffix(word, "s") {
				keywordSet[word+"s"] = true
			}
		}
	}

	var keywords []string
	for k := range keywordSet {
		keywords = append(keywords, k)
	}

	return keywords
}

func filterKeywordsByCompound(keywords []string, compoundTerms []string) []string {
	if len(compoundTerms) == 0 {
		return keywords
	}

	// Build a set of words that are part of compound terms
	compoundWords := make(map[string]bool)
	for _, term := range compoundTerms {
		for _, word := range strings.Fields(term) {
			compoundWords[word] = true
		}
	}

	// Filter out keywords that are part of compound terms
	var filtered []string
	for _, kw := range keywords {
		if !compoundWords[kw] {
			filtered = append(filtered, kw)
		}
	}

	return filtered
}

func extractCompoundTerms(query string) []string {
	// Known compound terms to look for
	compoundPatterns := []string{
		"android auto",
		"apple carplay",
		"lane assist",
		"high beam",
		"rear view",
		"side airbag",
		"curtain airbag",
		"blind spot",
		"parking sensor",
		"fuel efficiency",
		"fuel economy",
		"kerb weight",
		"ground clearance",
		"boot space",
		"trunk space",
		"touch screen",
		"color option",
		"colour option",
		"exterior color",
		"interior color",
		"safety features",
		"safety feature",
		"child safety",
		"children safety",
	}

	queryLower := strings.ToLower(query)
	var found []string

	for _, pattern := range compoundPatterns {
		if strings.Contains(queryLower, pattern) {
			found = append(found, pattern)
		}
	}

	return found
}

func filterRelevantSpecs(specs []scoredSpec, query string, keywords []string) []scoredSpec {
	queryLower := strings.ToLower(query)
	
	// If query mentions "children" or "child", prioritize safety features
	if strings.Contains(queryLower, "child") || strings.Contains(queryLower, "children") {
		var safetySpecs []scoredSpec
		var otherRelevant []scoredSpec
		
		for _, s := range specs {
			categoryLower := strings.ToLower(s.Category)
			nameLower := strings.ToLower(s.Name)
			valueLower := strings.ToLower(s.Value)
			
			// Prioritize safety features
			if strings.Contains(categoryLower, "safety") || 
			   strings.Contains(nameLower, "safety") ||
			   strings.Contains(valueLower, "airbag") ||
			   strings.Contains(valueLower, "safety") {
				safetySpecs = append(safetySpecs, s)
			} else if strings.Contains(categoryLower, "seat") || 
			          strings.Contains(categoryLower, "rear") ||
			          strings.Contains(nameLower, "seat") ||
			          strings.Contains(nameLower, "rear") {
				// Also include rear seat features (relevant for children)
				otherRelevant = append(otherRelevant, s)
			}
		}
		
		// Combine: safety first, then other relevant
		result := append(safetySpecs, otherRelevant...)
		if len(result) > 0 {
			return result
		}
		// Fallback: return original if nothing matches
		return specs
	}
	
	// For other queries, filter out completely unrelated categories
	// (e.g., if query is about safety, don't show fuel efficiency)
	irrelevantCategories := []string{"performance", "fuel", "display", "comfort", "multimedia"}
	if strings.Contains(queryLower, "safety") {
		var filtered []scoredSpec
		for _, s := range specs {
			categoryLower := strings.ToLower(s.Category)
			isIrrelevant := false
			for _, irr := range irrelevantCategories {
				if strings.Contains(categoryLower, irr) && !strings.Contains(categoryLower, "safety") {
					isIrrelevant = true
					break
				}
			}
			if !isIrrelevant {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}
	
	return specs
}

func runMigrations(db *sql.DB) error {
	migrations := `
	CREATE TABLE IF NOT EXISTS tenants (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		plan_tier TEXT DEFAULT 'sandbox',
		contact_email TEXT,
		settings TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		name TEXT NOT NULL,
		segment TEXT,
		body_type TEXT,
		model_year INTEGER,
		is_public_benchmark INTEGER DEFAULT 0,
		default_campaign_variant_id TEXT,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaign_variants (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		locale TEXT NOT NULL DEFAULT 'en-US',
		trim TEXT,
		market TEXT,
		status TEXT DEFAULT 'draft',
		version INTEGER DEFAULT 1,
		effective_from DATETIME,
		effective_through DATETIME,
		is_draft INTEGER DEFAULT 1,
		last_published_by TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS spec_categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		display_order INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS spec_items (
		id TEXT PRIMARY KEY,
		category_id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		unit TEXT,
		data_type TEXT DEFAULT 'text',
		validation_rules TEXT DEFAULT '{}',
		aliases TEXT DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS spec_values (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		campaign_variant_id TEXT NOT NULL,
		spec_item_id TEXT NOT NULL,
		value_numeric REAL,
		value_text TEXT,
		unit TEXT,
		confidence REAL DEFAULT 1.0,
		status TEXT DEFAULT 'active',
		source_doc_id TEXT,
		source_page INTEGER,
		version INTEGER DEFAULT 1,
		effective_from DATETIME,
		effective_through DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS knowledge_chunks (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		campaign_variant_id TEXT,
		chunk_type TEXT NOT NULL,
		text TEXT NOT NULL,
		metadata TEXT DEFAULT '{}',
		embedding_vector BLOB,
		embedding_model TEXT,
		embedding_version TEXT,
		source_doc_id TEXT,
		source_page INTEGER,
		visibility TEXT DEFAULT 'private',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_spec_values_search ON spec_values(tenant_id, campaign_variant_id, status);
	CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_search ON knowledge_chunks(tenant_id, campaign_variant_id);
	`

	_, err := db.Exec(migrations)
	return err
}

