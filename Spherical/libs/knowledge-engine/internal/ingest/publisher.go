// Package ingest provides the brochure ingestion pipeline for the Knowledge Engine.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

var (
	// ErrConflictsExist indicates unresolved conflicts prevent publishing.
	ErrConflictsExist = errors.New("unresolved conflicts exist")
	// ErrVersionMismatch indicates a version conflict during publish.
	ErrVersionMismatch = errors.New("version mismatch")
	// ErrCampaignNotFound indicates the campaign doesn't exist.
	ErrCampaignNotFound = errors.New("campaign not found")
	// ErrCampaignNotDraft indicates the campaign is not in draft status.
	ErrCampaignNotDraft = errors.New("campaign is not a draft")
)

// Publisher handles campaign publish and rollback operations.
type Publisher struct {
	logger *observability.Logger
}

// PublishRequest represents a request to publish a campaign.
type PublishRequest struct {
	TenantID     uuid.UUID
	CampaignID   uuid.UUID
	Version      int
	ApprovedBy   string
	ReleaseNotes string
}

// PublishResult represents the result of a publish operation.
type PublishResult struct {
	CampaignID       uuid.UUID
	Version          int
	Status           storage.CampaignStatus
	EffectiveFrom    time.Time
	EffectiveThrough *time.Time
	PublishedBy      string
}

// RollbackRequest represents a request to rollback a campaign.
type RollbackRequest struct {
	TenantID      uuid.UUID
	CampaignID    uuid.UUID
	TargetVersion int
	Reason        string
	Operator      string
}

// RollbackResult represents the result of a rollback operation.
type RollbackResult struct {
	CampaignID      uuid.UUID
	PreviousVersion int
	CurrentVersion  int
	Status          storage.CampaignStatus
	RolledBackAt    time.Time
}

// NewPublisher creates a new Publisher.
func NewPublisher(logger *observability.Logger) *Publisher {
	return &Publisher{
		logger: logger,
	}
}

// Publish promotes a draft campaign to published status.
func (p *Publisher) Publish(ctx context.Context, req PublishRequest) (*PublishResult, error) {
	p.logger.Info().
		Str("tenant_id", req.TenantID.String()).
		Str("campaign_id", req.CampaignID.String()).
		Int("version", req.Version).
		Str("approved_by", req.ApprovedBy).
		Msg("Publishing campaign")

	// Step 1: Validate the campaign exists and is a draft
	campaign, err := p.getCampaign(ctx, req.TenantID, req.CampaignID)
	if err != nil {
		return nil, err
	}

	if campaign.Status != storage.CampaignStatusDraft {
		return nil, fmt.Errorf("%w: current status is %s", ErrCampaignNotDraft, campaign.Status)
	}

	// Step 2: Check for unresolved conflicts
	conflicts, err := p.checkConflicts(ctx, req.TenantID, req.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("check conflicts: %w", err)
	}

	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%w: %d unresolved conflicts", ErrConflictsExist, len(conflicts))
	}

	// Step 3: Verify version matches
	if req.Version > 0 && campaign.Version != req.Version {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrVersionMismatch, req.Version, campaign.Version)
	}

	// Step 4: Update the campaign status
	now := time.Now()
	newVersion := campaign.Version + 1

	// Close out the previous published version if one exists
	if err := p.archivePreviousVersion(ctx, req.TenantID, campaign.ProductID, req.CampaignID); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to archive previous version")
	}

	// Update the campaign
	campaign.Status = storage.CampaignStatusPublished
	campaign.Version = newVersion
	campaign.EffectiveFrom = &now
	campaign.IsDraft = false
	campaign.LastPublishedBy = &req.ApprovedBy

	// TODO: Persist the updated campaign to database

	// Step 5: Refresh materialized views
	if err := p.refreshMaterializedViews(ctx, req.TenantID); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to refresh materialized views")
	}

	// Step 6: Emit audit event
	if err := p.emitPublishEvent(ctx, req, newVersion); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to emit publish event")
	}

	// Step 7: Invalidate caches
	if err := p.invalidateCaches(ctx, req.TenantID, req.CampaignID); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to invalidate caches")
	}

	p.logger.Info().
		Str("campaign_id", req.CampaignID.String()).
		Int("new_version", newVersion).
		Msg("Campaign published successfully")

	return &PublishResult{
		CampaignID:    req.CampaignID,
		Version:       newVersion,
		Status:        storage.CampaignStatusPublished,
		EffectiveFrom: now,
		PublishedBy:   req.ApprovedBy,
	}, nil
}

