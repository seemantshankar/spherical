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
	"github.com/spf13/cobra"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

func newDemoCmd() *cobra.Command {
	var brochurePath string
	var dbPath string
	var useRealEmbeddings bool

	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Interactive demo of the Knowledge Engine",
		Long: `Run an interactive demo that ingests a brochure and allows you to query it.

This demo:
1. Parses a Toyota Camry brochure (or your specified file)
2. Extracts specifications, features, and USPs
3. Stores data in SQLite with embeddings
4. Lets you query the knowledge base interactively

Example:
  knowledge-engine-cli demo
  knowledge-engine-cli demo --brochure /path/to/brochure.md
  knowledge-engine-cli demo --real-embeddings`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemo(brochurePath, dbPath, useRealEmbeddings)
		},
	}

	cmd.Flags().StringVarP(&brochurePath, "brochure", "b", "", "Path to brochure markdown file")
	cmd.Flags().StringVarP(&dbPath, "db", "d", "", "Path to SQLite database (default: temp file)")
	cmd.Flags().BoolVar(&useRealEmbeddings, "real-embeddings", false, "Use OpenRouter API for embeddings")

	return cmd
}

func runDemo(brochurePath, dbPath string, useRealEmbeddings bool) error {
	ctx := context.Background()
	logger := observability.NewLogger(observability.LogConfig{
		Level:       "info",
		Format:      "console",
		ServiceName: "demo",
	})

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║     🚗 Knowledge Engine Interactive Demo                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Find brochure
	if brochurePath == "" {
		brochurePath = findDefaultBrochure()
	}

	if brochurePath == "" {
		return fmt.Errorf("no brochure found. Please specify with --brochure flag")
	}

	fmt.Printf("📄 Loading brochure: %s\n", brochurePath)
	content, err := os.ReadFile(brochurePath)
	if err != nil {
		return fmt.Errorf("failed to read brochure: %w", err)
	}
	fmt.Printf("   Size: %d bytes\n\n", len(content))

	// Parse brochure
	fmt.Println("🔍 Parsing brochure...")
	parseStart := time.Now()
	parser := ingest.NewParser(ingest.ParserConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
	})
	parsed, err := parser.Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse: %w", err)
	}
	parseTime := time.Since(parseStart)

	fmt.Printf("   ✓ Parsed in %v\n", parseTime)
	fmt.Printf("   ✓ Specifications: %d\n", len(parsed.SpecValues))
	fmt.Printf("   ✓ Features: %d\n", len(parsed.Features))
	fmt.Printf("   ✓ USPs: %d\n\n", len(parsed.USPs))

	// Setup database
	if dbPath == "" {
		dbPath = filepath.Join(os.TempDir(), fmt.Sprintf("knowledge_demo_%d.db", time.Now().Unix()))
	}

	fmt.Printf("💾 Setting up database: %s\n", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := runDemoMigrations(db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create tenant/product
	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	if err := createDemoTenant(db, tenantID); err != nil {
		return err
	}
	if err := createDemoProduct(db, tenantID, productID); err != nil {
		return err
	}
	if err := createDemoCampaign(db, tenantID, productID, campaignID); err != nil {
		return err
	}

	// Setup embedding client
	var embClient embedding.Embedder
	if useRealEmbeddings {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("⚠️  OPENROUTER_API_KEY not set, using mock embeddings")
			embClient = embedding.NewMockClient(768)
		} else {
			client, err := embedding.NewClient(embedding.Config{
				APIKey:  apiKey,
				Model:   "google/gemini-embedding-001",
				BaseURL: "https://openrouter.ai/api/v1",
			})
			if err != nil {
				logger.Warn().Err(err).Msg("Failed to create embedding client")
				embClient = embedding.NewMockClient(768)
			} else {
				embClient = client
				fmt.Println("   Using OpenRouter embeddings (google/gemini-embedding-001)")
			}
		}
	} else {
		embClient = embedding.NewMockClient(768)
		fmt.Println("   Using mock embeddings (add --real-embeddings for OpenRouter)")
	}

	// Store data
	fmt.Println("\n📥 Storing data with embeddings...")
	storeStart := time.Now()
	specCount, chunkCount, err := storeDemoData(ctx, db, tenantID, productID, campaignID, parsed, embClient)
	storeTime := time.Since(storeStart)
	if err != nil {
		return fmt.Errorf("failed to store data: %w", err)
	}
	fmt.Printf("   ✓ Stored in %v\n", storeTime)
	fmt.Printf("   ✓ Specs: %d, Chunks: %d\n\n", specCount, chunkCount)

	// Interactive query loop
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("🎯 Interactive Query Mode")
	fmt.Println("   Type your questions about the Toyota Camry")
	fmt.Println("   Type 'quit' or 'exit' to end the demo")
	fmt.Println("   Type 'examples' to see sample queries")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("❓ Your question: ")
		if !scanner.Scan() {
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}

		if query == "quit" || query == "exit" {
			fmt.Println("\n👋 Thanks for trying the Knowledge Engine demo!")
			break
		}

		if query == "examples" {
			showExamples()
			continue
		}

		// Execute query
		queryStart := time.Now()
		results := executeDemoQuery(db, tenantID, campaignID, query)
		queryTime := time.Since(queryStart)

		fmt.Printf("\n📊 Results (found in %v):\n", queryTime)
		fmt.Println("─────────────────────────────────────────")

		if len(results.Specs) == 0 && len(results.Chunks) == 0 {
			fmt.Println("   No specific results found. Try rephrasing your question.")
		} else {
			if len(results.Specs) > 0 {
				fmt.Println("   📋 Specifications:")
				for i, spec := range results.Specs {
					if i >= 5 {
						fmt.Printf("      ... and %d more\n", len(results.Specs)-5)
						break
					}
					fmt.Printf("      • %s > %s: %s %s\n", spec.Category, spec.Name, spec.Value, spec.Unit)
				}
			}

			if len(results.Chunks) > 0 {
				fmt.Println("   💡 Related Information:")
				for i, chunk := range results.Chunks {
					if i >= 3 {
						fmt.Printf("      ... and %d more\n", len(results.Chunks)-3)
						break
					}
					text := chunk.Text
					if len(text) > 100 {
						text = text[:100] + "..."
					}
					fmt.Printf("      • [%s] %s\n", chunk.Type, text)
				}
			}
		}
		fmt.Println()
	}

	return nil
}

