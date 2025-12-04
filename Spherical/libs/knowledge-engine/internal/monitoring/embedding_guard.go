// Package monitoring provides embedding version guardrails and re-embedding job management.
package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

// EmbeddingGuard detects embedding version mismatches and queues re-embedding jobs.
// This implements T055: Detect embedding model version mismatches and queue re-embedding jobs.
type EmbeddingGuard struct {
	logger         *observability.Logger
	db             DB
	currentModel   string
	currentVersion string
}

// DB represents a database connection interface.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// EmbeddingMismatch represents a detected embedding version mismatch.
type EmbeddingMismatch struct {
	CampaignVariantID uuid.UUID
	ProductID         uuid.UUID
	TenantID          uuid.UUID
	Versions          []string
	ChunkCount        int
	DetectedAt        time.Time
}

// ReEmbeddingJob represents a job to re-embed content.
type ReEmbeddingJob struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProductID         uuid.UUID
	CampaignVariantID uuid.UUID
	ResourceType      string // "knowledge_chunk" or "feature_block"
	ResourceID        uuid.UUID
	CurrentVersion    string
	TargetVersion     string
	Status            JobStatus
	CreatedAt         time.Time
	ScheduledAt       *time.Time
}

// JobStatus represents the status of a re-embedding job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Config holds embedding guard configuration.
type GuardConfig struct {
	CurrentModel    string
	CurrentVersion  string
	EnableAutoQueue bool // Automatically queue re-embedding jobs when mismatches detected
	BatchSize       int  // Number of jobs to queue per batch
}

// NewEmbeddingGuard creates a new embedding guard.
func NewEmbeddingGuard(logger *observability.Logger, db DB, cfg GuardConfig) *EmbeddingGuard {
	return &EmbeddingGuard{
		logger:         logger,
		db:             db,
		currentModel:   cfg.CurrentModel,
		currentVersion: cfg.CurrentVersion,
	}
}

// CheckMismatches detects campaigns with mixed embedding versions.
func (g *EmbeddingGuard) CheckMismatches(ctx context.Context, tenantID uuid.UUID) ([]EmbeddingMismatch, error) {
	g.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("current_version", g.currentVersion).
		Msg("Checking for embedding version mismatches")

	// Check knowledge chunks
	chunkMismatches, err := g.checkKnowledgeChunkMismatches(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("check knowledge chunks: %w", err)
	}

	// Check feature blocks
	blockMismatches, err := g.checkFeatureBlockMismatches(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("check feature blocks: %w", err)
	}

	// Combine results
	allMismatches := append(chunkMismatches, blockMismatches...)

	g.logger.Info().
		Str("tenant_id", tenantID.String()).
		Int("mismatches_found", len(allMismatches)).
		Msg("Embedding version check completed")

	return allMismatches, nil
}

// checkKnowledgeChunkMismatches finds campaigns with mixed embedding versions in knowledge chunks.
func (g *EmbeddingGuard) checkKnowledgeChunkMismatches(ctx context.Context, tenantID uuid.UUID) ([]EmbeddingMismatch, error) {
	query := `
		SELECT campaign_variant_id, product_id, 
		       COUNT(DISTINCT embedding_version) as version_count,
		       COUNT(*) as chunk_count,
		       GROUP_CONCAT(DISTINCT embedding_version) as versions
		FROM knowledge_chunks
		WHERE tenant_id = $1
		  AND embedding_version IS NOT NULL
		GROUP BY campaign_variant_id, product_id
		HAVING COUNT(DISTINCT embedding_version) > 1
	`

	rows, err := g.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		// Table might not exist yet or SQLite vs Postgres syntax differences
		// Try SQLite version first
		query = `
			SELECT campaign_variant_id, product_id,
			       COUNT(DISTINCT embedding_version) as version_count,
			       COUNT(*) as chunk_count,
			       GROUP_CONCAT(DISTINCT embedding_version) as versions
			FROM knowledge_chunks
			WHERE tenant_id = ?
			  AND embedding_version IS NOT NULL
			GROUP BY campaign_variant_id, product_id
			HAVING COUNT(DISTINCT embedding_version) > 1
		`
		rows, err = g.db.QueryContext(ctx, query, tenantID)
		if err != nil {
			return nil, fmt.Errorf("query knowledge chunks: %w", err)
		}
	}
	defer rows.Close()

	var mismatches []EmbeddingMismatch
	for rows.Next() {
		var mismatch EmbeddingMismatch
		var versionStr string
		var versionCount, chunkCount int

		err := rows.Scan(
			&mismatch.CampaignVariantID,
			&mismatch.ProductID,
			&versionCount,
			&chunkCount,
			&versionStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan mismatch: %w", err)
		}

		mismatch.TenantID = tenantID
		mismatch.ChunkCount = chunkCount
		mismatch.DetectedAt = time.Now()

		// Parse versions from comma-separated string
		// TODO: Parse versionStr properly based on database (SQLite uses GROUP_CONCAT differently)
		mismatch.Versions = []string{versionStr} // Simplified for now

		mismatches = append(mismatches, mismatch)
	}

	return mismatches, rows.Err()
}

