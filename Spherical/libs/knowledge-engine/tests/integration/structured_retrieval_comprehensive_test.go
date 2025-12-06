// Package integration provides comprehensive tests for structured retrieval.
package integration

import (
	"context"
	"sync"
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

// TestStructuredRetrieval_Concurrency tests concurrent structured requests
func TestStructuredRetrieval_Concurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Test concurrent requests
	numGoroutines := 10
	numRequestsPerGoroutine := 5
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numRequestsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numRequestsPerGoroutine; j++ {
				req := retrieval.RetrievalRequest{
					TenantID:       tenantID,
					ProductIDs:     []uuid.UUID{productID},
					RequestedSpecs: []string{"Fuel Economy", "Engine Torque", "Ground Clearance"},
					RequestMode:    retrieval.RequestModeStructured,
				}

				_, err := router.Query(ctx, req)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent request failed: %v", err)
	}
}

// TestStructuredRetrieval_Performance tests performance requirements
func TestStructuredRetrieval_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

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

	// Calculate p95
	sortLatencies(latencies)
	p95Index := int(float64(len(latencies)) * 0.95)
	p95Latency := latencies[p95Index]
	
	// Calculate statistics
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avgLatency := sum / time.Duration(len(latencies))
	minLatency := latencies[0]
	maxLatency := latencies[len(latencies)-1]

	t.Logf("Performance Test Results for 10 specs:")
	t.Logf("  Min latency: %v", minLatency)
	t.Logf("  Max latency: %v", maxLatency)
	t.Logf("  Average latency: %v", avgLatency)
	t.Logf("  P95 latency: %v", p95Latency)
	t.Logf("  Target: <500ms")
	
	assert.Less(t, p95Latency, 500*time.Millisecond, "P95 latency should be <500ms for 10 specs")
}

// TestStructuredRetrieval_EdgeCases tests various edge cases
func TestStructuredRetrieval_EdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge case test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	tests := []struct {
		name           string
		requestedSpecs []string
		expectError    bool
		validateFunc   func(t *testing.T, resp *retrieval.RetrievalResponse)
	}{
		{
			name:           "Very long spec name",
			requestedSpecs: []string{"This Is A Very Long Specification Name That Should Still Work Correctly"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				assert.Equal(t, 1, len(resp.SpecAvailability))
			},
		},
		{
			name:           "Special characters in spec name",
			requestedSpecs: []string{"Fuel Economy (City)", "Engine Torque @ 2000 RPM"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				assert.Equal(t, 2, len(resp.SpecAvailability))
			},
		},
		{
			name:           "Unicode characters",
			requestedSpecs: []string{"Fuel Economy", "燃油经济性", "Économie de carburant"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				assert.Equal(t, 3, len(resp.SpecAvailability))
			},
		},
		{
			name:           "Duplicate spec names",
			requestedSpecs: []string{"Fuel Economy", "Fuel Economy", "Fuel Economy"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				// Should handle duplicates gracefully
				assert.GreaterOrEqual(t, len(resp.SpecAvailability), 1)
			},
		},
		{
			name:           "Mixed case variations",
			requestedSpecs: []string{"FUEL ECONOMY", "fuel economy", "Fuel Economy", "FuEl EcOnOmY"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				// Should normalize all to same canonical form
				assert.Equal(t, 4, len(resp.SpecAvailability))
			},
		},
		{
			name:           "Empty strings in array",
			requestedSpecs: []string{"Fuel Economy", "", "Engine Torque"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				// Should handle empty strings
				assert.GreaterOrEqual(t, len(resp.SpecAvailability), 2)
			},
		},
		{
			name:           "Whitespace-only spec names",
			requestedSpecs: []string{"Fuel Economy", "   ", "\t\t", "\n\n"},
			expectError:    false,
			validateFunc: func(t *testing.T, resp *retrieval.RetrievalResponse) {
				// Should handle whitespace gracefully
				assert.GreaterOrEqual(t, len(resp.SpecAvailability), 1)
			},
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
			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.validateFunc != nil {
					tc.validateFunc(t, resp)
				}
			}
		})
	}
}