func showExamples() {
	fmt.Println("\n📝 Example queries you can try:")
	fmt.Println("   • What is the fuel efficiency?")
	fmt.Println("   • How much horsepower does it have?")
	fmt.Println("   • Tell me about the safety features")
	fmt.Println("   • What is the seating capacity?")
	fmt.Println("   • Does it have Apple CarPlay?")
	fmt.Println("   • What are the dimensions?")
	fmt.Println("   • Tell me about the hybrid system")
	fmt.Println("   • What is the engine displacement?")
	fmt.Println("   • How many airbags?")
	fmt.Println("   • What about the infotainment system?")
	fmt.Println()
}

func findDefaultBrochure() string {
	paths := []string{
		"e-brochure-camry-hybrid-specs.md",
		"../../e-brochure-camry-hybrid-specs.md",
		"../../../../e-brochure-camry-hybrid-specs.md",
		"libs/knowledge-engine/testdata/camry-sample.md",
		"testdata/camry-sample.md",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func runDemoMigrations(db *sql.DB) error {
	migrations := `
	CREATE TABLE IF NOT EXISTS tenants (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaign_variants (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		status TEXT DEFAULT 'published',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS spec_categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE
	);

	CREATE TABLE IF NOT EXISTS spec_items (
		id TEXT PRIMARY KEY,
		category_id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		unit TEXT
	);

	CREATE TABLE IF NOT EXISTS spec_values (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		campaign_variant_id TEXT NOT NULL,
		spec_item_id TEXT NOT NULL,
		value_text TEXT,
		unit TEXT,
		confidence REAL DEFAULT 1.0
	);

	CREATE TABLE IF NOT EXISTS knowledge_chunks (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		campaign_variant_id TEXT,
		chunk_type TEXT NOT NULL,
		text TEXT NOT NULL,
		embedding_vector BLOB
	);

	CREATE INDEX IF NOT EXISTS idx_spec_values_search ON spec_values(tenant_id, campaign_variant_id);
	CREATE INDEX IF NOT EXISTS idx_chunks_search ON knowledge_chunks(tenant_id, campaign_variant_id);
	`
	_, err := db.Exec(migrations)
	return err
}

func createDemoTenant(db *sql.DB, id uuid.UUID) error {
	_, err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", id.String(), "Toyota Demo")
	return err
}

func createDemoProduct(db *sql.DB, tenantID, productID uuid.UUID) error {
	_, err := db.Exec("INSERT INTO products (id, tenant_id, name) VALUES (?, ?, ?)",
		productID.String(), tenantID.String(), "Camry Hybrid 2025")
	return err
}

func createDemoCampaign(db *sql.DB, tenantID, productID, campaignID uuid.UUID) error {
	_, err := db.Exec("INSERT INTO campaign_variants (id, product_id, tenant_id) VALUES (?, ?, ?)",
		campaignID.String(), productID.String(), tenantID.String())
	return err
}

func storeDemoData(ctx context.Context, db *sql.DB, tenantID, productID, campaignID uuid.UUID,
	parsed *ingest.ParsedBrochure, embClient embedding.Embedder) (int, int, error) {

	specCount := 0
	chunkCount := 0
	categoryCache := make(map[string]uuid.UUID)

	// Store specifications
	for _, spec := range parsed.SpecValues {
		categoryID, ok := categoryCache[spec.Category]
		if !ok {
			categoryID = uuid.New()
			db.Exec("INSERT OR IGNORE INTO spec_categories (id, name) VALUES (?, ?)",
				categoryID.String(), spec.Category)
			categoryCache[spec.Category] = categoryID
		}

		specItemID := uuid.New()
		db.Exec("INSERT INTO spec_items (id, category_id, display_name, unit) VALUES (?, ?, ?, ?)",
			specItemID.String(), categoryID.String(), spec.Name, spec.Unit)

		specValueID := uuid.New()
		_, err := db.Exec(`INSERT INTO spec_values (id, tenant_id, product_id, campaign_variant_id, 
			spec_item_id, value_text, unit, confidence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			specValueID.String(), tenantID.String(), productID.String(), campaignID.String(),
			specItemID.String(), spec.Value, spec.Unit, spec.Confidence)
		if err == nil {
			specCount++
		}
	}

	// Store features
	for _, feature := range parsed.Features {
		chunkID := uuid.New()
		var embVector []byte

		if embClient != nil {
			emb, err := embClient.EmbedSingle(ctx, feature.Body)
			if err == nil && len(emb) > 0 {
				embVector, _ = json.Marshal(emb)
			}
		}

		_, err := db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, embedding_vector) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), tenantID.String(), productID.String(), campaignID.String(),
			"feature", feature.Body, embVector)
		if err == nil {
			chunkCount++
		}
	}

	// Store USPs
	for _, usp := range parsed.USPs {
		chunkID := uuid.New()
		var embVector []byte

		if embClient != nil {
			emb, err := embClient.EmbedSingle(ctx, usp.Body)
			if err == nil && len(emb) > 0 {
				embVector, _ = json.Marshal(emb)
			}
		}

		_, err := db.Exec(`INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, 
			chunk_type, text, embedding_vector) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			chunkID.String(), tenantID.String(), productID.String(), campaignID.String(),
			"usp", usp.Body, embVector)
		if err == nil {
			chunkCount++
		}
	}

	return specCount, chunkCount, nil
}

type demoQueryResult struct {
	Specs  []demoSpec
	Chunks []demoChunk
}

type demoSpec struct {
	Category string
	Name     string
	Value    string
	Unit     string
}

type demoChunk struct {
	Type string
	Text string
}

func executeDemoQuery(db *sql.DB, tenantID, campaignID uuid.UUID, query string) demoQueryResult {
	result := demoQueryResult{}
	keywords := extractDemoKeywords(query)

	// Search specs
	for _, keyword := range keywords {
		rows, err := db.Query(`
			SELECT sc.name, si.display_name, sv.value_text, COALESCE(sv.unit, '')
			FROM spec_values sv
			JOIN spec_items si ON sv.spec_item_id = si.id
			JOIN spec_categories sc ON si.category_id = sc.id
			WHERE sv.tenant_id = ? AND sv.campaign_variant_id = ?
			  AND (LOWER(si.display_name) LIKE ? OR LOWER(sc.name) LIKE ?)
			LIMIT 10
		`, tenantID.String(), campaignID.String(), "%"+keyword+"%", "%"+keyword+"%")
		if err != nil {
			continue
		}

		for rows.Next() {
			var spec demoSpec
			if err := rows.Scan(&spec.Category, &spec.Name, &spec.Value, &spec.Unit); err == nil {
				result.Specs = append(result.Specs, spec)
			}
		}
		rows.Close()
	}

	// Search chunks
	for _, keyword := range keywords {
		rows, err := db.Query(`
			SELECT chunk_type, text
			FROM knowledge_chunks
			WHERE tenant_id = ? AND campaign_variant_id = ?
			  AND LOWER(text) LIKE ?
			LIMIT 5
		`, tenantID.String(), campaignID.String(), "%"+keyword+"%")
		if err != nil {
			continue
		}

		for rows.Next() {
			var chunk demoChunk
			if err := rows.Scan(&chunk.Type, &chunk.Text); err == nil {
				result.Chunks = append(result.Chunks, chunk)
			}
		}
		rows.Close()
	}

	return result
}

func extractDemoKeywords(query string) []string {
	stopWords := map[string]bool{
		"what": true, "is": true, "the": true, "a": true, "an": true,
		"how": true, "many": true, "does": true, "it": true, "have": true,
		"tell": true, "me": true, "about": true, "of": true, "are": true,
		"why": true, "should": true, "i": true, "buy": true, "much": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string

	for _, word := range words {
		word = strings.Trim(word, "?.,!")
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

