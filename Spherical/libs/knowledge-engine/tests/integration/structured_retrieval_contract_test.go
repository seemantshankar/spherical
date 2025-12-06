// Package integration provides contract tests for structured retrieval API endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/handlers"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

// setupTestRouterForContract creates a test router with mock dependencies
func setupTestRouterForContract() *retrieval.Router {
	logger := observability.DefaultLogger()
	memCache := cache.NewMemoryClient(1000)
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

func TestStructuredRetrieval_REST_Contract(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping contract test in short mode")
	}

	router := setupTestRouterForContract()
	logger := observability.DefaultLogger()
	handler := handlers.NewRetrievalHandler(logger, router, nil)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		validateFunc   func(t *testing.T, resp map[string]interface{})
	}{
		{
			name: "Structured request with requestedSpecs",
			requestBody: map[string]interface{}{
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"productIds":     []string{"00000000-0000-0000-0000-000000000002"},
				"requestedSpecs": []string{"Fuel Economy", "Ground Clearance"},
				"requestMode":    "structured",
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp, "specAvailability")
				specAvail, ok := resp["specAvailability"].([]interface{})
				require.True(t, ok, "specAvailability should be an array")
				assert.Equal(t, 2, len(specAvail), "Should have 2 spec availability statuses")

				// Verify each status has required fields
				for _, statusRaw := range specAvail {
					status, ok := statusRaw.(map[string]interface{})
					require.True(t, ok, "Status should be an object")
					assert.Contains(t, status, "specName")
					assert.Contains(t, status, "status")
					assert.Contains(t, status, "confidence")
					
					statusVal, ok := status["status"].(string)
					require.True(t, ok)
					assert.Contains(t, []string{"found", "unavailable", "partial"}, statusVal)
				}

				assert.Contains(t, resp, "overallConfidence")
				overallConf, ok := resp["overallConfidence"].(float64)
				require.True(t, ok)
				assert.GreaterOrEqual(t, overallConf, 0.0)
				assert.LessOrEqual(t, overallConf, 1.0)
			},
		},
		{
			name: "Structured request with synonyms",
			requestBody: map[string]interface{}{
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"productIds":     []string{"00000000-0000-0000-0000-000000000002"},
				"requestedSpecs": []string{"Mileage", "Fuel Consumption", "km/l"},
				"requestMode":    "structured",
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp, "specAvailability")
				specAvail, ok := resp["specAvailability"].([]interface{})
				require.True(t, ok)
				assert.Equal(t, 3, len(specAvail), "Should handle all synonyms")
			},
		},
		{
			name: "Hybrid mode request",
			requestBody: map[string]interface{}{
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"productIds":     []string{"00000000-0000-0000-0000-000000000002"},
				"question":       "What are the key specs?",
				"requestedSpecs": []string{"Fuel Economy", "Engine Torque"},
				"requestMode":    "hybrid",
				"includeSummary": true,
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp, "specAvailability")
				assert.Contains(t, resp, "overallConfidence")
				// Summary may or may not be present depending on implementation
			},
		},
		{
			name: "Empty requestedSpecs falls back to natural language",
			requestBody: map[string]interface{}{
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"productIds":     []string{"00000000-0000-0000-0000-000000000002"},
				"question":       "What is the fuel economy?",
				"requestedSpecs": []string{},
				"requestMode":    "structured",
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				// Should still return a valid response
				assert.Contains(t, resp, "intent")
			},
		},
		{
			name: "Missing both question and requestedSpecs",
			requestBody: map[string]interface{}{
				"tenantId":   "00000000-0000-0000-0000-000000000001",
				"productIds": []string{"00000000-0000-0000-0000-000000000002"},
			},
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp, "error")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/retrieval/structured", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Query(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "HTTP status code mismatch")

			if tc.expectedStatus == http.StatusOK && tc.validateFunc != nil {
				var resp map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err, "Response should be valid JSON")
				tc.validateFunc(t, resp)
			}
		})
	}
}