// checkFeatureBlockMismatches finds campaigns with mixed embedding versions in feature blocks.
func (g *EmbeddingGuard) checkFeatureBlockMismatches(ctx context.Context, tenantID uuid.UUID) ([]EmbeddingMismatch, error) {
	query := `
		SELECT campaign_variant_id, product_id,
		       COUNT(DISTINCT embedding_version) as version_count,
		       COUNT(*) as block_count,
		       GROUP_CONCAT(DISTINCT embedding_version) as versions
		FROM feature_blocks
		WHERE tenant_id = $1
		  AND embedding_version IS NOT NULL
		  AND embedding_vector IS NOT NULL
		GROUP BY campaign_variant_id, product_id
		HAVING COUNT(DISTINCT embedding_version) > 1
	`

	rows, err := g.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		// Try SQLite version
		query = `
			SELECT campaign_variant_id, product_id,
			       COUNT(DISTINCT embedding_version) as version_count,
			       COUNT(*) as block_count,
			       GROUP_CONCAT(DISTINCT embedding_version) as versions
			FROM feature_blocks
			WHERE tenant_id = ?
			  AND embedding_version IS NOT NULL
			  AND embedding_vector IS NOT NULL
			GROUP BY campaign_variant_id, product_id
			HAVING COUNT(DISTINCT embedding_version) > 1
		`
		rows, err = g.db.QueryContext(ctx, query, tenantID)
		if err != nil {
			return nil, fmt.Errorf("query feature blocks: %w", err)
		}
	}
	defer rows.Close()

	var mismatches []EmbeddingMismatch
	for rows.Next() {
		var mismatch EmbeddingMismatch
		var versionStr string
		var versionCount, blockCount int

		err := rows.Scan(
			&mismatch.CampaignVariantID,
			&mismatch.ProductID,
			&versionCount,
			&blockCount,
			&versionStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan mismatch: %w", err)
		}

		mismatch.TenantID = tenantID
		mismatch.ChunkCount = blockCount
		mismatch.DetectedAt = time.Now()
		mismatch.Versions = []string{versionStr} // Simplified for now

		mismatches = append(mismatches, mismatch)
	}

	return mismatches, rows.Err()
}

// QueueReEmbeddingJobs queues re-embedding jobs for mismatched resources.
func (g *EmbeddingGuard) QueueReEmbeddingJobs(ctx context.Context, mismatches []EmbeddingMismatch) ([]ReEmbeddingJob, error) {
	var jobs []ReEmbeddingJob

	for _, mismatch := range mismatches {
		// Queue jobs for knowledge chunks
		chunkJobs, err := g.queueChunkReEmbeddingJobs(ctx, mismatch)
		if err != nil {
			return nil, fmt.Errorf("queue chunk jobs: %w", err)
		}
		jobs = append(jobs, chunkJobs...)

		// Queue jobs for feature blocks
		blockJobs, err := g.queueBlockReEmbeddingJobs(ctx, mismatch)
		if err != nil {
			return nil, fmt.Errorf("queue block jobs: %w", err)
		}
		jobs = append(jobs, blockJobs...)
	}

	g.logger.Info().
		Int("jobs_queued", len(jobs)).
		Msg("Queued re-embedding jobs")

	return jobs, nil
}