// Rollback reverts a campaign to a previous version.
func (p *Publisher) Rollback(ctx context.Context, req RollbackRequest) (*RollbackResult, error) {
	p.logger.Info().
		Str("tenant_id", req.TenantID.String()).
		Str("campaign_id", req.CampaignID.String()).
		Int("target_version", req.TargetVersion).
		Str("reason", req.Reason).
		Msg("Rolling back campaign")

	// Step 1: Get current campaign
	campaign, err := p.getCampaign(ctx, req.TenantID, req.CampaignID)
	if err != nil {
		return nil, err
	}

	previousVersion := campaign.Version

	// Step 2: Verify target version exists
	targetCampaign, err := p.getCampaignVersion(ctx, req.TenantID, req.CampaignID, req.TargetVersion)
	if err != nil {
		return nil, fmt.Errorf("target version not found: %w", err)
	}

	// Step 3: Archive current version
	now := time.Now()
	campaign.Status = storage.CampaignStatusArchived
	campaign.EffectiveThrough = &now

	// TODO: Persist the archived campaign

	// Step 4: Restore target version
	targetCampaign.Status = storage.CampaignStatusPublished
	targetCampaign.EffectiveFrom = &now
	targetCampaign.EffectiveThrough = nil

	// TODO: Persist the restored campaign

	// Step 5: Emit audit event
	if err := p.emitRollbackEvent(ctx, req, previousVersion); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to emit rollback event")
	}

	// Step 6: Invalidate caches
	if err := p.invalidateCaches(ctx, req.TenantID, req.CampaignID); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to invalidate caches")
	}

	// Step 7: Refresh materialized views
	if err := p.refreshMaterializedViews(ctx, req.TenantID); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to refresh materialized views")
	}

	p.logger.Info().
		Str("campaign_id", req.CampaignID.String()).
		Int("from_version", previousVersion).
		Int("to_version", req.TargetVersion).
		Msg("Campaign rolled back successfully")

	return &RollbackResult{
		CampaignID:      req.CampaignID,
		PreviousVersion: previousVersion,
		CurrentVersion:  req.TargetVersion,
		Status:          storage.CampaignStatusPublished,
		RolledBackAt:    now,
	}, nil
}

// getCampaign retrieves a campaign by ID.
func (p *Publisher) getCampaign(_ context.Context, tenantID, campaignID uuid.UUID) (*storage.CampaignVariant, error) {
	// TODO: Implement actual database query
	// For now, return a placeholder

	return &storage.CampaignVariant{
		ID:       campaignID,
		TenantID: tenantID,
		Status:   storage.CampaignStatusDraft,
		Version:  1,
		IsDraft:  true,
	}, nil
}

// getCampaignVersion retrieves a specific version of a campaign.
func (p *Publisher) getCampaignVersion(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ int) (*storage.CampaignVariant, error) {
	// TODO: Implement actual database query
	return nil, ErrCampaignNotFound
}

// checkConflicts returns any unresolved spec conflicts for the campaign.
func (p *Publisher) checkConflicts(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]uuid.UUID, error) {
	// TODO: Query spec_values where status = 'conflict' for this campaign
	return nil, nil
}

// archivePreviousVersion marks the previous published version as archived.
func (p *Publisher) archivePreviousVersion(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) error {
	// TODO: Update previous published campaign variants for this product
	// SET status = 'archived', effective_through = NOW()
	// WHERE product_id = $1 AND status = 'published' AND id != $2
	return nil
}

// refreshMaterializedViews updates the spec_view_latest view.
func (p *Publisher) refreshMaterializedViews(_ context.Context, _ uuid.UUID) error {
	// TODO: Execute REFRESH MATERIALIZED VIEW spec_view_latest
	return nil
}

// emitPublishEvent records a publish audit event.
func (p *Publisher) emitPublishEvent(_ context.Context, _ PublishRequest, _ int) error {
	// TODO: Insert into lineage_events
	return nil
}

// emitRollbackEvent records a rollback audit event.
func (p *Publisher) emitRollbackEvent(_ context.Context, _ RollbackRequest, _ int) error {
	// TODO: Insert into lineage_events
	return nil
}

// invalidateCaches clears cached data for the campaign.
func (p *Publisher) invalidateCaches(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	// TODO: Delete cache entries for this campaign
	return nil
}

// ResolveConflict marks a spec value conflict as resolved.
func (p *Publisher) ResolveConflict(_ context.Context, _ uuid.UUID, specValueID uuid.UUID, keepValueID uuid.UUID) error {
	p.logger.Info().
		Str("spec_value_id", specValueID.String()).
		Str("keep_value_id", keepValueID.String()).
		Msg("Resolving spec value conflict")

	// TODO: Implement conflict resolution
	// 1. Mark the "kept" value as active
	// 2. Mark the conflicting value as deprecated
	// 3. Emit lineage event

	return nil
}

// ValidateForPublish checks if a campaign is ready to be published.
func (p *Publisher) ValidateForPublish(ctx context.Context, tenantID, campaignID uuid.UUID) ([]string, error) {
	var issues []string

	// Check for conflicts
	conflicts, err := p.checkConflicts(ctx, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		issues = append(issues, fmt.Sprintf("%d unresolved conflicts", len(conflicts)))
	}

	// Check for minimum content
	// TODO: Query for minimum spec values, features, etc.
	// tenantID and campaignID will be used when implementing minimum content check

	// Check for required fields
	// TODO: Verify required metadata is present
	// campaignID will be used when implementing required fields check

	return issues, nil
}

