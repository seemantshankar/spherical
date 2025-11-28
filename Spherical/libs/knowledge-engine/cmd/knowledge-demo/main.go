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
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
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
	db            *sql.DB
	tenantID      uuid.UUID
	productID     uuid.UUID
	campaignID    uuid.UUID
	embedder      embedding.Embedder
	vectorAdapter *retrieval.FAISSAdapter
	router        *retrieval.Router
	specViewRepo  *storage.SpecViewRepository
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
		// Google Gemini embedding-001 - dimension will be detected from API response
		// Don't set dimension here, let it be auto-detected from first embedding
		client, err := embedding.NewClient(embedding.Config{
			APIKey:  apiKey,
			Model:   "google/gemini-embedding-001",
			BaseURL: "https://openrouter.ai/api/v1",
			// Dimension: not set - will be detected from API response
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

	// Initialize vector adapter - dimension will be set dynamically when first embedding is generated
	// Start with a default that matches the embedder, but it will be updated on first use
	embedderDimension := 768 // Default fallback
	if embedder != nil {
		embedderDimension = embedder.Dimension()
	}
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: embedderDimension})

	// Create spec view repository
	specViewRepo := storage.NewSpecViewRepository(db)

	// Create cache
	memCache := cache.NewMemoryClient(1000)

	// Create logger for router
	routerLogger := observability.NewLogger(observability.LogConfig{
		Level:       "info",
		Format:      "console",
		ServiceName: "knowledge-demo-router",
	})

	// Create router with production configuration
	router := retrieval.NewRouter(
		routerLogger,
		memCache,
		vectorAdapter,
		embedder, // Pass embedder for vector search
		specViewRepo,
		retrieval.RouterConfig{
			StructuredFirst:           true,
			SemanticFallback:          true,
			IntentConfidenceThreshold: 0.7,
			KeywordConfidenceThreshold: 0.8,
			CacheResults:             true,
			CacheTTL:                 5 * time.Minute,
		},
	)

	kb := &KnowledgeBase{
		db:            db,
		embedder:      embedder,
		vectorAdapter: vectorAdapter,
		router:        router,
		specViewRepo:  specViewRepo,
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
		kb.loadData(ctx)
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
		if query == "stats" {
			kb.printStats()
			continue
		}
		if query == "reload" {
			fmt.Println(colorYellow + "Reloading data..." + colorReset)
			kb.loadData(ctx)
			kb.printStats()
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(query, "/") {
			kb.handleCommand(query)
			continue
		}

		// Use Router to process regular queries
		kb.runQueryWithRouter(ctx, query)
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

func (kb *KnowledgeBase) loadData(ctx context.Context) {
	kb.db.QueryRow("SELECT id FROM tenants LIMIT 1").Scan(&kb.tenantID)
	kb.db.QueryRow("SELECT id FROM products LIMIT 1").Scan(&kb.productID)
	kb.db.QueryRow("SELECT id FROM campaign_variants LIMIT 1").Scan(&kb.campaignID)

	// Load vectors
	fmt.Print("Loading vectors into memory... ")
	rows, err := kb.db.Query("SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type, embedding_vector, metadata FROM knowledge_chunks")
	if err != nil {
		fmt.Printf("Error loading vectors: %v\n", err)
		return
	}
	defer rows.Close()

	count := 0
	var entries []retrieval.VectorEntry
	for rows.Next() {
		var id, tenantID, productID string
		var campaignID sql.NullString
		var chunkType string
		var embBytes []byte
		var metadataJSON string

		if err := rows.Scan(&id, &tenantID, &productID, &campaignID, &chunkType, &embBytes, &metadataJSON); err != nil {
			continue
		}

		if len(embBytes) == 0 {
			continue
		}

		var vector []float32
		if err := json.Unmarshal(embBytes, &vector); err != nil {
			continue
		}

		var metadata map[string]interface{}
		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &metadata)
		}

		entry := retrieval.VectorEntry{
			ID:        uuid.MustParse(id),
			TenantID:  uuid.MustParse(tenantID),
			ProductID: uuid.MustParse(productID),
			ChunkType: chunkType,
			Vector:    vector,
			Metadata:  metadata,
		}
		if campaignID.Valid {
			cid := uuid.MustParse(campaignID.String)
			entry.CampaignVariantID = &cid
		}

		entries = append(entries, entry)
		count++
	}

	if len(entries) > 0 {
		kb.vectorAdapter.Insert(ctx, entries)
	}
	fmt.Printf("✓ Loaded %d vectors\n", count)
}

