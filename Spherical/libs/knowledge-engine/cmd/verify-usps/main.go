package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

func main() {
	ctx := context.Background()
	_ = observability.NewLogger(observability.LogConfig{
		Level:       "info",
		Format:      "console",
		ServiceName: "verify-usps",
	})

	// Find database file
	dbPath := filepath.Join(os.TempDir(), "knowledge_demo.db")
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	fmt.Printf("🔍 Checking database: %s\n\n", dbPath)

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Printf("❌ Database file not found: %s\n", dbPath)
		fmt.Printf("   Run the knowledge-demo first to create the database.\n")
		os.Exit(1)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("❌ Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. Check if USP chunks exist in database
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("1. CHECKING USP CHUNKS IN DATABASE")
	fmt.Println("=" + strings.Repeat("=", 70))

	var totalUSPs int
	err = db.QueryRow("SELECT COUNT(*) FROM knowledge_chunks WHERE chunk_type = 'usp'").Scan(&totalUSPs)
	if err != nil {
		fmt.Printf("❌ Error querying USP chunks: %v\n", err)
	} else {
		fmt.Printf("✓ Found %d USP chunks in database\n", totalUSPs)
	}

	// 2. Check embeddings for USP chunks
	fmt.Println("\n" + "=" + strings.Repeat("=", 70))
	fmt.Println("2. CHECKING EMBEDDINGS FOR USP CHUNKS")
	fmt.Println("=" + strings.Repeat("=", 70))

	var uspsWithEmbeddings int
	var uspsWithoutEmbeddings int
	rows, err := db.Query(`
		SELECT 
			id, 
			text, 
			embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0 as has_embedding,
			embedding_model
		FROM knowledge_chunks 
		WHERE chunk_type = 'usp'
		ORDER BY text
		LIMIT 20
	`)
	if err != nil {
		fmt.Printf("❌ Error querying USP chunks: %v\n", err)
	} else {
		defer rows.Close()
		
		fmt.Println("\nSample USP chunks:")
		count := 0
		for rows.Next() {
			var id, text, embeddingModel string
			var hasEmbedding bool
			if err := rows.Scan(&id, &text, &hasEmbedding, &embeddingModel); err != nil {
				continue
			}
			
			count++
			if hasEmbedding {
				uspsWithEmbeddings++
				fmt.Printf("\n[%d] ✓ Has embedding (%s)\n", count, embeddingModel)
			} else {
				uspsWithoutEmbeddings++
				fmt.Printf("\n[%d] ❌ Missing embedding\n", count)
			}
			
			// Show first 100 chars of text
			preview := text
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			fmt.Printf("    Text: %s\n", preview)
		}
		
		if count == 0 {
			fmt.Println("   ⚠ No USP chunks found in database")
		}
	}

	// Get total counts (SQLite-compatible syntax)
	err = db.QueryRow(`
		SELECT 
			SUM(CASE WHEN embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN embedding_vector IS NULL OR LENGTH(embedding_vector) = 0 THEN 1 ELSE 0 END)
		FROM knowledge_chunks 
		WHERE chunk_type = 'usp'
	`).Scan(&uspsWithEmbeddings, &uspsWithoutEmbeddings)
	if err != nil {
		fmt.Printf("⚠ Could not get embedding stats: %v\n", err)
	} else {
		fmt.Printf("\n📊 Embedding Statistics:\n")
		fmt.Printf("   With embeddings: %d\n", uspsWithEmbeddings)
		fmt.Printf("   Without embeddings: %d\n", uspsWithoutEmbeddings)
		fmt.Printf("   Total: %d\n", uspsWithEmbeddings+uspsWithoutEmbeddings)
	}

	// 3. Test vector search filtering
	fmt.Println("\n" + "=" + strings.Repeat("=", 70))
	fmt.Println("3. TESTING VECTOR SEARCH FILTERING")
	fmt.Println("=" + strings.Repeat("=", 70))

	// Get tenant/product/campaign IDs from database
	var tenantID, productID, campaignID string
	err = db.QueryRow("SELECT id FROM tenants LIMIT 1").Scan(&tenantID)
	if err != nil {
		fmt.Printf("⚠ No tenant found, skipping vector search test\n")
	} else {
		db.QueryRow("SELECT id FROM products LIMIT 1").Scan(&productID)
		db.QueryRow("SELECT id FROM campaign_variants LIMIT 1").Scan(&campaignID)

		// Load vector adapter from database (load all chunks into memory)
		// First, we need to detect dimension from first embedding
		dimension := 768 // default
		var sampleEmb []byte
		db.QueryRow("SELECT embedding_vector FROM knowledge_chunks WHERE chunk_type = 'usp' AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0 LIMIT 1").Scan(&sampleEmb)
		if len(sampleEmb) > 0 {
			var sampleVec []float32
			if json.Unmarshal(sampleEmb, &sampleVec) == nil && len(sampleVec) > 0 {
				dimension = len(sampleVec)
			}
		}
		
		faissAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: dimension})
		if err != nil {
			fmt.Printf("⚠ Could not create FAISS adapter: %v\n", err)
			fmt.Printf("   Vector search filtering test skipped\n")
		} else {
			// Load vectors from database into FAISS adapter
			rows, err := db.Query("SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type, embedding_vector, metadata FROM knowledge_chunks WHERE chunk_type = 'usp'")
			if err != nil {
				fmt.Printf("⚠ Could not load USP chunks: %v\n", err)
				fmt.Printf("   Vector search filtering test skipped\n")
			} else {
			defer rows.Close()
			
			var loaded int
			for rows.Next() {
				var id, tenantID, productID, campaignID, chunkType string
				var embeddingVector []byte
				var metadataJSON sql.NullString
				
				if err := rows.Scan(&id, &tenantID, &productID, &campaignID, &chunkType, &embeddingVector, &metadataJSON); err != nil {
					continue
				}
				
				// Parse embedding vector
				if len(embeddingVector) > 0 {
					var emb []float32
					if err := json.Unmarshal(embeddingVector, &emb); err == nil {
						idUUID, _ := uuid.Parse(id)
						tenantUUID, _ := uuid.Parse(tenantID)
						productUUID, _ := uuid.Parse(productID)
						campaignUUID, _ := uuid.Parse(campaignID)
						
						entry := retrieval.VectorEntry{
							ID:                idUUID,
							TenantID:          tenantUUID,
							ProductID:         productUUID,
							CampaignVariantID: &campaignUUID,
							ChunkType:         chunkType,
							Vector:            emb,
							Metadata:          make(map[string]interface{}),
						}
						
						if metadataJSON.Valid {
							json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
						}
						
						faissAdapter.Insert(ctx, []retrieval.VectorEntry{entry})
						loaded++
					}
				}
			}
			
			fmt.Printf("✓ Loaded %d USP chunks into FAISS adapter\n", loaded)
			
			if loaded == 0 {
				fmt.Printf("   ⚠ No USP chunks with embeddings found in database\n")
				fmt.Printf("   Vector search filtering test skipped\n")
			} else {
				// Get embedder (if available)
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
					}
				}

				if embedder == nil {
					fmt.Printf("⚠ No embedder available (OPENROUTER_API_KEY not set)\n")
					fmt.Printf("   Vector search filtering test skipped\n")
				} else {
					// Test query
					testQuery := "What are the USPs of this car?"
					fmt.Printf("\nTesting query: \"%s\"\n", testQuery)
					
					// Generate embedding
					queryVector, err := embedder.EmbedSingle(ctx, testQuery)
					if err != nil {
						fmt.Printf("❌ Error generating embedding: %v\n", err)
					} else {
						fmt.Printf("✓ Generated query embedding (dimension: %d)\n", len(queryVector))

						// Test with USP filter
						tenantUUID, _ := uuid.Parse(tenantID)
						productUUID, _ := uuid.Parse(productID)
						campaignUUID, _ := uuid.Parse(campaignID)
						
						filters := retrieval.VectorFilters{
							TenantID:          &tenantUUID,
							ProductIDs:        []uuid.UUID{productUUID},
							CampaignVariantID: &campaignUUID,
							ChunkTypes:        []string{"usp"},
						}

						results, err := faissAdapter.Search(ctx, queryVector, 5, filters)
						if err != nil {
							fmt.Printf("❌ Vector search error: %v\n", err)
						} else {
							fmt.Printf("✓ Vector search returned %d USP chunks\n", len(results))
							
							if len(results) > 0 {
								fmt.Printf("\nTop results:\n")
								for i, result := range results {
									fmt.Printf("  [%d] Score: %.4f, ID: %s\n", i+1, result.Score, result.ID)
									
									// Get text from database
									var text string
									db.QueryRow("SELECT text FROM knowledge_chunks WHERE id = ?", result.ID.String()).Scan(&text)
									preview := text
									if len(preview) > 80 {
										preview = preview[:80] + "..."
									}
									fmt.Printf("       Text: %s\n", preview)
								}
							} else {
								fmt.Printf("   ⚠ No USP chunks found - check if embeddings exist in FAISS index\n")
							}
						}
					}
				}
			}
			}
		}
	}

	// Summary
	fmt.Println("\n" + "=" + strings.Repeat("=", 70))
	fmt.Println("SUMMARY")
	fmt.Println("=" + strings.Repeat("=", 70))
	
	if totalUSPs == 0 {
		fmt.Printf("❌ No USP chunks found in database\n")
		fmt.Printf("   Run the ingestion process to import USP data.\n")
	} else if uspsWithoutEmbeddings > 0 {
		fmt.Printf("⚠ Found %d USP chunks but %d are missing embeddings\n", totalUSPs, uspsWithoutEmbeddings)
		fmt.Printf("   Re-run the ingestion to generate embeddings.\n")
	} else {
		fmt.Printf("✓ All checks passed!\n")
		fmt.Printf("   - %d USP chunks found in database\n", totalUSPs)
		fmt.Printf("   - All have embeddings\n")
	}
}

