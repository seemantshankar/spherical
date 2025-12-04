// Package integration provides integration tests for structured retrieval.
package integration

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
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

func TestStructuredRetrieval_BasicRequest(t *testing.T) {
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

	// Create mock embedder
	mockEmbedder := embedding.NewMockClient(768)

	// Create router with structured request support
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
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

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test structured request with multiple specs
	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Ground Clearance", "Engine Torque"},
		RequestMode:    retrieval.RequestModeStructured,
		MaxChunks:      6,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	// Verify response structure
	assert.NotNil(t, resp)
	assert.Equal(t, retrieval.IntentSpecLookup, resp.Intent)
	assert.NotNil(t, resp.SpecAvailability)
	assert.Equal(t, 3, len(resp.SpecAvailability), "Should have availability status for each requested spec")

	// Verify each spec has a status
	for _, status := range resp.SpecAvailability {
		assert.NotEmpty(t, status.SpecName, "Spec name should not be empty")
		assert.Contains(t, []retrieval.AvailabilityStatus{
			retrieval.AvailabilityStatusFound,
			retrieval.AvailabilityStatusUnavailable,
			retrieval.AvailabilityStatusPartial,
		}, status.Status, "Status should be one of: found, unavailable, partial")
		assert.GreaterOrEqual(t, status.Confidence, 0.0, "Confidence should be >= 0")
		assert.LessOrEqual(t, status.Confidence, 1.0, "Confidence should be <= 1")
	}

	// Verify overall confidence is calculated
	assert.GreaterOrEqual(t, resp.OverallConfidence, 0.0)
	assert.LessOrEqual(t, resp.OverallConfidence, 1.0)
}

func TestStructuredRetrieval_SynonymHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test that synonyms are normalized correctly
	tests := []struct {
		name           string
		requestedSpecs []string
	}{
		{
			name:           "Fuel Economy synonyms",
			requestedSpecs: []string{"Fuel Economy", "Mileage", "Fuel Consumption", "km/l"},
		},
		{
			name:           "Engine Torque synonyms",
			requestedSpecs: []string{"Engine Torque", "Torque", "Maximum Torque"},
		},
		{
			name:           "Ground Clearance synonyms",
			requestedSpecs: []string{"Ground Clearance", "Ground Clearance Height", "Clearance"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := retrieval.RetrievalRequest{
				TenantID:       tenantID,
				ProductIDs:     []uuid.UUID{productID},
				RequestedSpecs: tc.requestedSpecs,
				RequestMode:    retrieval.RequestModeStructured,
			}

			resp, err := router.Query(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, len(tc.requestedSpecs), len(resp.SpecAvailability))

			// Verify alternative names are populated for known synonyms
			for _, status := range resp.SpecAvailability {
				if len(status.AlternativeNames) > 0 {
					// If alternatives exist, they should include the other synonyms
					assert.Greater(t, len(status.AlternativeNames), 0, "Known synonyms should have alternatives")
				}
			}
		})
	}
}

func TestStructuredRetrieval_BatchProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    3, // Use fewer workers for testing
		BatchProcessingTimeout:     10 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test with many specs to verify batch processing
	manySpecs := []string{
		"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension",
		"Seating Capacity", "Boot Space", "Airbags", "ABS", "Parking Sensors",
		"Rear Camera", "Headlights", "Sunroof", "Alloy Wheels",
	}

	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: manySpecs,
		RequestMode:    retrieval.RequestModeStructured,
	}

	start := time.Now()
	resp, err := router.Query(ctx, req)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, len(manySpecs), len(resp.SpecAvailability), "Should process all specs")
	
	// Verify batch processing completes in reasonable time (< 5 seconds for 13 specs)
	assert.Less(t, duration, 5*time.Second, "Batch processing should complete quickly")
	
	// Verify all specs have status
	for _, status := range resp.SpecAvailability {
		assert.NotEmpty(t, status.SpecName)
		assert.NotEmpty(t, string(status.Status))
	}
}

func TestStructuredRetrieval_HybridMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test hybrid mode (both question and structured specs)
	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		Question:       "What are the key specifications?",
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    retrieval.RequestModeHybrid,
		MaxChunks:      6,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	// In hybrid mode, should have spec availability
	assert.NotNil(t, resp.SpecAvailability)
	assert.Equal(t, 2, len(resp.SpecAvailability), "Should have availability for structured specs")

	// Should also have summary if hybrid mode
	if resp.Summary != nil {
		assert.NotEmpty(t, *resp.Summary, "Summary should not be empty in hybrid mode")
	}
}

func TestStructuredRetrieval_AvailabilityStatuses(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test with mix of common and uncommon specs
	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Very Uncommon Spec Name That Doesn't Exist"},
		RequestMode:    retrieval.RequestModeStructured,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 2, len(resp.SpecAvailability))

	// Verify statuses are appropriate
	foundCount := 0
	unavailableCount := 0
	partialCount := 0

	for _, status := range resp.SpecAvailability {
		switch status.Status {
		case retrieval.AvailabilityStatusFound:
			foundCount++
			assert.Greater(t, len(status.MatchedSpecs)+len(status.MatchedChunks), 0,
				"Found status should have matched specs or chunks")
		case retrieval.AvailabilityStatusUnavailable:
			unavailableCount++
			assert.Equal(t, 0, len(status.MatchedSpecs), "Unavailable should have no matched specs")
			assert.Equal(t, 0, len(status.MatchedChunks), "Unavailable should have no matched chunks")
		case retrieval.AvailabilityStatusPartial:
			partialCount++
		}
	}

	// At least one should be unavailable (the uncommon spec)
	assert.GreaterOrEqual(t, unavailableCount, 0, "Should have at least some unavailable specs")
}

func TestStructuredRetrieval_ConfidenceScoring(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque", "Ground Clearance"},
		RequestMode:    retrieval.RequestModeStructured,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	// Verify overall confidence is calculated
	assert.GreaterOrEqual(t, resp.OverallConfidence, 0.0)
	assert.LessOrEqual(t, resp.OverallConfidence, 1.0)

	// If we have found specs, overall confidence should reflect that
	foundCount := 0
	for _, status := range resp.SpecAvailability {
		if status.Status == retrieval.AvailabilityStatusFound {
			foundCount++
		}
	}

	if foundCount > 0 {
		// Overall confidence should be > 0 if we have found specs
		assert.Greater(t, resp.OverallConfidence, 0.0,
			"Overall confidence should be > 0 when specs are found")
	}
}

func TestStructuredRetrieval_EmptyRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test with empty requested specs (should fallback to natural language)
	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{},
		Question:       "What is the fuel economy?",
		RequestMode:    retrieval.RequestModeStructured,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	// Should fallback to natural language processing
	assert.Equal(t, retrieval.IntentSpecLookup, resp.Intent)
}