func (kb *KnowledgeBase) ingestBrochure(ctx context.Context) error {
	// Find brochure - prioritize camry-output-v3.md which has all 49 USPs
	brochurePaths := []string{
		"../pdf-extractor/camry-output-v3.md",
		"../../pdf-extractor/camry-output-v3.md",
		"../../../pdf-extractor/camry-output-v3.md",
		"pdf-extractor/camry-output-v3.md",
		// Fallback to older file if camry-output-v3.md not found
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

		// Embed spec for vector search
		if kb.embedder != nil {
			specText := fmt.Sprintf("%s: %s - %s %s", spec.Category, spec.Name, spec.Value, spec.Unit)
			
			// Simple retry logic for rate limits
			var emb []float32
			var err error
			for retry := 0; retry < 3; retry++ {
				emb, err = kb.embedder.EmbedSingle(ctx, specText)
				if err == nil {
					break
				}
				// If rate limited, wait and retry
				time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
			}

			if err == nil {
				chunkID := uuid.New()
				embBytes, _ := json.Marshal(emb)
				metadata := map[string]interface{}{
					"spec_value_id": specValueID.String(),
					"category":      spec.Category,
					"name":          spec.Name,
				}
				metadataBytes, _ := json.Marshal(metadata)

				kb.db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
					chunk_type, text, metadata, embedding_vector, embedding_model) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					chunkID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
					"spec_row", specText, string(metadataBytes), embBytes, kb.embedder.Model())

				kb.vectorAdapter.Insert(ctx, []retrieval.VectorEntry{{
					ID:                chunkID,
					TenantID:          kb.tenantID,
					ProductID:         kb.productID,
					CampaignVariantID: &kb.campaignID,
					ChunkType:         "spec_row",
					Vector:            emb,
					Metadata:          metadata,
				}})
			} else {
				fmt.Printf("Warning: failed to embed spec: %v\n", err)
			}
		}
	}
	fmt.Printf("%s✓%s\n", colorGreen, colorReset)

	// Store features and USPs with embeddings
	totalChunks := len(result.Features) + len(result.USPs)
	fmt.Printf("Storing %d knowledge chunks (features/USPs) with embeddings... ", totalChunks)

	chunkCount := 0
	for _, feature := range result.Features {
		chunkID := uuid.New()
		var embVector []byte
		var emb []float32
		if kb.embedder != nil {
			if e, err := kb.embedder.EmbedSingle(ctx, feature.Body); err == nil {
				emb = e
				embVector, _ = json.Marshal(emb)
			}
		}
		meta := map[string]interface{}{"tags": feature.Tags}
		metadata, _ := json.Marshal(meta)
		kb.db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, metadata, embedding_vector, embedding_model) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
			"feature_block", feature.Body, string(metadata), embVector, kb.embedder.Model())

		if len(emb) > 0 {
			kb.vectorAdapter.Insert(ctx, []retrieval.VectorEntry{{
				ID:                chunkID,
				TenantID:          kb.tenantID,
				ProductID:         kb.productID,
				CampaignVariantID: &kb.campaignID,
				ChunkType:         "feature_block",
				Vector:            emb,
				Metadata:          meta,
			}})
		}

		chunkCount++
		if chunkCount%20 == 0 {
			fmt.Printf("\rStoring %d knowledge chunks with embeddings... %d/%d", totalChunks, chunkCount, totalChunks)
		}
	}

	for _, usp := range result.USPs {
		chunkID := uuid.New()
		var embVector []byte
		var emb []float32
		if kb.embedder != nil {
			if e, err := kb.embedder.EmbedSingle(ctx, usp.Body); err == nil {
				emb = e
				embVector, _ = json.Marshal(emb)
			} else {
				// Log embedding errors but continue - USP can still be stored without embedding
				fmt.Printf("\nWarning: Failed to generate embedding for USP: %v\n", err)
			}
		}
		
		// Insert into database with error checking
		_, err := kb.db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, embedding_vector, embedding_model) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), kb.tenantID.String(), kb.productID.String(), kb.campaignID.String(),
			"usp", usp.Body, embVector, kb.embedder.Model())
		if err != nil {
			fmt.Printf("\nError storing USP in database: %v\n", err)
			continue // Skip this USP if database insert fails
		}

		if len(emb) > 0 {
			if err := kb.vectorAdapter.Insert(ctx, []retrieval.VectorEntry{{
				ID:                chunkID,
				TenantID:          kb.tenantID,
				ProductID:         kb.productID,
				CampaignVariantID: &kb.campaignID,
				ChunkType:         "usp",
				Vector:            emb,
				Metadata:          map[string]interface{}{"type": "usp"},
			}}); err != nil {
				fmt.Printf("\nError inserting USP into vector adapter: %v\n", err)
			}
		}

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