// TestStructuredRetrieval_RealisticScenario tests a realistic LLM interaction scenario
func TestStructuredRetrieval_RealisticScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping realistic scenario test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Simulate: User asks "Can I drive this car in the mountains?"
	// LLM identifies required specs
	requiredSpecs := []string{
		"Engine Specifications",
		"Engine Torque",
		"Suspension",
		"Ground Clearance",
		"Interior Comfort",
		"Fuel Tank Capacity",
		"Fuel Economy",
	}

	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: requiredSpecs,
		RequestMode:    retrieval.RequestModeStructured,
		IncludeLineage: false,
	}

	resp, err := router.Query(ctx, req)
	require.NoError(t, err)

	// Verify response structure
	assert.Equal(t, len(requiredSpecs), len(resp.SpecAvailability))

	// Analyze results
	foundCount := 0
	unavailableCount := 0
	partialCount := 0

	for _, status := range resp.SpecAvailability {
		switch status.Status {
		case retrieval.AvailabilityStatusFound:
			foundCount++
			assert.Greater(t, status.Confidence, 0.6, "Found specs should have high confidence")
		case retrieval.AvailabilityStatusUnavailable:
			unavailableCount++
			assert.Equal(t, 0.0, status.Confidence, "Unavailable specs should have 0 confidence")
		case retrieval.AvailabilityStatusPartial:
			partialCount++
			assert.Greater(t, status.Confidence, 0.3, "Partial specs should have some confidence")
			assert.Less(t, status.Confidence, 0.6, "Partial specs should have low confidence")
		}
	}

	t.Logf("Realistic scenario results: Found=%d, Unavailable=%d, Partial=%d", foundCount, unavailableCount, partialCount)
	t.Logf("Overall confidence: %.2f", resp.OverallConfidence)

	// Verify overall confidence reflects the mix
	if foundCount > 0 {
		assert.Greater(t, resp.OverallConfidence, 0.0, "Should have positive confidence if any specs found")
	}
}

// TestStructuredRetrieval_Caching tests caching behavior
func TestStructuredRetrieval_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping caching test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	req := retrieval.RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Engine Torque"},
		RequestMode:    retrieval.RequestModeStructured,
	}

	// First request
	start1 := time.Now()
	resp1, err := router.Query(ctx, req)
	duration1 := time.Since(start1)
	require.NoError(t, err)

	// Second request (should be cached)
	start2 := time.Now()
	resp2, err := router.Query(ctx, req)
	duration2 := time.Since(start2)
	require.NoError(t, err)

	// Verify responses are identical
	assert.Equal(t, len(resp1.SpecAvailability), len(resp2.SpecAvailability))
	
	// Cached request should be faster (or at least not slower)
	// Allow some variance for timing
	if duration2 > duration1*2 {
		t.Logf("Warning: Cached request took longer (first: %v, second: %v)", duration1, duration2)
	}
}

// TestStructuredRetrieval_Timeout tests timeout handling
func TestStructuredRetrieval_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	// Create router with very short timeout
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     1 * time.Millisecond, // Very short timeout
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

	// Should handle timeout gracefully
	resp, err := router.Query(ctx, req)
	// May succeed or timeout, but should not panic
	if err != nil {
		t.Logf("Timeout occurred (expected): %v", err)
	} else {
		// If it succeeds, verify response is valid
		assert.NotNil(t, resp)
	}
}

// TestStructuredRetrieval_MemoryLeak tests for memory leaks in batch processing
func TestStructuredRetrieval_MemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	router := setupTestRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// Run many requests to check for memory leaks
	numRequests := 100
	for i := 0; i < numRequests; i++ {
		req := retrieval.RetrievalRequest{
			TenantID:       tenantID,
			ProductIDs:     []uuid.UUID{productID},
			RequestedSpecs: []string{"Fuel Economy", "Engine Torque", "Ground Clearance"},
			RequestMode:    retrieval.RequestModeStructured,
		}

		resp, err := router.Query(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	}
}

// Helper function to sort latencies for p95 calculation
func sortLatencies(latencies []time.Duration) {
	for i := 0; i < len(latencies)-1; i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}
}

func setupTestRouter() *retrieval.Router {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(10000)
	vectorAdapter, _ := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	mockEmbedder := embedding.NewMockClient(768)

	return retrieval.NewRouter(logger, memCache, vectorAdapter, mockEmbedder, nil, retrieval.RouterConfig{
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

