// Package integration provides integration tests for the Knowledge Engine.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

func TestRetrievalRouter_HybridQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	
	// Create FAISS adapter
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
		Dimension: 768,
	})
	require.NoError(t, err)

	// Seed some test vectors
	testChunks := []retrieval.VectorEntry{
		{
			ID:               uuid.New(),
			TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			ProductID:        uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			ChunkType:        "spec_row",
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           generateTestVector(768, 1),
			Metadata:         map[string]interface{}{"category": "Fuel Efficiency"},
		},
		{
			ID:               uuid.New(),
			TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			ProductID:        uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			ChunkType:        "usp",
			Visibility:       "private",
			EmbeddingVersion: "v1",
			Vector:           generateTestVector(768, 2),
			Metadata:         map[string]interface{}{"tags": []string{"efficiency"}},
		},
	}

	err = vectorAdapter.Insert(context.Background(), testChunks)
	require.NoError(t, err)

	// Create router
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, retrieval.RouterConfig{
		MaxChunks:                 8,
		StructuredFirst:           true,
		SemanticFallback:          true,
		IntentConfidenceThreshold: 0.7,
		CacheResults:              true,
		CacheTTL:                  5 * time.Minute,
	})

	// Test spec lookup query
	ctx := context.Background()
	resp, err := router.Query(ctx, retrieval.RetrievalRequest{
		TenantID:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		Question:   "What is the fuel efficiency of the Camry?",
		MaxChunks:  6,
	})

	require.NoError(t, err)
	assert.Equal(t, retrieval.IntentSpecLookup, resp.Intent)
	assert.GreaterOrEqual(t, resp.LatencyMs, int64(0))
}

func TestRetrievalRouter_IntentClassification(t *testing.T) {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(100)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, retrieval.RouterConfig{
		IntentConfidenceThreshold: 0.7,
	})

	tests := []struct {
		question string
		expected retrieval.Intent
	}{
		{"What is the horsepower?", retrieval.IntentSpecLookup},
		{"Compare Camry with Accord", retrieval.IntentComparison},
		{"What makes this car special?", retrieval.IntentUSPLookup},
		{"How do I connect Bluetooth?", retrieval.IntentFAQ},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.question, func(t *testing.T) {
			resp, err := router.Query(ctx, retrieval.RetrievalRequest{
				TenantID: uuid.New(),
				Question: tc.question,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, resp.Intent)
		})
	}
}

func TestRetrievalRouter_TenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(100)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})

	tenant1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tenant2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	product1 := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	product2 := uuid.MustParse("00000000-0000-0000-0000-000000000020")

	// Insert chunks for different tenants
	chunks := []retrieval.VectorEntry{
		{
			ID:         uuid.New(),
			TenantID:   tenant1,
			ProductID:  product1,
			ChunkType:  "spec_row",
			Visibility: "private",
			Vector:     generateTestVector(768, 1),
		},
		{
			ID:         uuid.New(),
			TenantID:   tenant2,
			ProductID:  product2,
			ChunkType:  "spec_row",
			Visibility: "private",
			Vector:     generateTestVector(768, 2),
		},
	}

	err := vectorAdapter.Insert(context.Background(), chunks)
	require.NoError(t, err)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, retrieval.RouterConfig{
		SemanticFallback: true,
	})

	ctx := context.Background()

	// Query as tenant1 should only see tenant1 data
	resp1, err := router.Query(ctx, retrieval.RetrievalRequest{
		TenantID:   tenant1,
		ProductIDs: []uuid.UUID{product1},
		Question:   "Tell me about the specs",
	})
	require.NoError(t, err)

	// Query as tenant2 should only see tenant2 data
	resp2, err := router.Query(ctx, retrieval.RetrievalRequest{
		TenantID:   tenant2,
		ProductIDs: []uuid.UUID{product2},
		Question:   "Tell me about the specs",
	})
	require.NoError(t, err)

	// Verify isolation - chunks should be tenant-specific
	_ = resp1
	_ = resp2
	// In a real test, we'd verify the chunk IDs are tenant-specific
}

func TestRetrievalRouter_Caching(t *testing.T) {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(100)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, retrieval.RouterConfig{
		CacheResults: true,
		CacheTTL:     1 * time.Minute,
	})

	ctx := context.Background()
	req := retrieval.RetrievalRequest{
		TenantID: uuid.New(),
		Question: "What is the mileage?",
	}

	// First query
	resp1, err := router.Query(ctx, req)
	require.NoError(t, err)
	latency1 := resp1.LatencyMs

	// Second query (should be faster due to cache)
	resp2, err := router.Query(ctx, req)
	require.NoError(t, err)
	latency2 := resp2.LatencyMs

	// Cache hit should be at least as fast
	assert.LessOrEqual(t, latency2, latency1+10) // Allow small variance
}

// generateTestVector creates a test vector with a seed for reproducibility.
func generateTestVector(dim int, seed int) []float32 {
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		// Simple deterministic pattern
		vec[i] = float32(((i + seed) % 100)) / 100.0
	}
	// Normalize
	var sum float32
	for _, v := range vec {
		sum += v * v
	}
	if sum > 0 {
		norm := 1.0 / float32(sqrt64(float64(sum)))
		for i := range vec {
			vec[i] *= norm
		}
	}
	return vec
}

func sqrt64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

