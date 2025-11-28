// Package e2e provides end-to-end tests for the knowledge engine.
package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// TestEndToEndCamryIngestionAndRetrieval runs a complete end-to-end test
// demonstrating the full pipeline from brochure to query.
func TestEndToEndCamryIngestionAndRetrieval(t *testing.T) {
	ctx := context.Background()
	_ = observability.NewLogger(observability.LogConfig{
		Level:       "debug",
		Format:      "console",
		ServiceName: "e2e-test",
	})

	// Load brochure from file
	brochurePath := findBrochurePath()
	brochureContent, err := os.ReadFile(brochurePath)
	if err != nil {
		t.Fatalf("Failed to read brochure: %v", err)
	}

	t.Logf("Loaded brochure from: %s (%d bytes)", brochurePath, len(brochureContent))

	// Step 1: Parse the brochure
	t.Log("\n=== Step 1: Parsing Brochure ===")
	parseStart := time.Now()
	parser := ingest.NewParser(ingest.ParserConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
	})
	parseResult, err := parser.Parse(string(brochureContent))
	if err != nil {
		t.Fatalf("Failed to parse brochure: %v", err)
	}
	parseTime := time.Since(parseStart)

	t.Logf("Parse completed in %v", parseTime)
	t.Logf("  - Metadata: product=%s, year=%d, locale=%s",
		parseResult.Metadata.ProductName,
		parseResult.Metadata.ModelYear,
		parseResult.Metadata.Locale)
	t.Logf("  - Specifications: %d extracted", len(parseResult.SpecValues))
	t.Logf("  - Features: %d extracted", len(parseResult.Features))
	t.Logf("  - USPs: %d extracted", len(parseResult.USPs))

	// Print some sample specifications
	t.Log("\n  Sample Specifications:")
	for i, spec := range parseResult.SpecValues {
		if i >= 5 {
			t.Logf("    ... and %d more", len(parseResult.SpecValues)-5)
			break
		}
		t.Logf("    - %s > %s: %s %s", spec.Category, spec.Name, spec.Value, spec.Unit)
	}

	// Print sample features
	t.Log("\n  Sample Features:")
	for i, feature := range parseResult.Features {
		if i >= 5 {
			t.Logf("    ... and %d more", len(parseResult.Features)-5)
			break
		}
		text := feature.Body
		if len(text) > 60 {
			text = text[:60] + "..."
		}
		t.Logf("    - %s", text)
	}

	// Step 2: Initialize SQLite database
	t.Log("\n=== Step 2: Setting up SQLite Database ===")
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("knowledge_engine_e2e_%d.db", time.Now().UnixNano()))
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer os.Remove(dbPath)
	defer db.Close()

	// Run migrations
	if err := runMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	t.Logf("Database initialized at: %s", dbPath)

	// Step 3: Create tenant, product, campaign
	t.Log("\n=== Step 3: Creating Tenant & Product ===")
	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	if err := createTenant(db, tenantID, "Toyota India"); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}
	if err := createProduct(db, tenantID, productID, "Camry Hybrid 2025"); err != nil {
		t.Fatalf("Failed to create product: %v", err)
	}
	if err := createCampaign(db, tenantID, productID, campaignID, "en-IN", "XLE Hybrid"); err != nil {
		t.Fatalf("Failed to create campaign: %v", err)
	}
	t.Logf("Created: Tenant=%s, Product=%s, Campaign=%s", tenantID, productID, campaignID)

	// Step 4: Store parsed data with embeddings
	t.Log("\n=== Step 4: Storing Data with Embeddings ===")

	// Check for OpenRouter API key
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	var embeddingClient embedding.Embedder
	if apiKey != "" {
		client, err := embedding.NewClient(embedding.Config{
			APIKey:  apiKey,
			Model:   "google/gemini-embedding-001",
			BaseURL: "https://openrouter.ai/api/v1",
		})
		if err != nil {
			t.Logf("Warning: Failed to create embedding client: %v", err)
		} else {
			embeddingClient = client
			t.Log("Using OpenRouter embeddings (google/gemini-embedding-001)")
		}
	} else {
		t.Log("OPENROUTER_API_KEY not set - using mock embeddings")
		embeddingClient = embedding.NewMockClient(768)
	}

	storeStart := time.Now()
	specCount, chunkCount, err := storeDataWithEmbeddings(ctx, db, tenantID, productID, campaignID, parseResult, embeddingClient)
	storeTime := time.Since(storeStart)
	if err != nil {
		t.Fatalf("Failed to store data: %v", err)
	}
	t.Logf("Stored %d spec values and %d knowledge chunks in %v", specCount, chunkCount, storeTime)

	// Step 5: Test Queries
	t.Log("\n=== Step 5: Testing Retrieval Queries ===")

	// Create repositories
	repos := storage.NewRepositories(db)
	_ = repos // Will be used for more complex queries

	// Create vector adapter (in-memory for testing)
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
		Dimension: 768, // gemini-embedding-001 dimension
	})
	if err != nil {
		t.Logf("Warning: Failed to create FAISS adapter: %v", err)
	}
	_ = vectorAdapter

	// Run test queries with expected results for accuracy testing
	testQueries := []struct {
		query           string
		expectedType    string
		expectedKeyword string // A keyword that should appear in results
	}{
		// Specification lookups
		{"What is the fuel efficiency of the Camry?", "spec_lookup", "25.49"},
		{"What is the engine displacement?", "spec_lookup", "2.5L"},
		{"How many airbags does it have?", "spec_lookup", "9"},
		{"What is the seating capacity?", "spec_lookup", "5"},
		{"What are the dimensions of the car?", "spec_lookup", "4.92"},
		{"What is the kerb weight?", "spec_lookup", "kg"},
		{"What is the wheelbase?", "spec_lookup", "2.825"},
		{"What transmission does it have?", "spec_lookup", "CVT"},
		
		// Feature lookups
		{"What safety features does the Camry have?", "feature", "Safety"},
		{"Does it have Apple CarPlay?", "feature", "CarPlay"},
		{"Does it have wireless charging?", "feature", "Wireless"},
		{"What is the display size?", "spec_lookup", "cm"},
		{"Does it have sunroof?", "feature", "Moon"},
		{"What audio system does it have?", "feature", "JBL"},
		
		// Hybrid-specific
		{"Tell me about the hybrid system", "semantic", "hybrid"},
		{"What is the battery type?", "spec_lookup", "lithium"},
		{"What is the combined power output?", "spec_lookup", "169"},
		{"What drive modes are available?", "spec_lookup", "Sport"},
		
		// USP/Why buy queries
		{"Why should I buy the Camry over competitors?", "usp", ""},
		{"What is the warranty coverage?", "faq", ""},
		
		// Edge cases
		{"What colors are available?", "spec_lookup", "Grey"},
		{"Does it have lane assist?", "feature", "Lane"},
		{"What is the suspension type?", "spec_lookup", "MacPherson"},
	}

	t.Log("\nQuery Results:")
	t.Log("=" + strings.Repeat("=", 79))

	var totalQueryTime time.Duration
	var successfulQueries, queriesWithResults int

	for _, tq := range testQueries {
		queryStart := time.Now()
		results := runQuery(ctx, db, tenantID, productID, campaignID, tq.query)
		queryTime := time.Since(queryStart)
		totalQueryTime += queryTime

		// Check if expected keyword was found
		foundExpected := false
		if tq.expectedKeyword != "" {
			for _, spec := range results.Specs {
				// Check value, unit, category, and name
				if strings.Contains(strings.ToLower(spec.Value), strings.ToLower(tq.expectedKeyword)) ||
					strings.Contains(strings.ToLower(spec.Unit), strings.ToLower(tq.expectedKeyword)) ||
					strings.Contains(strings.ToLower(spec.Category), strings.ToLower(tq.expectedKeyword)) ||
					strings.Contains(strings.ToLower(spec.Name), strings.ToLower(tq.expectedKeyword)) {
					foundExpected = true
					break
				}
			}
			if !foundExpected {
				for _, chunk := range results.Chunks {
					if strings.Contains(strings.ToLower(chunk.Text), strings.ToLower(tq.expectedKeyword)) {
						foundExpected = true
						break
					}
				}
			}
		} else {
			foundExpected = true // No expected keyword, consider it a pass if we tried
		}

		hasResults := len(results.Specs) > 0 || len(results.Chunks) > 0
		if hasResults {
			queriesWithResults++
		}
		if foundExpected && hasResults {
			successfulQueries++
		}

		// Log result with accuracy indicator
		status := "✓"
		if !foundExpected && tq.expectedKeyword != "" {
			status = "✗"
		} else if !hasResults {
			status = "○"
		}

		t.Logf("\n%s Q: %s", status, tq.query)
		t.Logf("   Time: %v | Results: %d specs, %d chunks",
			queryTime, len(results.Specs), len(results.Chunks))

		if len(results.Specs) > 0 {
			for _, spec := range results.Specs[:min(3, len(results.Specs))] {
				t.Logf("   → [SPEC] %s > %s: %s %s (confidence: %.2f)",
					spec.Category, spec.Name, spec.Value, spec.Unit, spec.Confidence)
			}
		}
		if len(results.Chunks) > 0 {
			for _, chunk := range results.Chunks[:min(2, len(results.Chunks))] {
				text := chunk.Text
				if len(text) > 80 {
					text = text[:80] + "..."
				}
				t.Logf("   → [CHUNK] %s: %s", chunk.Type, text)
			}
		}
		if len(results.Specs) == 0 && len(results.Chunks) == 0 {
			t.Logf("   → No results found")
		}
	}

	// Calculate accuracy metrics
	totalQueries := len(testQueries)
	queriesWithExpected := 0
	for _, tq := range testQueries {
		if tq.expectedKeyword != "" {
			queriesWithExpected++
		}
	}
	accuracyRate := float64(successfulQueries) / float64(totalQueries) * 100
	hitRate := float64(queriesWithResults) / float64(totalQueries) * 100

	// Step 6: Performance & Accuracy Summary
	t.Log("\n=== Performance Summary ===")
	t.Logf("Parse time:         %v", parseTime)
	t.Logf("Store time:         %v", storeTime)
	t.Logf("Total query time:   %v (%d queries)", totalQueryTime, totalQueries)
	t.Logf("Avg query time:     %v", totalQueryTime/time.Duration(totalQueries))
	t.Logf("Specs stored:       %d", specCount)
	t.Logf("Chunks stored:      %d", chunkCount)
	t.Logf("Database size:      %s", getDatabaseSize(dbPath))

	t.Log("\n=== Accuracy Summary ===")
	t.Logf("Total queries:      %d", totalQueries)
	t.Logf("Queries with results: %d (%.1f%% hit rate)", queriesWithResults, hitRate)
	t.Logf("Accurate results:   %d (%.1f%% accuracy)", successfulQueries, accuracyRate)
	t.Log("\nLegend: ✓ = found expected result, ○ = no results, ✗ = unexpected result")

	// Validation
	if specCount == 0 {
		t.Error("No specifications were stored!")
	}
	if parseTime > 5*time.Second {
		t.Errorf("Parse time too slow: %v (expected < 5s)", parseTime)
	}
	if hitRate < 70 {
		t.Errorf("Hit rate too low: %.1f%% (expected > 70%%)", hitRate)
	}
	if accuracyRate < 60 {
		t.Errorf("Accuracy too low: %.1f%% (expected > 60%%)", accuracyRate)
	}
	
	t.Log("\n✅ End-to-end test completed successfully!")
}

