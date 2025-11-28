// Package monitoring provides audit logging, drift detection, and lineage tracking.
package monitoring

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// AuditLogger handles audit event logging and lineage tracking.
type AuditLogger struct {
	logger      *observability.Logger
	redisClient *cache.RedisClient
}

// AuditEvent represents an auditable action.
type AuditEvent struct {
	ID                uuid.UUID              `json:"id"`
	TenantID          uuid.UUID              `json:"tenant_id"`
	ProductID         *uuid.UUID             `json:"product_id,omitempty"`
	CampaignVariantID *uuid.UUID             `json:"campaign_variant_id,omitempty"`
	ResourceType      string                 `json:"resource_type"`
	ResourceID        uuid.UUID              `json:"resource_id"`
	Action            storage.LineageAction  `json:"action"`
	Operator          string                 `json:"operator"`
	Payload           map[string]interface{} `json:"payload,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	OccurredAt        time.Time              `json:"occurred_at"`
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(logger *observability.Logger, redisClient *cache.RedisClient) *AuditLogger {
	return &AuditLogger{
		logger:      logger,
		redisClient: redisClient,
	}
}

// LogEvent records an audit event.
func (a *AuditLogger) LogEvent(ctx context.Context, event AuditEvent) error {
	// Set defaults
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}

	// Log to structured logger
	a.logger.Info().
		Str("event_id", event.ID.String()).
		Str("tenant_id", event.TenantID.String()).
		Str("resource_type", event.ResourceType).
		Str("resource_id", event.ResourceID.String()).
		Str("action", string(event.Action)).
		Str("operator", event.Operator).
		Msg("Audit event")

	// TODO: Persist to lineage_events table
	lineageEvent := &storage.LineageEvent{
		ID:           event.ID,
		TenantID:     event.TenantID,
		ProductID:    event.ProductID,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Action:       event.Action,
		OccurredAt:   event.OccurredAt,
	}

	if event.Payload != nil {
		payload, _ := json.Marshal(event.Payload)
		lineageEvent.Payload = payload
	}

	_ = lineageEvent // TODO: Save to database

	return nil
}

// LogIngestion records an ingestion event.
func (a *AuditLogger) LogIngestion(ctx context.Context, tenantID, productID, campaignID uuid.UUID, operator string, result map[string]interface{}) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "campaign_variant",
		ResourceID:        campaignID,
		Action:            storage.LineageActionUpdated,
		Operator:          operator,
		Payload:           result,
	})
}

// LogPublish records a publish event.
func (a *AuditLogger) LogPublish(ctx context.Context, tenantID, campaignID uuid.UUID, operator string, version int) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:          tenantID,
		CampaignVariantID: &campaignID,
		ResourceType:      "campaign_variant",
		ResourceID:        campaignID,
		Action:            storage.LineageActionUpdated,
		Operator:          operator,
		Payload: map[string]interface{}{
			"action":  "publish",
			"version": version,
		},
	})
}

// LogRollback records a rollback event.
func (a *AuditLogger) LogRollback(ctx context.Context, tenantID, campaignID uuid.UUID, operator string, fromVersion, toVersion int, reason string) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:          tenantID,
		CampaignVariantID: &campaignID,
		ResourceType:      "campaign_variant",
		ResourceID:        campaignID,
		Action:            storage.LineageActionUpdated,
		Operator:          operator,
		Payload: map[string]interface{}{
			"action":       "rollback",
			"from_version": fromVersion,
			"to_version":   toVersion,
			"reason":       reason,
		},
	})
}

// LogRetrieval records a retrieval query event.
func (a *AuditLogger) LogRetrieval(ctx context.Context, tenantID uuid.UUID, productIDs []uuid.UUID, question string, intent string, latencyMs int64, resultCount int) error {
	productIDStrings := make([]string, len(productIDs))
	for i, pid := range productIDs {
		productIDStrings[i] = pid.String()
	}

	return a.LogEvent(ctx, AuditEvent{
		TenantID:     tenantID,
		ResourceType: "retrieval_query",
		ResourceID:   uuid.New(), // Each query gets a unique ID
		Action:       storage.LineageActionCreated,
		Operator:     "agent", // Could be extracted from auth context
		Payload: map[string]interface{}{
			"question":     question,
			"intent":       intent,
			"product_ids":  productIDStrings,
			"latency_ms":   latencyMs,
			"result_count": resultCount,
		},
	})
}

// LogComparison records a comparison query event.
func (a *AuditLogger) LogComparison(ctx context.Context, tenantID, primaryProductID, secondaryProductID uuid.UUID, dimensions []string, resultCount int) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:     tenantID,
		ProductID:    &primaryProductID,
		ResourceType: "comparison_query",
		ResourceID:   uuid.New(),
		Action:       storage.LineageActionCreated,
		Operator:     "agent",
		Payload: map[string]interface{}{
			"primary_product_id":   primaryProductID.String(),
			"secondary_product_id": secondaryProductID.String(),
			"dimensions":           dimensions,
			"result_count":         resultCount,
		},
	})
}

// LogSpecConflict records a spec value conflict event.
func (a *AuditLogger) LogSpecConflict(ctx context.Context, tenantID, productID, campaignID, specValueID uuid.UUID, existingValue, newValue string) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "spec_value",
		ResourceID:        specValueID,
		Action:            storage.LineageActionUpdated,
		Operator:          "system",
		Payload: map[string]interface{}{
			"conflict_type":  "value_mismatch",
			"existing_value": existingValue,
			"new_value":      newValue,
		},
	})
}

// LogConflictResolution records a conflict resolution event.
func (a *AuditLogger) LogConflictResolution(ctx context.Context, tenantID, specValueID uuid.UUID, operator string, resolution string, keptValue string) error {
	return a.LogEvent(ctx, AuditEvent{
		TenantID:     tenantID,
		ResourceType: "spec_value",
		ResourceID:   specValueID,
		Action:       storage.LineageActionReconciled,
		Operator:     operator,
		Payload: map[string]interface{}{
			"resolution": resolution,
			"kept_value": keptValue,
		},
	})
}

// PublishDriftAlert publishes a drift alert to Redis for real-time notification.
func (a *AuditLogger) PublishDriftAlert(ctx context.Context, alert *storage.DriftAlert) error {
	if a.redisClient == nil {
		return nil
	}

	return a.redisClient.Publish(ctx, "drift.alerts", alert)
}

// QueryLineage retrieves lineage events for a resource.
func (a *AuditLogger) QueryLineage(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) ([]storage.LineageEvent, error) {
	// TODO: Query lineage_events table
	// SELECT * FROM lineage_events
	// WHERE tenant_id = $1 AND resource_type = $2 AND resource_id = $3
	// ORDER BY occurred_at DESC

	return nil, nil
}

// QueryAuditTrail retrieves audit events for a tenant within a time range.
func (a *AuditLogger) QueryAuditTrail(ctx context.Context, tenantID uuid.UUID, from, to time.Time, limit int) ([]storage.LineageEvent, error) {
	// TODO: Query lineage_events table
	// SELECT * FROM lineage_events
	// WHERE tenant_id = $1 AND occurred_at BETWEEN $2 AND $3
	// ORDER BY occurred_at DESC
	// LIMIT $4

	return nil, nil
}

