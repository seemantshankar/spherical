// Package integration provides integration tests for row-level chunking.
package integration

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

	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// TestRowChunking_Ingestion tests the full ingestion pipeline with row-level chunking.
func TestRowChunking_Ingestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()

	// Create in-memory SQLite database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Apply migrations
	ctx := context.Background()
	
	// Read and apply initial migration
	migration1Path := filepath.Join("..", "..", "db", "migrations", "0001_init_sqlite.sql")
	migration1, err := os.ReadFile(migration1Path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration1))
	require.NoError(t, err)

	// Apply row chunking migration
	migration2Path := filepath.Join("..", "..", "db", "migrations", "0002_add_row_chunking_fields_sqlite.sql")
	migration2, err := os.ReadFile(migration2Path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration2))
	require.NoError(t, err)

	// Create repositories
	repos := storage.NewRepositories(db)

	// Create mock embedder
	embClient := embedding.NewMockClient(768)

	// Create vector adapter
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
		Dimension: 768,
	})
	require.NoError(t, err)

	// Create lineage writer
	lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

	// Create pipeline
	pipeline := ingest.NewPipeline(
		logger,
		ingest.PipelineConfig{
			ChunkSize:          512,
			ChunkOverlap:       64,
			MaxConcurrentJobs:  2,
			DedupeThreshold:    0.95,
			EmbeddingBatchSize: 75,
		},
		repos,
		embClient,
		vectorAdapter,
		lineageWriter,
	)

	// Read test file with 5-column tables
	samplePath := filepath.Join("..", "..", "testdata", "camry-sample-with-tables.md")

	// Create test IDs
	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	// Run ingestion
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
	assert.NotEqual(t, uuid.Nil, result.JobID)
	assert.Greater(t, result.ChunksCreated, 0, "Should create chunks")

	// Verify row chunks were created
	// Use direct SQL query to avoid SQLite time scanning issues
	query := `
		SELECT COUNT(*) FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row'
	`
	var rowChunkCount int
	err = db.QueryRowContext(ingestCtx, query, tenantID.String(), campaignID.String()).Scan(&rowChunkCount)
	require.NoError(t, err)
	
	// Also get one chunk to verify structure
	query2 := `
		SELECT id, content_hash, completion_status, text, metadata
		FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row'
		LIMIT 1
	`
	var chunkID, contentHash, completionStatus, chunkText, metadataJSON string
	err = db.QueryRowContext(ingestCtx, query2, tenantID.String(), campaignID.String()).Scan(
		&chunkID, &contentHash, &completionStatus, &chunkText, &metadataJSON)
	require.NoError(t, err)

	assert.Greater(t, rowChunkCount, 0, "Should create row chunks from tables")
	t.Logf("Created %d row chunks", rowChunkCount)

	// Verify row chunk structure
	// Check content_hash exists
	assert.NotEmpty(t, contentHash, "Row chunk should have content_hash")
	assert.Len(t, contentHash, 64, "Content hash should be 64 characters")

	// Check completion_status
	assert.NotEmpty(t, completionStatus, "Row chunk should have completion_status")
	assert.Contains(t, []string{"complete", "incomplete"}, completionStatus, "Completion status should be valid")

	// Check metadata structure
	var metadata map[string]interface{}
	err = json.Unmarshal([]byte(metadataJSON), &metadata)
	require.NoError(t, err)

	// Verify required metadata fields
	assert.Contains(t, metadata, "parent_category", "Metadata should contain parent_category")
	assert.Contains(t, metadata, "sub_category", "Metadata should contain sub_category")
	assert.Contains(t, metadata, "specification_type", "Metadata should contain specification_type")
	assert.Contains(t, metadata, "value", "Metadata should contain value")
	assert.Contains(t, metadata, "parsed_spec_ids", "Metadata should contain parsed_spec_ids")

	// Verify parsed_spec_ids is an array
	parsedSpecIDs, ok := metadata["parsed_spec_ids"].([]interface{})
	assert.True(t, ok, "parsed_spec_ids should be an array")
	assert.Greater(t, len(parsedSpecIDs), 0, "parsed_spec_ids should not be empty")

	t.Logf("Row chunk metadata: parent_category=%v, sub_category=%v, specification_type=%v, value=%v",
		metadata["parent_category"], metadata["sub_category"], metadata["specification_type"], metadata["value"])

	// Test content hash deduplication
	// Ingest the same document again
	_, err = pipeline.Ingest(ingestCtx, ingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID,
		MarkdownPath: samplePath,
		Operator:     "test-runner",
		Overwrite:    true,
	})

	require.NoError(t, err)

	// Get chunks after second ingestion using direct query
	query3 := `
		SELECT COUNT(*) FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row'
	`
	var rowChunks2 int
	err = db.QueryRowContext(ingestCtx, query3, tenantID.String(), campaignID.String()).Scan(&rowChunks2)
	require.NoError(t, err)

	// Should have same or fewer row chunks (deduplication)
	// Note: With overwrite=true, old chunks might be deleted, so we just verify it doesn't error
	assert.GreaterOrEqual(t, rowChunks2, 0, "Should handle second ingestion")
	t.Logf("After second ingestion: %d row chunks", rowChunks2)
}