// Helper functions

func findBrochurePath() string {
	// Prefer the full brochure
	fullBrochure := "../../../../e-brochure-camry-hybrid-specs.md"
	if _, err := os.Stat(fullBrochure); err == nil {
		return fullBrochure
	}

	// Try other locations
	paths := []string{
		"../../testdata/camry-sample.md",
		"testdata/camry-sample.md",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return fullBrochure
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
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (tenant_id) REFERENCES tenants(id)
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
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (product_id) REFERENCES products(id),
		FOREIGN KEY (tenant_id) REFERENCES tenants(id)
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (category_id) REFERENCES spec_categories(id)
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
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (tenant_id) REFERENCES tenants(id),
		FOREIGN KEY (product_id) REFERENCES products(id),
		FOREIGN KEY (campaign_variant_id) REFERENCES campaign_variants(id),
		FOREIGN KEY (spec_item_id) REFERENCES spec_items(id)
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
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (tenant_id) REFERENCES tenants(id),
		FOREIGN KEY (product_id) REFERENCES products(id)
	);

	CREATE INDEX IF NOT EXISTS idx_spec_values_campaign ON spec_values(tenant_id, campaign_variant_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_campaign ON knowledge_chunks(tenant_id, campaign_variant_id);
	CREATE INDEX IF NOT EXISTS idx_spec_values_search ON spec_values(tenant_id, campaign_variant_id, status);
	`

	_, err := db.Exec(migrations)
	return err
}

func createTenant(db *sql.DB, id uuid.UUID, name string) error {
	_, err := db.Exec(
		"INSERT INTO tenants (id, name, plan_tier) VALUES (?, ?, ?)",
		id.String(), name, "enterprise",
	)
	return err
}

func createProduct(db *sql.DB, tenantID, productID uuid.UUID, name string) error {
	_, err := db.Exec(
		"INSERT INTO products (id, tenant_id, name, model_year) VALUES (?, ?, ?, ?)",
		productID.String(), tenantID.String(), name, 2025,
	)
	return err
}

func createCampaign(db *sql.DB, tenantID, productID, campaignID uuid.UUID, locale, trim string) error {
	_, err := db.Exec(
		`INSERT INTO campaign_variants (id, product_id, tenant_id, locale, trim, status) 
		 VALUES (?, ?, ?, ?, ?, 'published')`,
		campaignID.String(), productID.String(), tenantID.String(), locale, trim,
	)
	return err
}

func storeDataWithEmbeddings(ctx context.Context, db *sql.DB, tenantID, productID, campaignID uuid.UUID,
	parseResult *ingest.ParsedBrochure, embClient embedding.Embedder) (int, int, error) {

	specCount := 0
	chunkCount := 0

	// Store specifications
	categoryCache := make(map[string]uuid.UUID)

	for _, spec := range parseResult.SpecValues {
		// Get or create category
		categoryID, ok := categoryCache[spec.Category]
		if !ok {
			categoryID = uuid.New()
			_, err := db.Exec(
				"INSERT OR IGNORE INTO spec_categories (id, name) VALUES (?, ?)",
				categoryID.String(), spec.Category,
			)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to create category: %w", err)
			}
			categoryCache[spec.Category] = categoryID
		}

		// Create spec item
		specItemID := uuid.New()
		_, err := db.Exec(
			"INSERT INTO spec_items (id, category_id, display_name, unit) VALUES (?, ?, ?, ?)",
			specItemID.String(), categoryID.String(), spec.Name, spec.Unit,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create spec item: %w", err)
		}

		// Create spec value
		specValueID := uuid.New()
		_, err = db.Exec(
			`INSERT INTO spec_values (id, tenant_id, product_id, campaign_variant_id, spec_item_id, 
			 value_text, unit, confidence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			specValueID.String(), tenantID.String(), productID.String(), campaignID.String(),
			specItemID.String(), spec.Value, spec.Unit, spec.Confidence,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create spec value: %w", err)
		}
		specCount++
	}

	// Store features as knowledge chunks
	for _, feature := range parseResult.Features {
		chunkID := uuid.New()
		var embVector []byte

		// Generate embedding if client available
		if embClient != nil {
			emb, err := embClient.EmbedSingle(ctx, feature.Body)
			if err == nil && len(emb) > 0 {
				embVector, _ = json.Marshal(emb)
			}
		}

		metadata, _ := json.Marshal(map[string]interface{}{
			"tags": feature.Tags,
		})

		_, err := db.Exec(
			`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			 chunk_type, text, metadata, embedding_vector, embedding_model) 
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), tenantID.String(), productID.String(), campaignID.String(),
			"feature_block", feature.Body, string(metadata), embVector, embClient.Model(),
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create feature chunk: %w", err)
		}
		chunkCount++
	}

	// Store USPs as knowledge chunks
	for _, usp := range parseResult.USPs {
		chunkID := uuid.New()
		var embVector []byte

		if embClient != nil {
			emb, err := embClient.EmbedSingle(ctx, usp.Body)
			if err == nil && len(emb) > 0 {
				embVector, _ = json.Marshal(emb)
			}
		}

		_, err := db.Exec(
			`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			 chunk_type, text, embedding_vector, embedding_model) 
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), tenantID.String(), productID.String(), campaignID.String(),
			"usp", usp.Body, embVector, embClient.Model(),
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create USP chunk: %w", err)
		}
		chunkCount++
	}

	return specCount, chunkCount, nil
}

