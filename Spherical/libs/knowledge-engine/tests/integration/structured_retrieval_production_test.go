// Package integration provides production-ready tests with real database and OpenRouter API.
package integration

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStructuredRetrieval_ProductionRealData tests with real WagonR campaign data and OpenRouter API
func TestStructuredRetrieval_ProductionRealData(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping production test in short mode")
	}

	// Check for OpenRouter API key
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set - skipping production test")
	}

	// Connect to SQLite database
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "/tmp/knowledge-engine.db"
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skipf("Database not found at %s - skipping production test", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Find any product with embeddings
	ctx := context.Background()
	
	// Query for any product that has chunks with embeddings
	// First, get a product_id from chunks, then get the product details
	chunkQuery := `
		SELECT DISTINCT product_id, tenant_id
		FROM knowledge_chunks
		WHERE embedding_vector IS NOT NULL
		LIMIT 1
	`
	
	var productIDStr, tenantIDStr string
	err = db.QueryRowContext(ctx, chunkQuery).Scan(&productIDStr, &tenantIDStr)
	if err == sql.ErrNoRows {
		t.Skip("No chunks with embeddings found in database - skipping production test")
	}
	require.NoError(t, err)
	
	productID, err := uuid.Parse(productIDStr)
	require.NoError(t, err)
	tenantID, err := uuid.Parse(tenantIDStr)
	require.NoError(t, err)
	
	// Get product name
	productNameQuery := `SELECT name FROM products WHERE id = ? LIMIT 1`
	var productName string
	err = db.QueryRowContext(ctx, productNameQuery, productIDStr).Scan(&productName)
	if err == sql.ErrNoRows {
		productName = "Unknown Product"
	}

	t.Logf("Found product with embeddings: %s (ID: %s) - Tenant: %s", productName, productID, tenantID)

	// Find campaign for this product
	campaignQuery := `
		SELECT DISTINCT campaign_variant_id
		FROM knowledge_chunks
		WHERE tenant_id = ? AND product_id = ? AND campaign_variant_id IS NOT NULL
		LIMIT 1
	`
	
	var campaignIDPtr sql.NullString
	err = db.QueryRowContext(ctx, campaignQuery, tenantID.String(), productID.String()).Scan(&campaignIDPtr)
	if err == sql.ErrNoRows {
		t.Log("No campaign found, using product-level chunks")
	} else {
		require.NoError(t, err)
	}

	// Count chunks with embeddings
	chunkCountQuery := `
		SELECT COUNT(*)
		FROM knowledge_chunks
		WHERE tenant_id = ? AND product_id = ? 
		AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0
	`
	
	var chunkCount int
	err = db.QueryRowContext(ctx, chunkCountQuery, tenantID.String(), productID.String()).Scan(&chunkCount)
	require.NoError(t, err)

	if chunkCount == 0 {
		t.Skip("No chunks with embeddings found for WagonR - skipping production test")
	}

	t.Logf("Found %d chunks with embeddings", chunkCount)

	// Load vectors from database into FAISS
	// We'll detect embedding dimension from the first chunk
	loadVectorsQuery := `
		SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, embedding_vector, embedding_model, embedding_version,
			source_doc_id, source_page, visibility
		FROM knowledge_chunks
		WHERE tenant_id = ? AND product_id = ? 
		AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0
	`
	
	rows, err := db.QueryContext(ctx, loadVectorsQuery, tenantID.String(), productID.String())
	require.NoError(t, err)
	defer rows.Close()

	loadedChunks := 0
	vectorEntries := []retrieval.VectorEntry{}
	embeddingDimension := 0 // Will be detected from first chunk
	var vectorAdapter *retrieval.FAISSAdapter

	for rows.Next() {
		var chunkID, chunkTenantID, chunkProductID uuid.UUID
		var chunkType, chunkText, embeddingModel, embeddingVersion, visibility string
		var embeddingBlob []byte
		var metadataBlob sql.NullString
		var campaignVariantIDPtr, sourceDocIDPtr sql.NullString
		var sourcePagePtr sql.NullInt64

		err := rows.Scan(
			&chunkID, &chunkTenantID, &chunkProductID, &campaignVariantIDPtr, &chunkType,
			&chunkText, &metadataBlob, &embeddingBlob, &embeddingModel, &embeddingVersion,
			&sourceDocIDPtr, &sourcePagePtr, &visibility,
		)
		require.NoError(t, err)

		// Parse embedding vector (stored as BLOB, could be JSON or binary)
		if len(embeddingBlob) == 0 {
			continue
		}

		var vector []float32
		
		// Try JSON format first (common in SQLite)
		var floats []float32
		if err := json.Unmarshal(embeddingBlob, &floats); err == nil {
			vector = floats
		} else {
			// Try float64 and convert
			var floats64 []float64
			if err := json.Unmarshal(embeddingBlob, &floats64); err == nil {
				vector = make([]float32, len(floats64))
				for i, f := range floats64 {
					vector[i] = float32(f)
				}
			} else if len(embeddingBlob)%4 == 0 {
				// Binary format (4 bytes per float32)
				dim := len(embeddingBlob) / 4
				vector = make([]float32, dim)
				for i := 0; i < dim; i++ {
					bits := binary.LittleEndian.Uint32(embeddingBlob[i*4 : (i+1)*4])
					vector[i] = math.Float32frombits(bits)
				}
			} else {
				t.Logf("Warning: Could not parse embedding for chunk %s", chunkID)
				continue
			}
		}

		// Validate dimension - set on first chunk, then validate consistency
		if len(vector) == 0 {
			t.Logf("Warning: Empty embedding vector for chunk %s", chunkID)
			continue
		}
		if embeddingDimension == 0 {
			// First chunk - set the dimension and create FAISS adapter
			embeddingDimension = len(vector)
			t.Logf("Detected embedding dimension: %d from first chunk", embeddingDimension)
			vectorAdapter, err = retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: embeddingDimension})
			require.NoError(t, err)
		} else if len(vector) != embeddingDimension {
			t.Logf("Warning: Embedding dimension mismatch %d (expected %d) for chunk %s - skipping", len(vector), embeddingDimension, chunkID)
			continue
		}

		// Parse metadata
		metadata := make(map[string]interface{})
		if metadataBlob.Valid && metadataBlob.String != "" {
			// Try to parse as JSON
			_ = json.Unmarshal([]byte(metadataBlob.String), &metadata)
		}
		metadata["text"] = chunkText

		// Parse optional fields
		var campaignVariantID *uuid.UUID
		if campaignVariantIDPtr.Valid {
			if id, err := uuid.Parse(campaignVariantIDPtr.String); err == nil {
				campaignVariantID = &id
			}
		}

		vectorEntry := retrieval.VectorEntry{
			ID:               chunkID,
			TenantID:         chunkTenantID,
			ProductID:        chunkProductID,
			CampaignVariantID: campaignVariantID,
			ChunkType:        chunkType,
			Visibility:       visibility,
			EmbeddingVersion: embeddingVersion,
			Vector:           vector,
			Metadata:         metadata,
		}

		vectorEntries = append(vectorEntries, vectorEntry)
		loadedChunks++
	}

	require.NoError(t, rows.Err())
	require.Greater(t, loadedChunks, 0, "Should load at least one chunk")
	require.NotNil(t, vectorAdapter, "FAISS adapter should be created after detecting dimension")

	// Insert vectors into FAISS
	err = vectorAdapter.Insert(ctx, vectorEntries)
	require.NoError(t, err)

	t.Logf("Loaded %d vectors into FAISS adapter (dimension: %d)", loadedChunks, embeddingDimension)

	// Get embedding model from database
	var embeddingModel string
	modelQuery := `SELECT embedding_model FROM knowledge_chunks WHERE embedding_vector IS NOT NULL LIMIT 1`
	err = db.QueryRowContext(ctx, modelQuery).Scan(&embeddingModel)
	if err == sql.ErrNoRows || embeddingModel == "" {
		embeddingModel = "google/gemini-embedding-001" // Default fallback
	}
	
	// Create real OpenRouter embedding client with detected dimension
	embeddingClient, err := embedding.NewClient(embedding.Config{
		APIKey:    apiKey,
		Model:     embeddingModel, // Use the model from database
		BaseURL:   "https://openrouter.ai/api/v1",
		Dimension: embeddingDimension, // Use detected dimension
		Timeout:   30 * time.Second,
	})
	require.NoError(t, err)
	
	t.Logf("Using embedding model: %s (dimension: %d)", embeddingModel, embeddingDimension)

	// Create repositories for structured fact lookup
	specViewRepo := storage.NewSpecViewRepository(db)
	
	// Create router with real components
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, embeddingClient, specViewRepo, retrieval.RouterConfig{
		MaxChunks:                 8,
		StructuredFirst:           true,
		SemanticFallback:          true,
		IntentConfidenceThreshold: 0.7,
		KeywordConfidenceThreshold: 0.8,
		CacheResults:              false, // Disable cache for realistic testing
		CacheTTL:                  5 * time.Minute,
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	// Test with realistic spec queries
	specs := []string{
		"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension",
		"Seating Capacity", "Boot Space", "Airbags", "ABS", "Parking Sensors",
		"Rear Camera",
	}

	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: specs,
		RequestMode:    retrieval.RequestModeStructured,
	}

	// Run performance test
	latencies := make([]time.Duration, 0, 10)
	for i := 0; i < 10; i++ {
		start := time.Now()
		resp, err := router.Query(ctx, req)
		duration := time.Since(start)
		require.NoError(t, err)
		latencies = append(latencies, duration)
		assert.Equal(t, len(specs), len(resp.SpecAvailability))
	}

	// Calculate statistics
	sortLatencies(latencies)
	p95Index := int(float64(len(latencies)) * 0.95)
	p95Latency := latencies[p95Index]
	
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avgLatency := sum / time.Duration(len(latencies))
	minLatency := latencies[0]
	maxLatency := latencies[len(latencies)-1]

	// Count found specs
	foundCount := 0
	unavailableCount := 0
	partialCount := 0
	
	if len(latencies) > 0 {
		// Use last response for analysis
		lastResp, _ := router.Query(ctx, req)
		for _, status := range lastResp.SpecAvailability {
			switch status.Status {
			case retrieval.AvailabilityStatusFound:
				foundCount++
			case retrieval.AvailabilityStatusUnavailable:
				unavailableCount++
			case retrieval.AvailabilityStatusPartial:
				partialCount++
			}
		}
	}

	t.Logf("Production Performance Test Results (Real WagonR Data + OpenRouter API):")
	t.Logf("  Product: %s", productName)
	t.Logf("  Tenant ID: %s", tenantID)
	t.Logf("  Vectors loaded: %d", loadedChunks)
	t.Logf("  Specs requested: %d", len(specs))
	t.Logf("  Min latency: %v", minLatency)
	t.Logf("  Max latency: %v", maxLatency)
	t.Logf("  Average latency: %v", avgLatency)
	t.Logf("  P95 latency: %v", p95Latency)
	t.Logf("  Target: <500ms")
	t.Logf("  Results: Found=%d, Unavailable=%d, Partial=%d", foundCount, unavailableCount, partialCount)
	
	// Verify we're using real API (latency should be >50ms)
	if p95Latency < 50*time.Millisecond {
		t.Logf("WARNING: Latency too low - may not be using real OpenRouter API")
	} else {
		t.Logf("✓ Latency indicates real OpenRouter API calls")
	}

	// Should still be under target
	assert.Less(t, p95Latency, 500*time.Millisecond, "P95 latency should be <500ms")
}

