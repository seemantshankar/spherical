package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

// BenchmarkStructuredRetrieval_SingleSpec benchmarks a single spec request
func BenchmarkStructuredRetrieval_SingleSpec(b *testing.B) {
	router := setupBenchmarkRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	req := RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy"},
		RequestMode:    RequestModeStructured,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Query(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStructuredRetrieval_MultipleSpecs benchmarks multiple specs in one request
func BenchmarkStructuredRetrieval_MultipleSpecs(b *testing.B) {
	router := setupBenchmarkRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	req := RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: []string{"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension", "Seating Capacity"},
		RequestMode:    RequestModeStructured,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Query(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStructuredRetrieval_ManySpecs benchmarks a large batch of specs
func BenchmarkStructuredRetrieval_ManySpecs(b *testing.B) {
	router := setupBenchmarkRouter()
	ctx := context.Background()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	// 20 specs - realistic LLM request size
	specs := []string{
		"Fuel Economy", "Ground Clearance", "Engine Torque", "Suspension",
		"Seating Capacity", "Boot Space", "Airbags", "ABS", "Parking Sensors",
		"Rear Camera", "Headlights", "Sunroof", "Alloy Wheels", "Navigation",
		"Bluetooth", "USB", "Climate Control", "Leather Upholstery", "Power Windows",
		"Central Locking",
	}

	req := RetrievalRequest{
		TenantID:       tenantID,
		ProductIDs:     []uuid.UUID{productID},
		RequestedSpecs: specs,
		RequestMode:    RequestModeStructured,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Query(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSpecNormalizer benchmarks spec name normalization
func BenchmarkSpecNormalizer_Normalize(b *testing.B) {
	normalizer := NewSpecNormalizer()
	specs := []string{
		"Fuel Economy", "Mileage", "Fuel Consumption", "km/l",
		"Engine Torque", "Torque", "Maximum Torque",
		"Ground Clearance", "Ground Clearance Height",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, spec := range specs {
			normalizer.NormalizeSpecName(spec)
		}
	}
}

// BenchmarkAvailabilityDetector benchmarks availability detection
func BenchmarkAvailabilityDetector_Determine(b *testing.B) {
	detector := NewAvailabilityDetector(0.6, 0.5)
	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.9,
		},
	}
	chunks := []SemanticChunk{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetermineAvailability("Fuel Economy", facts, chunks)
	}
}

// BenchmarkConfidenceCalculator benchmarks confidence calculation
func BenchmarkConfidenceCalculator_Calculate(b *testing.B) {
	calc := NewConfidenceCalculator()
	facts := []SpecFact{
		{
			Category:   "Fuel Efficiency",
			Name:       "Fuel Economy",
			Value:      "25.49",
			Confidence: 0.9,
		},
	}
	chunks := []SemanticChunk{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.CalculateOverallConfidence(facts, chunks, 0.8)
	}
}

func setupBenchmarkRouter() *Router {
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



