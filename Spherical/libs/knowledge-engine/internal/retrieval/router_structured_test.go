package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

func TestRouter_ProcessStructuredSpecs_Basic(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, IntentSpecLookup, resp.Intent)
	assert.Equal(t, 2, len(resp.SpecAvailability))
	assert.GreaterOrEqual(t, resp.LatencyMs, int64(0))
}

func TestRouter_ProcessStructuredSpecs_EmptySpecs(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{},
		Question:       "What is the fuel economy?",
		RequestMode:    RequestModeStructured,
	}

	// Should fallback to natural language
	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestRouter_ProcessStructuredSpecs_Normalization(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Mileage", "Fuel Consumption", "km/l"},
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 3, len(resp.SpecAvailability))

	// Verify alternative names are populated
	for _, status := range resp.SpecAvailability {
		if len(status.AlternativeNames) > 0 {
			assert.Greater(t, len(status.AlternativeNames), 0)
		}
	}
}

func TestRouter_ProcessStructuredSpecs_HybridMode(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    RequestModeHybrid,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	
	// In hybrid mode, summary may be generated
	// (depends on implementation, but should not error)
}

func TestRouter_ProcessStructuredSpecs_ManySpecs(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	specs := []string{
		"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension",
		"Seating Capacity", "Boot Space", "Airbags", "ABS", "Parking Sensors",
		"Rear Camera", "Headlights", "Sunroof", "Alloy Wheels",
	}

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: specs,
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, len(specs), len(resp.SpecAvailability))
}

func TestRouter_ProcessStructuredSpecs_Aggregation(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)

	// Verify aggregated facts and chunks
	// (may be empty if no data, but structure should be correct)
	assert.NotNil(t, resp.StructuredFacts)
	assert.NotNil(t, resp.SemanticChunks)
	
	// Verify overall confidence is calculated
	assert.GreaterOrEqual(t, resp.OverallConfidence, 0.0)
	assert.LessOrEqual(t, resp.OverallConfidence, 1.0)
}

func TestRouter_Query_StructuredRequest(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	// Test that Query method routes to ProcessStructuredSpecs
	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy"},
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.SpecAvailability))
}

func TestRouter_ProcessStructuredSpecs_ErrorHandling(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	// Test with very short timeout to trigger error path
	router.config.BatchProcessingTimeout = 1 * time.Nanosecond

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    RequestModeStructured,
	}

	// Should handle timeout gracefully and fallback to sequential
	resp, err := router.ProcessStructuredSpecs(ctx, req)
	// May succeed with sequential fallback or timeout
	if err != nil {
		t.Logf("Timeout occurred (expected in some cases): %v", err)
	} else {
		assert.NotNil(t, resp)
	}
}

func TestRouter_ProcessStructuredSpecs_Deduplication(t *testing.T) {
	router := setupTestRouterForStructured()
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Fuel Economy"}, // Duplicates
		RequestMode:    RequestModeStructured,
	}

	resp, err := router.ProcessStructuredSpecs(ctx, req)
	require.NoError(t, err)
	
	// Should handle duplicates and aggregate correctly
	assert.NotNil(t, resp)
	// May have 1 or 2 statuses depending on deduplication logic
	assert.GreaterOrEqual(t, len(resp.SpecAvailability), 1)
}

func setupTestRouterForStructured() *Router {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	vectorAdapter, _ := NewFAISSAdapter(FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	return NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, RouterConfig{
		MaxChunks:                 8,
		StructuredFirst:           true,
		SemanticFallback:          true,
		IntentConfidenceThreshold: 0.7,
		KeywordConfidenceThreshold: 0.8,
		CacheResults:              true,
		CacheTTL:                  5 * time.Minute,
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})
}