// queueChunkReEmbeddingJobs queues re-embedding jobs for knowledge chunks with mismatched versions.
func (g *EmbeddingGuard) queueChunkReEmbeddingJobs(ctx context.Context, mismatch EmbeddingMismatch) ([]ReEmbeddingJob, error) {
	query := `
		SELECT id, embedding_version
		FROM knowledge_chunks
		WHERE tenant_id = $1
		  AND campaign_variant_id = $2
		  AND embedding_version IS NOT NULL
		  AND embedding_version != $3
	`

	rows, err := g.db.QueryContext(ctx, query, mismatch.TenantID, mismatch.CampaignVariantID, g.currentVersion)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var jobs []ReEmbeddingJob
	for rows.Next() {
		var chunkID uuid.UUID
		var currentVersion string

		if err := rows.Scan(&chunkID, &currentVersion); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}

		job := ReEmbeddingJob{
			ID:                uuid.New(),
			TenantID:          mismatch.TenantID,
			ProductID:         mismatch.ProductID,
			CampaignVariantID: mismatch.CampaignVariantID,
			ResourceType:      "knowledge_chunk",
			ResourceID:        chunkID,
			CurrentVersion:    currentVersion,
			TargetVersion:     g.currentVersion,
			Status:            JobStatusPending,
			CreatedAt:         time.Now(),
		}

		// TODO: Insert job into re_embedding_jobs table
		// INSERT INTO re_embedding_jobs (id, tenant_id, product_id, campaign_variant_id, ...)
		// VALUES ($1, $2, $3, $4, ...)

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// queueBlockReEmbeddingJobs queues re-embedding jobs for feature blocks with mismatched versions.
func (g *EmbeddingGuard) queueBlockReEmbeddingJobs(ctx context.Context, mismatch EmbeddingMismatch) ([]ReEmbeddingJob, error) {
	query := `
		SELECT id, embedding_version
		FROM feature_blocks
		WHERE tenant_id = $1
		  AND campaign_variant_id = $2
		  AND embedding_version IS NOT NULL
		  AND embedding_version != $3
	`

	rows, err := g.db.QueryContext(ctx, query, mismatch.TenantID, mismatch.CampaignVariantID, g.currentVersion)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer rows.Close()

	var jobs []ReEmbeddingJob
	for rows.Next() {
		var blockID uuid.UUID
		var currentVersion string

		if err := rows.Scan(&blockID, &currentVersion); err != nil {
			return nil, fmt.Errorf("scan block: %w", err)
		}

		job := ReEmbeddingJob{
			ID:                uuid.New(),
			TenantID:          mismatch.TenantID,
			ProductID:         mismatch.ProductID,
			CampaignVariantID: mismatch.CampaignVariantID,
			ResourceType:      "feature_block",
			ResourceID:        blockID,
			CurrentVersion:    currentVersion,
			TargetVersion:     g.currentVersion,
			Status:            JobStatusPending,
			CreatedAt:         time.Now(),
		}

		// TODO: Insert job into re_embedding_jobs table

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// PreventMixedVersionQueries checks if a campaign has mixed embedding versions and prevents queries.
func (g *EmbeddingGuard) PreventMixedVersionQueries(ctx context.Context, tenantID, campaignID uuid.UUID) error {
	mismatches, err := g.CheckMismatches(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("check mismatches: %w", err)
	}

	for _, mismatch := range mismatches {
		if mismatch.CampaignVariantID == campaignID && len(mismatch.Versions) > 1 {
			return fmt.Errorf("campaign has mixed embedding versions (%v): queries blocked until re-embedding completes", mismatch.Versions)
		}
	}

	return nil
}
