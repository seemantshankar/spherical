package retrieval

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
)

type mockVectorAdapter struct {
	results []VectorResult
	called  bool
}

func (m *mockVectorAdapter) Search(ctx context.Context, query []float32, k int, filters VectorFilters) ([]VectorResult, error) {
	m.called = true
	return m.results, nil
}
func (m *mockVectorAdapter) Insert(ctx context.Context, vectors []VectorEntry) error { return nil }
func (m *mockVectorAdapter) Delete(ctx context.Context, ids []uuid.UUID) error       { return nil }
func (m *mockVectorAdapter) Count(ctx context.Context) (int64, error) {
	return int64(len(m.results)), nil
}
func (m *mockVectorAdapter) Close() error { return nil }

func TestRouter_SemanticFallbackReturnsSpecFacts(t *testing.T) {
	tenantID := uuid.New()

	vector := &mockVectorAdapter{
		results: []VectorResult{
			{
				ID:       uuid.New(),
				Distance: 0.1,
				Score:    0.9,
				Metadata: map[string]interface{}{
					"chunk_type":           string(storage.ChunkTypeSpecFact),
					"chunk_text":           "Engine > Battery Range: 300 km; Key features: Fast charge support",
					"category":             "Engine",
					"name":                 "Battery Range",
					"value":                "300",
					"unit":                 "km",
					"key_features":         "Fast charge support",
					"variant_availability": "Standard",
					"explanation":          "Battery range is 300 km.",
				},
			},
		},
	}

	router := NewRouter(
		observability.DefaultLogger(),
		nil, // cache
		vector,
		embedding.NewMockClient(16),
		nil, // spec view repo (forces fallback)
		RouterConfig{
			SemanticFallback:           true,
			KeywordConfidenceThreshold: 0.8,
			MaxChunks:                  5,
		},
	)

	req := RetrievalRequest{
		TenantID:  tenantID,
		Question:  "What is the battery range?",
		MaxChunks: 5,
	}

	resp, err := router.Query(context.Background(), req)
	assert.NoError(t, err)
	assert.True(t, vector.called, "vector search should be used for fallback")
	assert.Len(t, resp.SemanticChunks, 1)
	assert.Len(t, resp.StructuredFacts, 1)
	assert.Equal(t, "semantic", resp.StructuredFacts[0].Provenance)
	assert.Equal(t, "Battery Range", resp.StructuredFacts[0].Name)
	assert.NotEmpty(t, resp.StructuredFacts[0].Explanation)
}
