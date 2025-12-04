// Package main provides the API router setup.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/handlers"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/comparison"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// NewRouter creates the main API router with all routes configured.
func NewRouter(logger *observability.Logger, appCfg *AppConfig, engineCfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger) // Use chi's built-in logger
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.CORS([]string{"*"}))
	r.Use(chimiddleware.Timeout(appCfg.RequestTimeout))

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

	// Open database connection
	var db *sql.DB
	var err error
	if engineCfg != nil {
		dsn := engineCfg.DatabaseDSN()
		var driver string
		if engineCfg.Database.Driver == "sqlite" {
			driver = "sqlite3"
		} else if engineCfg.Database.Driver == "postgres" {
			driver = "postgres"
			// TODO: Import postgres driver if needed
			logger.Warn().Msg("Postgres driver not yet implemented in API")
		} else {
			logger.Warn().Str("driver", engineCfg.Database.Driver).Msg("Unsupported database driver")
		}

		if driver != "" {
			db, err = sql.Open(driver, dsn)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to open database connection")
			} else {
				// Set connection pool settings for SQLite
				if engineCfg.Database.Driver == "sqlite" {
					db.SetMaxOpenConns(engineCfg.Database.SQLite.MaxOpenConns)
				}
			}
		}
	}

	// Create repositories
	var repos *storage.Repositories
	if db != nil {
		repos = storage.NewRepositories(db)
	}

	// Create service dependencies
	memCache := cache.NewMemoryClient(appCfg.CacheSize)

	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
		Dimension: appCfg.EmbeddingDimension,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create vector adapter")
		vectorAdapter = nil
	}

	// Create embedding client
	var embClient embedding.Embedder
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey != "" && engineCfg != nil {
		client, err := embedding.NewClient(embedding.Config{
			APIKey:  apiKey,
			Model:   engineCfg.Embedding.Model,
			BaseURL: "https://openrouter.ai/api/v1",
		})
		if err == nil {
			embClient = client
		} else {
			logger.Warn().Err(err).Msg("Failed to create embedding client, using mock")
			embClient = embedding.NewMockClient(appCfg.EmbeddingDimension)
		}
	} else {
		embClient = embedding.NewMockClient(appCfg.EmbeddingDimension)
	}

	// Create spec view repository
	var specViewRepo *storage.SpecViewRepository
	if repos != nil {
		specViewRepo = storage.NewSpecViewRepository(db)
	}

	// Initialize services
	router := retrieval.NewRouter(logger, memCache, vectorAdapter, embClient, specViewRepo, retrieval.RouterConfig{
		MaxChunks:                 appCfg.MaxChunks,
		StructuredFirst:           true,
		SemanticFallback:          true,
		IntentConfidenceThreshold: 0.7,
		KeywordConfidenceThreshold: 0.8,
		CacheResults:              true,
		CacheTTL:                  appCfg.CacheTTL,
		MinAvailabilityConfidence: 0.6,
		BatchProcessingWorkers:    5,
		BatchProcessingTimeout:     30 * time.Second,
	})

	// Create lineage writer
	var lineageWriter *monitoring.LineageWriter
	if repos != nil {
		lineageWriter = monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())
	} else {
		lineageWriter = monitoring.NewLineageWriter(logger, nil, monitoring.DefaultLineageConfig())
	}

	// Use provided batch size or default to 75
	batchSize := appCfg.EmbeddingBatchSize
	if batchSize <= 0 {
		batchSize = 75 // Default batch size
	}
	
	pipeline := ingest.NewPipeline(
		logger,
		ingest.PipelineConfig{
			ChunkSize:         512,
			ChunkOverlap:      64,
			MaxConcurrentJobs: appCfg.MaxConcurrentJobs,
			DedupeThreshold:   0.95,
			EmbeddingBatchSize: batchSize,
		},
		repos,
		embClient,
		vectorAdapter,
		lineageWriter,
	)

	publisher := ingest.NewPublisher(logger)

	compCache := comparison.NewMemoryComparisonCache()
	// TODO: Create ComparisonRepository and add to Repositories
	// For now, use a no-op store since ComparisonStore interface doesn't allow nil
	compStore := &noOpComparisonStore{}
	materializer := comparison.NewMaterializer(logger, compCache, compStore, comparison.Config{
		CacheTTL: appCfg.CacheTTL,
	})

	// Create Redis client for audit logger (nil for now if Redis not configured)
	var redisClient *cache.RedisClient = nil
	auditLogger := monitoring.NewAuditLogger(logger, redisClient)
	driftRunner := monitoring.NewDriftRunner(logger, auditLogger, monitoring.DriftConfig{
		CheckInterval:      appCfg.DriftCheckInterval,
		FreshnessThreshold: appCfg.StalenessWindow,
	})

	// Initialize handlers
	retrievalHandler := handlers.NewRetrievalHandler(logger, router, lineageWriter)
	ingestionHandler := handlers.NewIngestionHandler(logger, pipeline, publisher)
	comparisonHandler := handlers.NewComparisonHandler(logger, materializer, lineageWriter)
	lineageHandler := handlers.NewLineageHandler(logger, auditLogger)
	driftHandler := handlers.NewDriftHandler(logger, driftRunner)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Authentication middleware for all API routes
		r.Use(middleware.Auth(appCfg.AuthConfig))

		// Retrieval routes
		r.Route("/retrieval", func(r chi.Router) {
			r.Post("/query", retrievalHandler.Query)
			r.Post("/structured", retrievalHandler.Query) // Same handler, supports both formats
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
	RequestTimeout      time.Duration
	CacheSize           int
	CacheTTL            time.Duration
	MaxChunks           int
	MaxConcurrentJobs   int
	EmbeddingDimension  int
	EmbeddingBatchSize  int
	DriftCheckInterval  time.Duration
	StalenessWindow     time.Duration
	AuthConfig          middleware.AuthConfig
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
		EmbeddingBatchSize: 75, // Default batch size for embedding generation
		DriftCheckInterval: 1 * time.Hour,
		StalenessWindow:    30 * 24 * time.Hour, // 30 days
		AuthConfig: middleware.AuthConfig{
			Enabled: false, // Disabled by default for development
		},
	}
}

// noOpComparisonStore is a no-op implementation of ComparisonStore.
// TODO: Replace with actual ComparisonRepository when implemented.
type noOpComparisonStore struct{}

func (n *noOpComparisonStore) GetComparison(ctx context.Context, tenantID, primaryID, secondaryID uuid.UUID) ([]storage.ComparisonRow, error) {
	return nil, nil
}

func (n *noOpComparisonStore) SaveComparison(ctx context.Context, rows []storage.ComparisonRow) error {
	return nil
}
