// Package ingestion provides ingestion orchestration.
package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	keingest "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	keobservability "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	keretrieval "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	keembedding "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	kemontoring "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
)

// Orchestrator handles ingestion operations.
type Orchestrator struct {
	pipeline *keingest.Pipeline
	logger   *keobservability.Logger
}

// NewOrchestrator creates a new ingestion orchestrator.
func NewOrchestrator(
	logger *keobservability.Logger,
	cfg keingest.PipelineConfig,
	repos *kestorage.Repositories,
	embedder keembedding.Embedder,
	vectorAdapter keretrieval.VectorAdapter,
	lineageWriter *kemontoring.LineageWriter,
) *Orchestrator {
	pipeline := keingest.NewPipeline(logger, cfg, repos, embedder, vectorAdapter, lineageWriter)
	return &Orchestrator{
		pipeline: pipeline,
		logger:   logger,
	}
}

// IngestResult represents the result of an ingestion operation.
type IngestResult struct {
	JobID           uuid.UUID
	SpecsCreated    int
	FeaturesCreated int
	USPsCreated     int
	ChunksCreated   int
	Duration        time.Duration
}

// Ingest ingests markdown content into the knowledge engine.
func (o *Orchestrator) Ingest(ctx context.Context, req keingest.IngestionRequest) (*IngestResult, error) {
	result, err := o.pipeline.Ingest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ingestion failed: %w", err)
	}
	
	return &IngestResult{
		JobID:           result.JobID,
		SpecsCreated:    result.SpecsCreated,
		FeaturesCreated: result.FeaturesCreated,
		USPsCreated:     result.USPsCreated,
		ChunksCreated:   result.ChunksCreated,
		Duration:        result.Duration,
	}, nil
}

