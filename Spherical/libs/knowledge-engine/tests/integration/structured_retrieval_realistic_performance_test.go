// Package integration provides realistic performance tests with vector retrieval.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStructuredRetrieval_RealisticPerformance tests performance with actual vector data
func TestStructuredRetrieval_RealisticPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping realistic performance test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	
	// Create FAISS adapter
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	require.NoError(t, err)

	// Use embedder that simulates real API latency (50-100ms per batch)
	// Real OpenRouter embedding API calls take 50-200ms per batch
	baseEmbedder := embedding.NewMockClient(768)
	mockEmbedder := &SimulatedLatencyEmbedder{
		baseEmbedder:    baseEmbedder,
		latencyPerBatch: 75 * time.Millisecond, // Simulate 75ms API call latency
	}
	
	// Seed vector database with test chunks
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Insert test chunks that match our spec queries
	testChunks := []retrieval.VectorEntry{
		{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProductID:        productID,
			ChunkType:        string(storage.ChunkTypeSpecRow),
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           generateTestVector(768, 1), // Fuel Economy related
			Metadata: map[string]interface{}{
				"category":           "Fuel Efficiency",
				"parent_category":    "Fuel Efficiency",
				"specification_type": "Fuel Economy",
				"value":               "25.49",
				"unit":                "km/l",
			},
		},
		{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProductID:        productID,
			ChunkType:        string(storage.ChunkTypeSpecRow),
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           generateTestVector(768, 2), // Engine Torque related
			Metadata: map[string]interface{}{
				"category":           "Engine",
				"parent_category":    "Engine",
				"specification_type": "Engine Torque",
				"value":               "221",
				"unit":                "Nm",
			},
		},
		{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProductID:        productID,
			ChunkType:        string(storage.ChunkTypeSpecRow),
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           generateTestVector(768, 3), // Suspension related
			Metadata: map[string]interface{}{
				"category":           "Suspension",
				"parent_category":    "Suspension",
				"specification_type": "Suspension",
				"value":               "Independent",
			},
		},
	}

	err = vectorAdapter.Insert(ctx, testChunks)
	require.NoError(t, err)

	// Create router with populated vector database
	// Note: Cache is disabled to measure actual retrieval latency
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
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

	// Test with 10 specs (target: <500ms p95)
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

	// Run multiple times to get p95
	latencies := make([]time.Duration, 0, 20)
	for i := 0; i < 20; i++ {
		start := time.Now()
		resp, err := router.Query(ctx, req)
		duration := time.Since(start)
		require.NoError(t, err)
		latencies = append(latencies, duration)
		assert.Equal(t, len(specs), len(resp.SpecAvailability))
	}

	// Calculate statistics (sortLatencies is defined in structured_retrieval_comprehensive_test.go)
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

	t.Logf("Realistic Performance Test Results (with vector search + embedding API latency):")
	t.Logf("  Specs requested: %d", len(specs))
	t.Logf("  Min latency: %v", minLatency)
	t.Logf("  Max latency: %v", maxLatency)
	t.Logf("  Average latency: %v", avgLatency)
	t.Logf("  P95 latency: %v", p95Latency)
	t.Logf("  Target: <500ms")
	t.Logf("  Simulated embedding API latency: 75ms per batch")
	t.Logf("  Vector database contains %d test chunks", len(testChunks))
	
	// With vector search + embedding latency, expect higher latency
	// For 10 specs with 5 workers, we might have 2 batches = ~150ms embedding + processing
	// Should still be under 500ms target
	assert.Less(t, p95Latency, 500*time.Millisecond, "P95 latency should be <500ms for 10 specs with vector search")
	
	// Verify we're actually doing vector search and embedding generation
	// The latency should reflect embedding API calls (75ms per batch)
	if p95Latency < 50*time.Millisecond {
		t.Logf("WARNING: Latency too low - may not be exercising embedding generation")
	} else {
		t.Logf("✓ Latency includes embedding generation overhead")
	}
}

