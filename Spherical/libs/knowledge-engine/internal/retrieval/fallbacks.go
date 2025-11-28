// Package retrieval provides fallback handling for edge cases.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// Common fallback errors.
var (
	ErrCampaignDeleted     = errors.New("campaign has been deleted")
	ErrCampaignNotFound    = errors.New("campaign not found")
	ErrTrimMismatch        = errors.New("requested trim not available")
	ErrNoPublishedVersion  = errors.New("no published version available")
	ErrProductNotAccessible = errors.New("product not accessible")
)

// FallbackHandler manages edge case handling for retrieval.
type FallbackHandler struct {
	logger       *observability.Logger
	campaignRepo CampaignRepository
	config       FallbackConfig
}

// CampaignRepository provides campaign data access.
type CampaignRepository interface {
	GetCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) (*storage.CampaignVariant, error)
	GetLatestPublishedCampaign(ctx context.Context, tenantID, productID uuid.UUID) (*storage.CampaignVariant, error)
	GetCampaignByTrim(ctx context.Context, tenantID, productID uuid.UUID, trim, locale string) (*storage.CampaignVariant, error)
}

// FallbackConfig configures fallback behavior.
type FallbackConfig struct {
	// EnableDeletedCampaignFallback falls back to last published version when campaign is deleted
	EnableDeletedCampaignFallback bool
	// EnableTrimFallback falls back to default trim when requested trim not found
	EnableTrimFallback bool
	// DefaultResponseMessage message when no data available
	DefaultResponseMessage string
	// MaxFallbackAttempts limits fallback chain depth
	MaxFallbackAttempts int
	// FallbackCacheTTL how long to cache fallback decisions
	FallbackCacheTTL time.Duration
}

// DefaultFallbackConfig returns default fallback configuration.
func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		EnableDeletedCampaignFallback: true,
		EnableTrimFallback:            true,
		DefaultResponseMessage:        "The requested information is currently unavailable.",
		MaxFallbackAttempts:           3,
		FallbackCacheTTL:              5 * time.Minute,
	}
}

// NewFallbackHandler creates a new fallback handler.
func NewFallbackHandler(logger *observability.Logger, campaignRepo CampaignRepository, config FallbackConfig) *FallbackHandler {
	return &FallbackHandler{
		logger:       logger,
		campaignRepo: campaignRepo,
		config:       config,
	}
}

// FallbackResult contains the result of fallback resolution.
type FallbackResult struct {
	// OriginalCampaignID the originally requested campaign
	OriginalCampaignID uuid.UUID
	// ResolvedCampaignID the campaign to actually use
	ResolvedCampaignID uuid.UUID
	// FallbackApplied whether any fallback was used
	FallbackApplied bool
	// FallbackReason why fallback was applied
	FallbackReason string
	// PolicyMessage user-facing message about the fallback
	PolicyMessage string
	// ShouldReturnEmpty whether to return empty results
	ShouldReturnEmpty bool
}

// ResolveCampaign attempts to resolve a campaign with fallback handling.
func (h *FallbackHandler) ResolveCampaign(ctx context.Context, tenantID, productID uuid.UUID, campaignID *uuid.UUID, trim, locale string) (*FallbackResult, error) {
	result := &FallbackResult{}

	// If specific campaign requested, try to get it
	if campaignID != nil {
		result.OriginalCampaignID = *campaignID

		campaign, err := h.campaignRepo.GetCampaign(ctx, tenantID, *campaignID)
		if err == nil && campaign != nil {
			// Check if campaign is deleted/archived
			if campaign.Status == storage.CampaignStatusArchived {
				h.logger.Warn().
					Str("campaign_id", campaignID.String()).
					Msg("Requested campaign is archived, attempting fallback")

				if h.config.EnableDeletedCampaignFallback {
					return h.fallbackToLatestPublished(ctx, tenantID, productID, result)
				}

				result.ShouldReturnEmpty = true
				result.FallbackReason = "campaign_archived"
				result.PolicyMessage = "The requested campaign is no longer available."
				return result, nil
			}

			// Campaign found and active
			result.ResolvedCampaignID = campaign.ID
			return result, nil
		}

		// Campaign not found, attempt fallback
		h.logger.Warn().
			Str("campaign_id", campaignID.String()).
			Msg("Campaign not found, attempting fallback")

		if h.config.EnableDeletedCampaignFallback {
			return h.fallbackToLatestPublished(ctx, tenantID, productID, result)
		}

		result.ShouldReturnEmpty = true
		result.FallbackReason = "campaign_not_found"
		result.PolicyMessage = h.config.DefaultResponseMessage
		return result, nil
	}

	// No specific campaign, try trim-specific lookup
	if trim != "" && h.campaignRepo != nil {
		campaign, err := h.campaignRepo.GetCampaignByTrim(ctx, tenantID, productID, trim, locale)
		if err == nil && campaign != nil {
			result.ResolvedCampaignID = campaign.ID
			return result, nil
		}

		// Trim not found, fallback to default
		if h.config.EnableTrimFallback {
			h.logger.Info().
				Str("trim", trim).
				Str("locale", locale).
				Msg("Requested trim not found, falling back to default")

			result.FallbackApplied = true
			result.FallbackReason = "trim_not_found"
			result.PolicyMessage = fmt.Sprintf("Data for %s trim not available, showing default.", trim)
		}
	}

	// Fall back to latest published campaign
	return h.fallbackToLatestPublished(ctx, tenantID, productID, result)
}

