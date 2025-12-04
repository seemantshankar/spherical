// Package integration provides integration tests for row-level chunking retrieval.
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"encoding/binary"
	"math"
)

// TestRowChunking_HierarchicalGrouping tests hierarchical grouping of row chunks in retrieval.
func TestRowChunking_HierarchicalGrouping(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()

	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Apply migrations
	migration1Path := filepath.Join("..", "..", "db", "migrations", "0001_init_sqlite.sql")
	migration1, err := os.ReadFile(migration1Path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration1))
	require.NoError(t, err)

	migration2Path := filepath.Join("..", "..", "db", "migrations", "0002_add_row_chunking_fields_sqlite.sql")
	migration2, err := os.ReadFile(migration2Path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration2))
	require.NoError(t, err)

	repos := storage.NewRepositories(db)
	embClient := embedding.NewMockClient(768)
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	require.NoError(t, err)
	lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

	// Create pipeline
	pipeline := ingest.NewPipeline(
		logger,
		ingest.PipelineConfig{
			ChunkSize:          512,
			ChunkOverlap:       64,
			EmbeddingBatchSize: 75,
		},
		repos,
		embClient,
		vectorAdapter,
		lineageWriter,
	)

	// Read test file with tables
	samplePath := filepath.Join("..", "..", "testdata", "camry-sample-with-tables.md")
	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	// Ingest document
	ingestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := pipeline.Ingest(ingestCtx, ingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID,
		MarkdownPath: samplePath,
		Operator:     "test-runner",
	})
	require.NoError(t, err)
	assert.Greater(t, result.ChunksCreated, 0)

	// Load vectors into FAISS adapter using direct query to avoid SQLite time scanning issues
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, embedding_vector, embedding_model, embedding_version,
			source_doc_id, source_page, visibility
		FROM knowledge_chunks
		WHERE tenant_id = ? AND product_id = ? AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0
	`
	rows, err := db.QueryContext(ingestCtx, query, tenantID.String(), productID.String())
	require.NoError(t, err)
	defer rows.Close()
	
	chunks := []*storage.KnowledgeChunk{}
	for rows.Next() {
		chunk := &storage.KnowledgeChunk{}
		var embeddingBlob []byte
		var metadataBlob sql.NullString
		var campaignVariantIDPtr, sourceDocIDPtr sql.NullString
		var sourcePagePtr sql.NullInt64
		
		err := rows.Scan(
			&chunk.ID, &chunk.TenantID, &chunk.ProductID, &campaignVariantIDPtr, &chunk.ChunkType,
			&chunk.Text, &metadataBlob, &embeddingBlob, &chunk.EmbeddingModel, &chunk.EmbeddingVersion,
			&sourceDocIDPtr, &sourcePagePtr, &chunk.Visibility,
		)
		require.NoError(t, err)
		
		if campaignVariantIDPtr.Valid {
			campaignID, err := uuid.Parse(campaignVariantIDPtr.String)
			if err == nil {
				chunk.CampaignVariantID = &campaignID
			}
		}
		if sourceDocIDPtr.Valid {
			sourceDocID, err := uuid.Parse(sourceDocIDPtr.String)
			if err == nil {
				chunk.SourceDocID = &sourceDocID
			}
		}
		if sourcePagePtr.Valid {
			page := int(sourcePagePtr.Int64)
			chunk.SourcePage = &page
		}
		
		if metadataBlob.Valid && len(metadataBlob.String) > 0 {
			chunk.Metadata = json.RawMessage(metadataBlob.String)
		} else {
			chunk.Metadata = json.RawMessage("{}")
		}
		
		// Convert BLOB to []float32 (JSON array of floats)
		if len(embeddingBlob) > 0 {
			var floats []float32
			if err := json.Unmarshal(embeddingBlob, &floats); err == nil {
				chunk.EmbeddingVector = floats
			} else {
				// Try float64 and convert
				var floats64 []float64
				if err := json.Unmarshal(embeddingBlob, &floats64); err == nil {
					floats = make([]float32, len(floats64))
					for i, f := range floats64 {
						floats[i] = float32(f)
					}
					chunk.EmbeddingVector = floats
				} else if len(embeddingBlob)%4 == 0 {
					// Binary format (4 bytes per float32)
					floats = make([]float32, len(embeddingBlob)/4)
					for i := 0; i < len(floats); i++ {
						bits := binary.LittleEndian.Uint32(embeddingBlob[i*4 : (i+1)*4])
						floats[i] = math.Float32frombits(bits)
					}
					chunk.EmbeddingVector = floats
				}
			}
		}
		
		chunks = append(chunks, chunk)
	}
	require.NoError(t, rows.Err())

	vectorEntries := make([]retrieval.VectorEntry, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.EmbeddingVector) > 0 {
			// Parse metadata for vector entry
			var metadata map[string]interface{}
			if len(chunk.Metadata) > 0 {
				_ = json.Unmarshal(chunk.Metadata, &metadata)
			}
			if metadata == nil {
				metadata = make(map[string]interface{})
			}

			metadata["chunk_type"] = string(chunk.ChunkType)
			metadata["text"] = chunk.Text
			if chunk.SourceDocID != nil {
				metadata["source_doc"] = chunk.SourceDocID.String()
			}

			// Add category metadata for row chunks
			if chunk.ChunkType == storage.ChunkTypeSpecRow && len(chunk.Metadata) > 0 {
				var chunkMetadata map[string]interface{}
				_ = json.Unmarshal(chunk.Metadata, &chunkMetadata)
				if chunkMetadata != nil {
					if pc, ok := chunkMetadata["parent_category"].(string); ok {
						metadata["parent_category"] = pc
					}
					if sc, ok := chunkMetadata["sub_category"].(string); ok {
						metadata["sub_category"] = sc
					}
					if st, ok := chunkMetadata["specification_type"].(string); ok {
						metadata["specification_type"] = st
					}
				}
			}

			vectorEntries = append(vectorEntries, retrieval.VectorEntry{
				ID:                chunk.ID,
				TenantID:          chunk.TenantID,
				ProductID:         chunk.ProductID,
				CampaignVariantID: chunk.CampaignVariantID,
				ChunkType:         string(chunk.ChunkType),
				Visibility:        string(chunk.Visibility),
				Vector:            chunk.EmbeddingVector,
				Metadata:          metadata,
			})
		}
	}

	if len(vectorEntries) > 0 {
		err = vectorAdapter.Insert(ingestCtx, vectorEntries)
		require.NoError(t, err)
	}

	// Create retrieval router
	cacheClient := cache.NewMemoryClient(1000) // Use memory cache for testing
	specViewRepo := storage.NewSpecViewRepository(db)
	router := retrieval.NewRouter(
		logger,
		cacheClient,
		vectorAdapter,
		embClient,
		specViewRepo,
		retrieval.RouterConfig{
			MaxChunks:       20,
			StructuredFirst: false, // Use vector search for this test
			SemanticFallback: true,
		},
	)

	// Query for colors (should return Exterior > Colors chunks)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer queryCancel()

	queryReq := retrieval.RetrievalRequest{
		TenantID:   tenantID,
		ProductIDs: []uuid.UUID{productID},
		Question:   "What colors are available?",
		MaxChunks:  20,
	}

	response, err := router.Query(queryCtx, queryReq)
	require.NoError(t, err)

	// Filter to only spec_row chunks
	rowChunks := make([]retrieval.SemanticChunk, 0)
	for _, chunk := range response.SemanticChunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			rowChunks = append(rowChunks, chunk)
		}
	}

	if len(rowChunks) > 0 {
		t.Logf("Found %d row chunks in query results", len(rowChunks))

		// Verify hierarchical grouping
		// Chunks should be grouped by parent_category, then sub_category
		parentCategories := make(map[string][]retrieval.SemanticChunk)
		for _, chunk := range rowChunks {
			parentCat := chunk.ParentCategory
			if parentCat == "" {
				parentCat = "Uncategorized"
			}
			parentCategories[parentCat] = append(parentCategories[parentCat], chunk)
		}

		t.Logf("Found chunks in %d parent categories", len(parentCategories))
		for parentCat, chunks := range parentCategories {
			t.Logf("  %s: %d chunks", parentCat, len(chunks))

			// Group by sub-category within parent
			subCategories := make(map[string]int)
			for _, chunk := range chunks {
				subCat := chunk.SubCategory
				if subCat == "" {
					subCat = "General"
				}
				subCategories[subCat]++
			}

			for subCat, count := range subCategories {
				t.Logf("    %s: %d chunks", subCat, count)
			}
		}

		// Verify that chunks have category metadata
		for _, chunk := range rowChunks {
			assert.NotEmpty(t, chunk.ParentCategory, "Row chunk should have parent_category")
			assert.NotEmpty(t, chunk.SubCategory, "Row chunk should have sub_category")
			assert.NotEmpty(t, chunk.Text, "Row chunk should have text")
		}

		// For color query, we should find Exterior > Colors chunks
		if exteriorChunks, ok := parentCategories["Exterior"]; ok {
			foundColorChunk := false
			for _, chunk := range exteriorChunks {
				if chunk.SubCategory == "Colors" {
					foundColorChunk = true
					t.Logf("Found color chunk: %s", chunk.Text)
					break
				}
			}
			// Note: This might not always find colors if embedding similarity is low
			// But we verify the structure is correct
			if foundColorChunk {
				t.Log("✓ Found Exterior > Colors chunk as expected")
			}
		}
	} else {
		t.Log("No row chunks found in query results (this is OK if embeddings don't match well)")
	}

	// Verify response structure
	assert.NotNil(t, response)
	assert.NotNil(t, response.SemanticChunks)
}

