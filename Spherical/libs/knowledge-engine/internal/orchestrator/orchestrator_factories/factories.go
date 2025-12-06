package orchestrator_factories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	keembedding "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	keingest "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	kemontoring "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	keobservability "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	keretrieval "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/ingestion"
)

// IngestionOrchestratorWrapper wraps ingestion orchestrator with convenience methods.
type IngestionOrchestratorWrapper struct {
	logger       *keobservability.Logger
	repos        *kestorage.Repositories
	embedder     keembedding.Embedder
	lineageWriter *kemontoring.LineageWriter
	cfg          *config.Config
	pipelineCfg  keingest.PipelineConfig
}

// QueryResult represents a query result with structured facts and semantic chunks.
type QueryResult struct {
	StructuredFacts []keretrieval.SpecFact
	SemanticChunks  []keretrieval.SemanticChunk
}

// QueryOrchestratorWrapper wraps the query orchestrator with convenience methods.
type QueryOrchestratorWrapper struct {
	logger        *keobservability.Logger
	repos         *kestorage.Repositories
	embedder      keembedding.Embedder
	routerCfg     keretrieval.RouterConfig
	cfg           *config.Config
	db            *sql.DB
}

// NewIngestionOrchestrator creates a fully configured ingestion orchestrator wrapper.
func NewIngestionOrchestrator(cfg *config.Config, db *sql.DB) (*IngestionOrchestratorWrapper, error) {
	// Create logger
	logger := keobservability.NewLogger(keobservability.LogConfig{
		Level:       cfg.KnowledgeEngine.Observability.LogLevel,
		Format:      cfg.KnowledgeEngine.Observability.LogFormat,
		ServiceName: "orchestrator-ingestion",
	})

	// Create repositories
	repos := kestorage.NewRepositories(db)

	// Get API key
	apiKey, err := cfg.GetOpenRouterAPIKey()
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	// Determine embedding model and dimension
	embeddingModel := cfg.KnowledgeEngine.Embedding.Model
	if embeddingModel == "" {
		embeddingModel = "google/gemini-embedding-001" // Default
	}
	dimension := cfg.KnowledgeEngine.Embedding.Dimension
	if dimension == 0 {
		dimension = 768 // Default
	}

	// Create embedding client
	embedder, err := keembedding.NewClient(keembedding.Config{
		APIKey:    apiKey,
		Model:     embeddingModel,
		BaseURL:   "https://openrouter.ai/api/v1",
		Dimension: dimension,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	// Create lineage writer
	lineageWriter := kemontoring.NewLineageWriter(logger, repos.Lineage, kemontoring.DefaultLineageConfig())

	// Create pipeline config
	pipelineCfg := keingest.PipelineConfig{
		ChunkSize:         cfg.KnowledgeEngine.Ingestion.ChunkSize,
		ChunkOverlap:      cfg.KnowledgeEngine.Ingestion.ChunkOverlap,
		MaxConcurrentJobs: cfg.KnowledgeEngine.Ingestion.MaxConcurrentJobs,
		DedupeThreshold:   cfg.KnowledgeEngine.Ingestion.DedupeThreshold,
		EmbeddingBatchSize: cfg.Ingestion.EmbeddingBatchSize,
	}

	return &IngestionOrchestratorWrapper{
		logger:        logger,
		repos:         repos,
		embedder:      embedder,
		lineageWriter: lineageWriter,
		cfg:           cfg,
		pipelineCfg:   pipelineCfg,
	}, nil
}

// IngestMarkdown ingests markdown content for a campaign with a per-campaign vector adapter.
func (w *IngestionOrchestratorWrapper) IngestMarkdown(
	ctx context.Context,
	tenantID, productID, campaignID uuid.UUID,
	markdownPath string,
	vectorAdapter keretrieval.VectorAdapter,
) (*ingestion.IngestResult, error) {
	// Create pipeline with per-campaign vector adapter
	pipeline := keingest.NewPipeline(
		w.logger,
		w.pipelineCfg,
		w.repos,
		w.embedder,
		vectorAdapter,
		w.lineageWriter,
	)

	// Create ingestion request
	req := keingest.IngestionRequest{
		TenantID:     tenantID,
		ProductID:    productID,
		CampaignID:   campaignID,
		MarkdownPath: markdownPath,
		Operator:     "orchestrator-cli",
		Overwrite:    true,
		AutoPublish:  w.cfg.Ingestion.AutoPublish,
	}

	// Run ingestion
	result, err := pipeline.Ingest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ingestion failed: %w", err)
	}

	return &ingestion.IngestResult{
		JobID:           result.JobID,
		SpecsCreated:    result.SpecsCreated,
		FeaturesCreated: result.FeaturesCreated,
		USPsCreated:     result.USPsCreated,
		ChunksCreated:   result.ChunksCreated,
		Duration:        result.Duration,
	}, nil
}

// NewQueryOrchestrator creates a fully configured query orchestrator wrapper.
func NewQueryOrchestrator(cfg *config.Config, db *sql.DB) (*QueryOrchestratorWrapper, error) {
	// Create logger
	logger := keobservability.NewLogger(keobservability.LogConfig{
		Level:       cfg.KnowledgeEngine.Observability.LogLevel,
		Format:      cfg.KnowledgeEngine.Observability.LogFormat,
		ServiceName: "orchestrator-query",
	})

	// Create repositories
	repos := kestorage.NewRepositories(db)

	// Get API key
	apiKey, err := cfg.GetOpenRouterAPIKey()
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	// Determine embedding model and dimension
	embeddingModel := cfg.KnowledgeEngine.Embedding.Model
	if embeddingModel == "" {
		embeddingModel = "google/gemini-embedding-001" // Default
	}
	dimension := cfg.KnowledgeEngine.Embedding.Dimension
	if dimension == 0 {
		dimension = 768 // Default
	}

	// Create embedding client
	embedder, err := keembedding.NewClient(keembedding.Config{
		APIKey:    apiKey,
		Model:     embeddingModel,
		BaseURL:   "https://openrouter.ai/api/v1",
		Dimension: dimension,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	// Create router config
	routerCfg := keretrieval.RouterConfig{
		MaxChunks:                 cfg.KnowledgeEngine.Retrieval.MaxChunks,
		StructuredFirst:           cfg.KnowledgeEngine.Retrieval.StructuredFirst,
		SemanticFallback:          cfg.KnowledgeEngine.Retrieval.SemanticFallback,
		IntentConfidenceThreshold: cfg.KnowledgeEngine.Retrieval.IntentConfidenceThreshold,
		CacheResults:              cfg.KnowledgeEngine.Retrieval.CacheResults,
		CacheTTL:                  5 * time.Minute, // Default cache TTL
	}

	return &QueryOrchestratorWrapper{
		logger:    logger,
		repos:     repos,
		embedder:  embedder,
		routerCfg: routerCfg,
		cfg:       cfg,
		db:        db,
	}, nil
}

// Query executes a query with a per-campaign vector adapter.
func (w *QueryOrchestratorWrapper) Query(
	ctx context.Context,
	tenantID, productID, campaignID uuid.UUID,
	vectorAdapter keretrieval.VectorAdapter,
	question string,
) (*QueryResult, error) {
	// Create spec view repository (needs DB connection)
	specViewRepo := kestorage.NewSpecViewRepository(w.db)

	// Create router with per-campaign vector adapter
	router := keretrieval.NewRouter(
		w.logger,
		nil, // No cache for CLI
		vectorAdapter,
		w.embedder,
		specViewRepo,
		w.routerCfg,
	)

	// Create retrieval request
	req := keretrieval.RetrievalRequest{
		TenantID:          tenantID,
		ProductIDs:        []uuid.UUID{productID},
		CampaignVariantID: &campaignID,
		Question:          question,
		MaxChunks:         w.routerCfg.MaxChunks,
	}

	// Execute query
	startTime := time.Now()
	resp, err := router.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	_ = time.Since(startTime) // Latency available if needed

	return &QueryResult{
		StructuredFacts: resp.StructuredFacts,
		SemanticChunks:  resp.SemanticChunks,
	}, nil
}