// runQueryWithRouter uses the production Router to process queries.
func (kb *KnowledgeBase) runQueryWithRouter(ctx context.Context, query string) {
	if kb.router == nil {
		fmt.Printf("%sError: Router not initialized%s\n", colorRed, colorReset)
		return
	}

	// Create retrieval request
	// For USP queries, request more chunks to get all USPs
	maxChunks := 10
	if strings.Contains(strings.ToLower(query), "usp") || strings.Contains(strings.ToLower(query), "unique selling") {
		maxChunks = 50 // Get all USPs
	}
	req := retrieval.RetrievalRequest{
		TenantID:          kb.tenantID,
		ProductIDs:        []uuid.UUID{kb.productID},
		CampaignVariantID: &kb.campaignID,
		Question:          query,
		MaxChunks:         maxChunks,
	}

	// Query using Router
	resp, err := kb.router.Query(ctx, req)
	if err != nil {
		fmt.Printf("%sError: %v%s\n", colorRed, err, colorReset)
		return
	}

	// Display results
	fmt.Printf("\n%s📊 Results (Intent: %s, Latency: %dms)%s\n", colorBold, resp.Intent, resp.LatencyMs, colorReset)

	// Show structured facts
	if len(resp.StructuredFacts) > 0 {
		fmt.Printf("\n%s📋 Structured Facts:%s\n", colorBold, colorReset)
		for i, fact := range resp.StructuredFacts {
			if i >= 10 {
				break
			}
			fmt.Printf("  %s%s%s: %s%s %s%s\n",
				colorCyan, fact.Category, colorReset,
				colorGreen, fact.Name, colorReset,
				fact.Value)
			if fact.Unit != "" {
				fmt.Printf("    %s%s%s\n", colorYellow, fact.Unit, colorReset)
			}
		}
	}

	// Show semantic chunks
	if len(resp.SemanticChunks) > 0 {
		fmt.Printf("\n%s💡 Related Information:%s\n", colorBold, colorReset)
		// For USP queries, show all chunks; for others, limit to 5
		maxDisplay := 5
		if resp.Intent == retrieval.IntentUSPLookup {
			maxDisplay = 100 // Show all USPs
		}
		for i, chunk := range resp.SemanticChunks {
			if i >= maxDisplay {
				break
			}
			// Get chunk text from database (it's not included in VectorResult)
			var chunkText string
			err := kb.db.QueryRow("SELECT text FROM knowledge_chunks WHERE id = ?", chunk.ChunkID.String()).Scan(&chunkText)
			if err == nil && chunkText != "" {
				// Truncate long text for display
				if len(chunkText) > 200 {
					chunkText = chunkText[:197] + "..."
				}
				typeIcon := "📝"
				if string(chunk.ChunkType) == "usp" {
					typeIcon = "⭐"
				}
				fmt.Printf("  %s %s\n", typeIcon, chunkText)
			}
		}
		if resp.Intent == retrieval.IntentUSPLookup && len(resp.SemanticChunks) > maxDisplay {
			fmt.Printf("  ... and %d more USP(s)\n", len(resp.SemanticChunks)-maxDisplay)
		}
	}

	// Show if no results
	if len(resp.StructuredFacts) == 0 && len(resp.SemanticChunks) == 0 {
		fmt.Printf("%s⚠ No results found. Try rephrasing your question.%s\n", colorYellow, colorReset)
	}

	fmt.Println()
}

// runQuery is kept for backward compatibility but now delegates to Router.
func (kb *KnowledgeBase) runQuery(ctx context.Context, query string) {
	kb.runQueryWithRouter(ctx, query)
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

	-- Create spec_view_latest as a regular view (SQLite doesn't support materialized views)
	CREATE VIEW IF NOT EXISTS spec_view_latest AS
	SELECT 
		sv.id,
		sv.tenant_id,
		sv.product_id,
		sv.campaign_variant_id,
		sv.spec_item_id,
		si.display_name AS spec_name,
		sc.name AS category_name,
		COALESCE(sv.value_text, CAST(sv.value_numeric AS TEXT)) AS value,
		sv.unit,
		sv.confidence,
		sv.source_doc_id,
		sv.source_page,
		sv.version,
		cv.locale,
		cv.trim,
		cv.market,
		p.name AS product_name
	FROM spec_values sv
	JOIN spec_items si ON sv.spec_item_id = si.id
	JOIN spec_categories sc ON si.category_id = sc.id
	JOIN campaign_variants cv ON sv.campaign_variant_id = cv.id
	JOIN products p ON sv.product_id = p.id
	WHERE sv.status = 'active'
	  AND cv.status = 'published';
	`

	_, err := db.Exec(migrations)
	return err
}