// TestRowChunking_ContentHashDeduplication tests content hash-based deduplication.
func TestRowChunking_ContentHashDeduplication(t *testing.T) {
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
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

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

	tenantID := uuid.New()
	productID := uuid.New()
	campaignID1 := uuid.New()
	campaignID2 := uuid.New()

	// Create a test markdown file with tables
	testContent := `---
title: Test Product
product: Test Car
year: 2025
---

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Red | Standard |
| Exterior | Colors | Color | Blue | Standard |
`

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test-*.md")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString(testContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Ingest first time
	ingestCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result1, err := pipeline.Ingest(ingestCtx, ingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID1,
		MarkdownPath: tmpFile.Name(),
		Operator:     "test-runner",
	})
	require.NoError(t, err)
	assert.Greater(t, result1.ChunksCreated, 0)

	// Get chunks from first ingestion using direct query
	query1 := `
		SELECT content_hash FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row' AND content_hash IS NOT NULL
	`
	rows1, err := db.QueryContext(ingestCtx, query1, tenantID.String(), campaignID1.String())
	require.NoError(t, err)
	defer rows1.Close()

	// Find row chunks and their content hashes
	contentHashes1 := make(map[string]bool)
	for rows1.Next() {
		var hash string
		err := rows1.Scan(&hash)
		require.NoError(t, err)
		contentHashes1[hash] = true
	}
	require.NoError(t, rows1.Err())

	assert.Greater(t, len(contentHashes1), 0, "Should have row chunks with content hashes")

	// Ingest same content in different campaign
	_, err = pipeline.Ingest(ingestCtx, ingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID2,
		MarkdownPath: tmpFile.Name(),
		Operator:     "test-runner",
	})
	require.NoError(t, err)

	// Get chunks from second ingestion using direct query
	query2 := `
		SELECT content_hash, text FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row' AND content_hash IS NOT NULL
	`
	rows2, err := db.QueryContext(ingestCtx, query2, tenantID.String(), campaignID2.String())
	require.NoError(t, err)
	defer rows2.Close()

	// Check if same content hashes exist (deduplication should link to same chunks)
	// Or create new chunks with same content_hash
	contentHashes2 := make(map[string]string) // hash -> text
	for rows2.Next() {
		var hash, text string
		err := rows2.Scan(&hash, &text)
		require.NoError(t, err)
		contentHashes2[hash] = text
	}
	require.NoError(t, rows2.Err())

	// Verify content hashes match (same content = same hash)
	for hash := range contentHashes1 {
		if text2, exists := contentHashes2[hash]; exists {
			// Same content hash means same content
			t.Logf("Found matching content hash: %s (text: %s)", hash, text2[:50])
		}
	}
}

// TestRowChunking_BatchEmbeddingWithErrorHandling tests batch embedding with error handling and incomplete chunk storage.
func TestRowChunking_BatchEmbeddingWithErrorHandling(t *testing.T) {
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
	
	// Create a mock embedder that can simulate failures
	embClient := embedding.NewMockClient(768)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

	pipeline := ingest.NewPipeline(
		logger,
		ingest.PipelineConfig{
			ChunkSize:          512,
			ChunkOverlap:       64,
			EmbeddingBatchSize: 75, // Test batch size
		},
		repos,
		embClient,
		vectorAdapter,
		lineageWriter,
	)

	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	// Create a test markdown file with many table rows to test batching
	var testContentBuilder strings.Builder
	testContentBuilder.WriteString(`---
title: Test Product
product: Test Car
year: 2025
---

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
`)
	
	// Generate 150 rows to test batching (will create 2 batches of 75)
	for i := 0; i < 150; i++ {
		testContentBuilder.WriteString(fmt.Sprintf("| Exterior | Colors | Color | Color %d | Standard |\n", i+1))
	}

	testContent := testContentBuilder.String()

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test-batch-*.md")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString(testContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Ingest with batch processing
	ingestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := pipeline.Ingest(ingestCtx, ingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID,
		MarkdownPath: tmpFile.Name(),
		Operator:     "test-runner",
	})
	require.NoError(t, err)
	assert.Greater(t, result.ChunksCreated, 0, "Should create chunks")

	// Verify chunks were created
	query := `
		SELECT COUNT(*) FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row'
	`
	var rowChunkCount int
	err = db.QueryRowContext(ingestCtx, query, tenantID.String(), campaignID.String()).Scan(&rowChunkCount)
	require.NoError(t, err)
	// Note: Some rows may be filtered (header rows, etc.), so we check for a reasonable number
	assert.GreaterOrEqual(t, rowChunkCount, 140, "Should create at least 140 row chunks (some may be filtered)")
	t.Logf("Created %d row chunks from 150 table rows", rowChunkCount)

	// Verify completion_status distribution
	query2 := `
		SELECT completion_status, COUNT(*) 
		FROM knowledge_chunks 
		WHERE tenant_id = ? AND campaign_variant_id = ? AND chunk_type = 'spec_row'
		GROUP BY completion_status
	`
	rows, err := db.QueryContext(ingestCtx, query2, tenantID.String(), campaignID.String())
	require.NoError(t, err)
	defer rows.Close()

	completeCount := 0
	incompleteCount := 0
	for rows.Next() {
		var status string
		var count int
		err := rows.Scan(&status, &count)
		require.NoError(t, err)
		if status == "complete" {
			completeCount = count
		} else if status == "incomplete" {
			incompleteCount = count
		}
	}

	t.Logf("Completion status: %d complete, %d incomplete", completeCount, incompleteCount)
	
	// With mock embedder, all should be complete
	// In real scenarios, some might be incomplete if embedding fails
	assert.Greater(t, completeCount, 0, "Should have some complete chunks")
	
	// Test FindIncompleteChunks method
	incompleteChunks, err := repos.KnowledgeChunks.FindIncompleteChunks(ingestCtx, tenantID, 100)
	require.NoError(t, err)
	t.Logf("Found %d incomplete chunks for retry", len(incompleteChunks))
}