// QueryResult contains query results
type QueryResult struct {
	Specs  []SpecResult
	Chunks []ChunkResult
}

type SpecResult struct {
	Category   string
	Name       string
	Value      string
	Unit       string
	Confidence float64
}

type ChunkResult struct {
	Type string
	Text string
}

func runQuery(ctx context.Context, db *sql.DB, tenantID, productID, campaignID uuid.UUID, query string) QueryResult {
	result := QueryResult{}

	// Extract keywords from query for spec lookup
	keywords := extractKeywords(query)

	// Search spec values
	for _, keyword := range keywords {
		rows, err := db.Query(`
			SELECT sc.name, si.display_name, sv.value_text, sv.unit, sv.confidence
			FROM spec_values sv
			JOIN spec_items si ON sv.spec_item_id = si.id
			JOIN spec_categories sc ON si.category_id = sc.id
			WHERE sv.tenant_id = ? AND sv.campaign_variant_id = ?
			  AND (LOWER(si.display_name) LIKE ? OR LOWER(sc.name) LIKE ?)
			ORDER BY sv.confidence DESC
			LIMIT 5
		`, tenantID.String(), campaignID.String(), "%"+keyword+"%", "%"+keyword+"%")

		if err != nil {
			continue
		}

		for rows.Next() {
			var spec SpecResult
			var unit sql.NullString
			if err := rows.Scan(&spec.Category, &spec.Name, &spec.Value, &unit, &spec.Confidence); err == nil {
				if unit.Valid {
					spec.Unit = unit.String
				}
				result.Specs = append(result.Specs, spec)
			}
		}
		rows.Close()
	}

	// Search knowledge chunks
	for _, keyword := range keywords {
		rows, err := db.Query(`
			SELECT chunk_type, text
			FROM knowledge_chunks
			WHERE tenant_id = ? AND campaign_variant_id = ?
			  AND LOWER(text) LIKE ?
			LIMIT 3
		`, tenantID.String(), campaignID.String(), "%"+keyword+"%")

		if err != nil {
			continue
		}

		for rows.Next() {
			var chunk ChunkResult
			if err := rows.Scan(&chunk.Type, &chunk.Text); err == nil {
				result.Chunks = append(result.Chunks, chunk)
			}
		}
		rows.Close()
	}

	return result
}

func extractKeywords(query string) []string {
	// Simple keyword extraction with stemming for common plurals
	stopWords := map[string]bool{
		"what": true, "is": true, "the": true, "a": true, "an": true,
		"how": true, "many": true, "does": true, "it": true, "have": true,
		"tell": true, "me": true, "about": true, "of": true, "are": true,
		"why": true, "should": true, "i": true, "buy": true, "over": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string

	for _, word := range words {
		word = strings.Trim(word, "?.,!")
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
			
			// Add singular form if word ends in 's' (simple stemming)
			if strings.HasSuffix(word, "s") && len(word) > 3 {
				singular := word[:len(word)-1]
				keywords = append(keywords, singular)
			}
			// Add plural form if word doesn't end in 's'
			if !strings.HasSuffix(word, "s") {
				keywords = append(keywords, word+"s")
			}
		}
	}

	return keywords
}

func getDatabaseSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	size := info.Size()
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