// fallbackToLatestPublished attempts to resolve to the latest published campaign.
func (h *FallbackHandler) fallbackToLatestPublished(ctx context.Context, tenantID, productID uuid.UUID, result *FallbackResult) (*FallbackResult, error) {
	if h.campaignRepo == nil {
		result.ShouldReturnEmpty = true
		result.FallbackReason = "no_repository"
		result.PolicyMessage = h.config.DefaultResponseMessage
		return result, nil
	}

	campaign, err := h.campaignRepo.GetLatestPublishedCampaign(ctx, tenantID, productID)
	if err != nil || campaign == nil {
		h.logger.Warn().
			Str("product_id", productID.String()).
			Msg("No published campaign found for product")

		result.ShouldReturnEmpty = true
		result.FallbackReason = "no_published_version"
		result.PolicyMessage = h.config.DefaultResponseMessage
		return result, nil
	}

	result.ResolvedCampaignID = campaign.ID
	result.FallbackApplied = true
	if result.FallbackReason == "" {
		result.FallbackReason = "latest_published"
	}

	return result, nil
}

// HandleDeletedCampaign handles the case when a referenced campaign is deleted mid-conversation.
func (h *FallbackHandler) HandleDeletedCampaign(ctx context.Context, tenantID, productID, deletedCampaignID uuid.UUID) (*FallbackResult, error) {
	h.logger.Info().
		Str("deleted_campaign_id", deletedCampaignID.String()).
		Msg("Handling deleted campaign scenario")

	result := &FallbackResult{
		OriginalCampaignID: deletedCampaignID,
		FallbackApplied:    true,
		FallbackReason:     "campaign_deleted_mid_conversation",
	}

	if !h.config.EnableDeletedCampaignFallback {
		result.ShouldReturnEmpty = true
		result.PolicyMessage = "The campaign you were viewing has been removed."
		return result, nil
	}

	return h.fallbackToLatestPublished(ctx, tenantID, productID, result)
}

// HandleEmbeddingVersionMismatch handles queries against campaigns with mixed embedding versions.
func (h *FallbackHandler) HandleEmbeddingVersionMismatch(ctx context.Context, tenantID, campaignID uuid.UUID, versions []string) (*FallbackResult, error) {
	h.logger.Warn().
		Str("campaign_id", campaignID.String()).
		Strs("embedding_versions", versions).
		Msg("Mixed embedding versions detected")

	// For mixed versions, we should filter to only query chunks with the latest version
	// or return a warning to the user
	result := &FallbackResult{
		OriginalCampaignID: campaignID,
		ResolvedCampaignID: campaignID,
		FallbackApplied:    true,
		FallbackReason:     "embedding_version_mismatch",
		PolicyMessage:      "Some data may be from an older index version.",
	}

	return result, nil
}

// CreateSafeResponse creates a policy-compliant response when fallback is needed.
func (h *FallbackHandler) CreateSafeResponse(result *FallbackResult) *RetrievalResponse {
	if result.ShouldReturnEmpty {
		return &RetrievalResponse{
			Intent:          IntentUnknown,
			LatencyMs:       0,
			StructuredFacts: []SpecFact{},
			SemanticChunks:  []SemanticChunk{},
			Lineage:         []LineageInfo{},
		}
	}

	return nil // No safe response needed, proceed normally
}

// ValidateProductAccess checks if the requested products are accessible.
func (h *FallbackHandler) ValidateProductAccess(ctx context.Context, tenantID uuid.UUID, productIDs []uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	var accessible, inaccessible []uuid.UUID

	for _, productID := range productIDs {
		// Check if product belongs to tenant or is public benchmark
		// For now, assume all products are accessible if in the request
		// TODO: Implement actual access check against product repository
		accessible = append(accessible, productID)
	}

	if len(inaccessible) > 0 {
		h.logger.Warn().
			Int("inaccessible_count", len(inaccessible)).
			Msg("Some requested products are not accessible")
	}

	return accessible, inaccessible, nil
}

// FallbackLineageInfo contains fallback-specific lineage information.
type FallbackLineageInfo struct {
	DocumentSourceID *uuid.UUID `json:"document_source_id,omitempty"`
	Page             *int       `json:"page,omitempty"`
	URL              *string    `json:"url,omitempty"`
	FallbackApplied  bool       `json:"fallback_applied,omitempty"`
	FallbackReason   string     `json:"fallback_reason,omitempty"`
	PolicyMessage    string     `json:"policy_message,omitempty"`
}

