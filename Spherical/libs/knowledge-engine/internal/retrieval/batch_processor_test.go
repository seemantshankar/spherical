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

func TestNewBatchProcessor(t *testing.T) {
	router := setupTestRouterForBatch()
	
	// Test with defaults
	bp := NewBatchProcessor(router, 0, 0)
	assert.NotNil(t, bp)
	assert.Equal(t, 5, bp.maxWorkers) // Default
	assert.Equal(t, 30*time.Second, bp.timeout) // Default

	// Test with custom values
	bp = NewBatchProcessor(router, 10, 60*time.Second)
	assert.Equal(t, 10, bp.maxWorkers)
	assert.Equal(t, 60*time.Second, bp.timeout)
}

func TestBatchProcessor_ProcessSpecsInParallel_SingleSpec(t *testing.T) {
	router := setupTestRouterForBatch()
	bp := NewBatchProcessor(router, 5, 30*time.Second)
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy"},
	}

	results, err := bp.ProcessSpecsInParallel(ctx, []string{"Fuel Economy"}, req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.NotEmpty(t, results[0].SpecName)
}

func TestBatchProcessor_ProcessSpecsInParallel_MultipleSpecs(t *testing.T) {
	router := setupTestRouterForBatch()
	bp := NewBatchProcessor(router, 3, 30*time.Second)
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque", "Ground Clearance"},
	}

	results, err := bp.ProcessSpecsInParallel(ctx, []string{"Fuel Economy", "Engine Torque", "Ground Clearance"}, req)
	require.NoError(t, err)
	assert.Equal(t, 3, len(results))
	
	for _, result := range results {
		assert.NotEmpty(t, result.SpecName)
		assert.Contains(t, []AvailabilityStatus{
			AvailabilityStatusFound,
			AvailabilityStatusUnavailable,
			AvailabilityStatusPartial,
		}, result.Status)
	}
}

func TestBatchProcessor_ProcessSpecsInParallel_EmptyList(t *testing.T) {
	router := setupTestRouterForBatch()
	bp := NewBatchProcessor(router, 5, 30*time.Second)
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
	}

	results, err := bp.ProcessSpecsInParallel(ctx, []string{}, req)
	require.NoError(t, err)
	assert.Equal(t, 0, len(results))
}

func TestBatchProcessor_ProcessSpecsInParallel_Timeout(t *testing.T) {
	router := setupTestRouterForBatch()
	bp := NewBatchProcessor(router, 5, 1*time.Millisecond) // Very short timeout
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
	}

	// Should handle timeout gracefully
	results, err := bp.ProcessSpecsInParallel(ctx, []string{"Fuel Economy"}, req)
	// May timeout or succeed, but should not panic
	if err != nil {
		assert.Contains(t, err.Error(), "timeout")
	} else {
		assert.NotNil(t, results)
	}
}

func TestBatchProcessor_ProcessSpecsInParallel_ManySpecs(t *testing.T) {
	router := setupTestRouterForBatch()
	bp := NewBatchProcessor(router, 5, 30*time.Second)
	ctx := context.Background()

	req := RetrievalRequest{
		TenantID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ProductIDs:     []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
	}

	// Test with 10 specs
	specs := []string{
		"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension",
		"Seating Capacity", "Boot Space", "Airbags", "ABS", "Parking Sensors",
		"Rear Camera",
	}

	results, err := bp.ProcessSpecsInParallel(ctx, specs, req)
	require.NoError(t, err)
	assert.Equal(t, len(specs), len(results))
}

func setupTestRouterForBatch() *Router {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
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