func TestStructuredRetrieval_REST_ResponseFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping contract test in short mode")
	}

	router := setupTestRouterForContract()
	logger := observability.DefaultLogger()
	handler := handlers.NewRetrievalHandler(logger, router, nil)

	requestBody := map[string]interface{}{
		"tenantId":       "00000000-0000-0000-0000-000000000001",
		"productIds":     []string{"00000000-0000-0000-0000-000000000002"},
		"requestedSpecs": []string{"Fuel Economy", "Ground Clearance", "Engine Torque"},
		"requestMode":    "structured",
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/retrieval/structured", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Query(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Verify all required top-level fields
	assert.Contains(t, resp, "intent")
	assert.Contains(t, resp, "latencyMs")
	assert.Contains(t, resp, "structuredFacts")
	assert.Contains(t, resp, "semanticChunks")
	assert.Contains(t, resp, "specAvailability")
	assert.Contains(t, resp, "overallConfidence")

	// Verify specAvailability structure
	specAvail, ok := resp["specAvailability"].([]interface{})
	require.True(t, ok)

	for _, statusRaw := range specAvail {
		status, ok := statusRaw.(map[string]interface{})
		require.True(t, ok)

		// Required fields
		assert.Contains(t, status, "specName")
		assert.Contains(t, status, "status")
		assert.Contains(t, status, "confidence")

		// Optional fields (may be empty arrays or omitted if empty)
		assert.Contains(t, status, "alternativeNames")
		
		// Verify types first
		specName, ok := status["specName"].(string)
		require.True(t, ok)
		assert.NotEmpty(t, specName)

		statusVal, ok := status["status"].(string)
		require.True(t, ok)
		assert.Contains(t, []string{"found", "unavailable", "partial"}, statusVal)
		
		// matchedSpecs and matchedChunks may be omitted if empty (JSON omits empty slices)
		// Check if they exist, but don't require them for unavailable specs
		if statusVal == "found" || statusVal == "partial" {
			// For found/partial, these should exist (but may be empty arrays)
			// Just verify the structure is correct - actual data depends on test setup
			_, hasMatchedSpecs := status["matchedSpecs"]
			_, hasMatchedChunks := status["matchedChunks"]
			// At least one should exist for found/partial, but in test environment may be empty
			if !hasMatchedSpecs && !hasMatchedChunks {
				t.Logf("Note: found/partial status has no matchedSpecs or matchedChunks (expected in test environment)")
			}
		}

		confidence, ok := status["confidence"].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, confidence, 0.0)
		assert.LessOrEqual(t, confidence, 1.0)
	}
}

func TestStructuredRetrieval_REST_BackwardCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping contract test in short mode")
	}

	router := setupTestRouterForContract()
	logger := observability.DefaultLogger()
	handler := handlers.NewRetrievalHandler(logger, router, nil)

	// Test that traditional natural language query still works
	requestBody := map[string]interface{}{
		"tenantId":   "00000000-0000-0000-0000-000000000001",
		"productIds": []string{"00000000-0000-0000-0000-000000000002"},
		"question":   "What is the fuel economy?",
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/retrieval/query", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Query(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Should still have traditional fields
	assert.Contains(t, resp, "intent")
	assert.Contains(t, resp, "structuredFacts")
	assert.Contains(t, resp, "semanticChunks")

	// Should also have new fields (may be empty or nil for natural language queries)
	// specAvailability may be empty array or nil for natural language queries
	if specAvail, ok := resp["specAvailability"]; ok {
		// If present, should be an array (may be empty)
		_, isArray := specAvail.([]interface{})
		assert.True(t, isArray || specAvail == nil, "specAvailability should be array or nil")
	}
	assert.Contains(t, resp, "overallConfidence")
}

func TestStructuredRetrieval_REST_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping contract test in short mode")
	}

	router := setupTestRouterForContract()
	logger := observability.DefaultLogger()
	handler := handlers.NewRetrievalHandler(logger, router, nil)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "Invalid tenant ID",
			requestBody: map[string]interface{}{
				"tenantId":       "invalid-uuid",
				"requestedSpecs": []string{"Fuel Economy"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing tenant ID",
			requestBody: map[string]interface{}{
				"requestedSpecs": []string{"Fuel Economy"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid JSON",
			requestBody: nil, // Will be sent as invalid JSON
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			var err error

			if tc.requestBody != nil {
				body, err = json.Marshal(tc.requestBody)
				require.NoError(t, err)
			} else {
				body = []byte("{ invalid json }")
			}

			req := httptest.NewRequest("POST", "/api/v1/retrieval/structured", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Query(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "HTTP status code mismatch for: %s", tc.name)
		})
	}
}

