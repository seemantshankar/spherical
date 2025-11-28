// Package main provides the API router setup.
package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/handlers"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/comparison"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

// NewRouter creates the main API router with all routes configured.
func NewRouter(logger *observability.Logger, cfg *AppConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger) // Use chi's built-in logger
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.CORS([]string{"*"}))
	r.Use(chimiddleware.Timeout(cfg.RequestTimeout))

	// Health check (unauthenticated)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy","service":"knowledge-engine"}`))
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check database connectivity
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Create service dependencies
	memCache := cache.NewMemoryClient(cfg.CacheSize)

	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
		Dimension: cfg.EmbeddingDimension,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create vector adapter")
		vectorAdapter = nil
	}

	// Initialize services
	// TODO: Create specViewRepo from database connection when available
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, nil, retrieval.RouterConfig{
		MaxChunks:                 cfg.MaxChunks,
		StructuredFirst:           true,
		SemanticFallback:          true,
		IntentConfidenceThreshold: 0.7,
		KeywordConfidenceThreshold: 0.8,
		CacheResults:              true,
		CacheTTL:                  cfg.CacheTTL,
	})

	pipeline := ingest.NewPipeline(logger, ingest.PipelineConfig{
		ChunkSize:         512,
		ChunkOverlap:      64,
		MaxConcurrentJobs: cfg.MaxConcurrentJobs,
		DedupeThreshold:   0.95,
	})

	publisher := ingest.NewPublisher(logger)

	compCache := comparison.NewMemoryComparisonCache()
	materializer := comparison.NewMaterializer(logger, compCache, nil, comparison.Config{
		CacheTTL: cfg.CacheTTL,
	})

	auditLogger := monitoring.NewAuditLogger(logger, nil)
	driftRunner := monitoring.NewDriftRunner(logger, nil, monitoring.DriftConfig{
		CheckInterval:      cfg.DriftCheckInterval,
		FreshnessThreshold: cfg.StalenessWindow,
	})
	lineageWriter := monitoring.NewLineageWriter(logger, nil, monitoring.DefaultLineageConfig())

	// Initialize handlers
	retrievalHandler := handlers.NewRetrievalHandler(logger, router, lineageWriter)
	ingestionHandler := handlers.NewIngestionHandler(logger, pipeline, publisher)
	comparisonHandler := handlers.NewComparisonHandler(logger, materializer, lineageWriter)
	lineageHandler := handlers.NewLineageHandler(logger, auditLogger)
	driftHandler := handlers.NewDriftHandler(logger, driftRunner)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Authentication middleware for all API routes
		r.Use(middleware.Auth(cfg.AuthConfig))

		// Retrieval routes
		r.Route("/retrieval", func(r chi.Router) {
			r.Post("/query", retrievalHandler.Query)
		})

		// Ingestion routes
		r.Route("/tenants/{tenantId}", func(r chi.Router) {
			r.Route("/products/{productId}/campaigns/{campaignId}", func(r chi.Router) {
				r.Post("/ingest", ingestionHandler.Ingest)
			})

			r.Route("/campaigns/{campaignId}", func(r chi.Router) {
				r.Post("/publish", ingestionHandler.Publish)
			})
		})

		// Comparison routes
		r.Route("/comparisons", func(r chi.Router) {
			r.Post("/query", comparisonHandler.Query)
		})

		// Lineage routes
		r.Route("/lineage", func(r chi.Router) {
			r.Get("/{resourceType}/{resourceId}", lineageHandler.GetLineage)
		})

		// Drift routes
		r.Route("/drift", func(r chi.Router) {
			r.Get("/alerts", driftHandler.ListAlerts)
			r.Post("/check", driftHandler.TriggerCheck)
		})
	})

	return r
}

// AppConfig holds application configuration.
type AppConfig struct {
	RequestTimeout     time.Duration
	CacheSize          int
	CacheTTL           time.Duration
	MaxChunks          int
	MaxConcurrentJobs  int
	EmbeddingDimension int
	DriftCheckInterval time.Duration
	StalenessWindow    time.Duration
	AuthConfig         middleware.AuthConfig
}

// DefaultAppConfig returns default configuration values.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		RequestTimeout:     30 * time.Second,
		CacheSize:          10000,
		CacheTTL:           5 * time.Minute,
		MaxChunks:          8,
		MaxConcurrentJobs:  4,
		EmbeddingDimension: 768,
		DriftCheckInterval: 1 * time.Hour,
		StalenessWindow:    30 * 24 * time.Hour, // 30 days
		AuthConfig: middleware.AuthConfig{
			Enabled: false, // Disabled by default for development
		},
	}
}