// TestStructuredRetrieval_WithEmbeddingLatency tests with simulated embedding API latency
func TestStructuredRetrieval_WithEmbeddingLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping embedding latency test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	
	// Create a mock embedder that simulates real API latency
	// Real OpenRouter API calls take 50-200ms per batch
	mockEmbedder := &SimulatedLatencyEmbedder{
		baseEmbedder: embedding.NewMockClient(768),
		latencyPerBatch: 50 * time.Millisecond, // Simulate 50ms API call
	}

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
		CacheResults:              false, // No cache for realistic test
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test with 10 specs - each needs embedding generation
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

	// Measure latency with embedding generation
	start := time.Now()
	resp, err := router.Query(ctx, req)
	duration := time.Since(start)
	
	require.NoError(t, err)
	assert.Equal(t, len(specs), len(resp.SpecAvailability))
	
	t.Logf("Latency with simulated embedding API calls (50ms per batch): %v", duration)
	t.Logf("  Specs requested: %d", len(specs))
	t.Logf("  Expected: ~50-200ms for embedding generation + processing")
	
	// With embedding latency, should be higher but still reasonable
	assert.Less(t, duration, 1*time.Second, "Should complete in <1s even with embedding latency")
}

// SimulatedLatencyEmbedder wraps an embedder to add simulated API latency
type SimulatedLatencyEmbedder struct {
	baseEmbedder    embedding.Embedder
	latencyPerBatch time.Duration
}

func (s *SimulatedLatencyEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// Simulate API call latency (real OpenRouter calls take 50-200ms)
	time.Sleep(s.latencyPerBatch)
	return s.baseEmbedder.Embed(ctx, texts)
}

func (s *SimulatedLatencyEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	// Simulate API call latency
	time.Sleep(s.latencyPerBatch)
	return s.baseEmbedder.EmbedSingle(ctx, text)
}

func (s *SimulatedLatencyEmbedder) Model() string {
	return s.baseEmbedder.Model()
}

func (s *SimulatedLatencyEmbedder) Dimension() int {
	return s.baseEmbedder.Dimension()
}

// TestStructuredRetrieval_WithPopulatedVectorDB tests with a populated vector database
func TestStructuredRetrieval_WithPopulatedVectorDB(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping populated vector DB test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	require.NoError(t, err)

	// Generate embeddings for test chunks
	mockEmbedder := embedding.NewMockClient(768)
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Create realistic test chunks with actual embeddings
	chunkTexts := []string{
		"Fuel Economy: 25.49 km/l (ARAI certified)",
		"Engine Torque: 221 Nm @ 2000-4000 RPM",
		"Suspension: Independent front and rear",
		"Ground Clearance: 165 mm",
		"Seating Capacity: 5 persons",
		"Boot Space: 470 liters",
		"Airbags: 6 airbags (driver, passenger, side, curtain)",
		"ABS: Anti-lock Braking System with EBD",
		"Parking Sensors: Rear parking sensors",
		"Rear Camera: High-resolution rear view camera",
	}

	// Generate embeddings for chunks
	chunkEmbeddings, err := mockEmbedder.Embed(ctx, chunkTexts)
	require.NoError(t, err)

	// Insert chunks into vector database
	testChunks := make([]retrieval.VectorEntry, len(chunkTexts))
	for i, text := range chunkTexts {
		testChunks[i] = retrieval.VectorEntry{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProductID:        productID,
			ChunkType:        string(storage.ChunkTypeSpecRow),
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           chunkEmbeddings[i],
			Metadata: map[string]interface{}{
				"text": text,
			},
		}
	}

	err = vectorAdapter.Insert(ctx, testChunks)
	require.NoError(t, err)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
		CacheResults:              false,
	})

	// Test with specs that should match our chunks
	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque", "Ground Clearance"},
		RequestMode:    retrieval.RequestModeStructured,
	}

	start := time.Now()
	resp, err := router.Query(ctx, req)
	duration := time.Since(start)
	
	require.NoError(t, err)
	
	// Verify we found some specs via vector search
	foundCount := 0
	for _, status := range resp.SpecAvailability {
		if status.Status == retrieval.AvailabilityStatusFound {
			foundCount++
			// Should have matched chunks from vector search
			assert.Greater(t, len(status.MatchedChunks), 0, "Found specs should have matched chunks from vector DB")
		}
	}
	
	t.Logf("Vector DB Performance Test:")
	t.Logf("  Latency: %v", duration)
	t.Logf("  Specs found via vector search: %d", foundCount)
	t.Logf("  Total chunks in DB: %d", len(testChunks))
	
	assert.Greater(t, foundCount, 0, "Should find at least one spec via vector search")
}

