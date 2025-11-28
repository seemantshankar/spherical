// Package monitoring provides lineage writing for audit trail.
package monitoring

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// LineageWriter captures and persists lineage events.
type LineageWriter struct {
	logger   *observability.Logger
	store    LineageStore
	buffer   chan *LineageEvent
	config   LineageConfig
	stopCh   chan struct{}
}

// LineageStore persists lineage events.
type LineageStore interface {
	SaveLineageEvent(ctx context.Context, event *storage.LineageEvent) error
	BatchSaveLineageEvents(ctx context.Context, events []storage.LineageEvent) error
}

// LineageConfig configures the lineage writer.
type LineageConfig struct {
	BufferSize     int
	FlushInterval  time.Duration
	EnableAsync    bool
	IncludePayload bool
}

// DefaultLineageConfig returns default lineage configuration.
func DefaultLineageConfig() LineageConfig {
	return LineageConfig{
		BufferSize:     1000,
		FlushInterval:  5 * time.Second,
		EnableAsync:    true,
		IncludePayload: true,
	}
}

// LineageEvent represents an event to be recorded.
type LineageEvent struct {
	TenantID          uuid.UUID
	ProductID         *uuid.UUID
	CampaignVariantID *uuid.UUID
	ResourceType      string
	ResourceID        uuid.UUID
	DocumentSourceID  *uuid.UUID
	IngestionJobID    *uuid.UUID
	Action            storage.LineageAction
	Payload           map[string]interface{}
	OccurredAt        time.Time
	Operator          string
}

// NewLineageWriter creates a new lineage writer.
func NewLineageWriter(logger *observability.Logger, store LineageStore, config LineageConfig) *LineageWriter {
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}

	w := &LineageWriter{
		logger: logger,
		store:  store,
		buffer: make(chan *LineageEvent, config.BufferSize),
		config: config,
		stopCh: make(chan struct{}),
	}

	if config.EnableAsync {
		go w.runFlushLoop()
	}

	return w
}

// RecordCreation records a resource creation event.
func (w *LineageWriter) RecordCreation(ctx context.Context, event LineageEvent) error {
	event.Action = storage.LineageActionCreated
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	return w.record(ctx, &event)
}

// RecordUpdate records a resource update event.
func (w *LineageWriter) RecordUpdate(ctx context.Context, event LineageEvent) error {
	event.Action = storage.LineageActionUpdated
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	return w.record(ctx, &event)
}

// RecordDeletion records a resource deletion event.
func (w *LineageWriter) RecordDeletion(ctx context.Context, event LineageEvent) error {
	event.Action = storage.LineageActionDeleted
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	return w.record(ctx, &event)
}

// RecordReconciliation records a reconciliation event.
func (w *LineageWriter) RecordReconciliation(ctx context.Context, event LineageEvent) error {
	event.Action = storage.LineageActionReconciled
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	return w.record(ctx, &event)
}

// RecordIngestionStart records the start of an ingestion job.
func (w *LineageWriter) RecordIngestionStart(ctx context.Context, tenantID, productID, campaignID, jobID uuid.UUID, operator string) error {
	return w.RecordCreation(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "ingestion_job",
		ResourceID:        jobID,
		IngestionJobID:    &jobID,
		Operator:          operator,
		Payload: map[string]interface{}{
			"status": "started",
		},
	})
}

// RecordIngestionComplete records the completion of an ingestion job.
func (w *LineageWriter) RecordIngestionComplete(ctx context.Context, tenantID, productID, campaignID, jobID uuid.UUID, stats map[string]int) error {
	return w.RecordUpdate(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "ingestion_job",
		ResourceID:        jobID,
		IngestionJobID:    &jobID,
		Payload: map[string]interface{}{
			"status": "completed",
			"stats":  stats,
		},
	})
}

// RecordSpecCreation records spec value creation.
func (w *LineageWriter) RecordSpecCreation(ctx context.Context, tenantID, productID, campaignID, specID uuid.UUID, docSourceID *uuid.UUID, jobID *uuid.UUID) error {
	return w.RecordCreation(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "spec_value",
		ResourceID:        specID,
		DocumentSourceID:  docSourceID,
		IngestionJobID:    jobID,
	})
}

// RecordFeatureCreation records feature block creation.
func (w *LineageWriter) RecordFeatureCreation(ctx context.Context, tenantID, productID, campaignID, featureID uuid.UUID, blockType string, docSourceID *uuid.UUID) error {
	return w.RecordCreation(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "feature_block",
		ResourceID:        featureID,
		DocumentSourceID:  docSourceID,
		Payload: map[string]interface{}{
			"block_type": blockType,
		},
	})
}

// RecordChunkCreation records knowledge chunk creation.
func (w *LineageWriter) RecordChunkCreation(ctx context.Context, tenantID, productID uuid.UUID, campaignID *uuid.UUID, chunkID uuid.UUID, chunkType string, docSourceID *uuid.UUID) error {
	return w.RecordCreation(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: campaignID,
		ResourceType:      "knowledge_chunk",
		ResourceID:        chunkID,
		DocumentSourceID:  docSourceID,
		Payload: map[string]interface{}{
			"chunk_type": chunkType,
		},
	})
}

// RecordPublish records a campaign publish event.
func (w *LineageWriter) RecordPublish(ctx context.Context, tenantID, productID, campaignID uuid.UUID, version int, approvedBy string) error {
	return w.RecordUpdate(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "campaign_variant",
		ResourceID:        campaignID,
		Operator:          approvedBy,
		Payload: map[string]interface{}{
			"action":  "publish",
			"version": version,
		},
	})
}

// RecordRollback records a campaign rollback event.
func (w *LineageWriter) RecordRollback(ctx context.Context, tenantID, productID, campaignID uuid.UUID, fromVersion, toVersion int, operator string) error {
	return w.RecordUpdate(ctx, LineageEvent{
		TenantID:          tenantID,
		ProductID:         &productID,
		CampaignVariantID: &campaignID,
		ResourceType:      "campaign_variant",
		ResourceID:        campaignID,
		Operator:          operator,
		Payload: map[string]interface{}{
			"action":       "rollback",
			"from_version": fromVersion,
			"to_version":   toVersion,
		},
	})
}

// RecordRetrievalRequest records a retrieval API request.
func (w *LineageWriter) RecordRetrievalRequest(ctx context.Context, tenantID uuid.UUID, productIDs []uuid.UUID, question, intent string, resultCount int) error {
	productIDStrs := make([]string, len(productIDs))
	for i, id := range productIDs {
		productIDStrs[i] = id.String()
	}

	return w.RecordCreation(ctx, LineageEvent{
		TenantID:     tenantID,
		ResourceType: "retrieval_request",
		ResourceID:   uuid.New(),
		Payload: map[string]interface{}{
			"product_ids":  productIDStrs,
			"question":     question,
			"intent":       intent,
			"result_count": resultCount,
		},
	})
}

// RecordComparisonRequest records a comparison API request.
func (w *LineageWriter) RecordComparisonRequest(ctx context.Context, tenantID, primaryID, secondaryID uuid.UUID, resultCount int) error {
	return w.RecordCreation(ctx, LineageEvent{
		TenantID:     tenantID,
		ResourceType: "comparison_request",
		ResourceID:   uuid.New(),
		Payload: map[string]interface{}{
			"primary_product_id":   primaryID.String(),
			"secondary_product_id": secondaryID.String(),
			"result_count":         resultCount,
		},
	})
}

// record sends an event for recording.
func (w *LineageWriter) record(ctx context.Context, event *LineageEvent) error {
	if w.config.EnableAsync {
		select {
		case w.buffer <- event:
			return nil
		default:
			// Buffer full, log warning and write synchronously
			w.logger.Warn().Msg("Lineage buffer full, writing synchronously")
			return w.writeEvent(ctx, event)
		}
	}
	return w.writeEvent(ctx, event)
}

// writeEvent persists an event to storage.
func (w *LineageWriter) writeEvent(ctx context.Context, event *LineageEvent) error {
	if w.store == nil {
		// Log only mode
		w.logger.Info().
			Str("resource_type", event.ResourceType).
			Str("resource_id", event.ResourceID.String()).
			Str("action", string(event.Action)).
			Msg("Lineage event (no store)")
		return nil
	}

	var payloadJSON json.RawMessage
	if event.Payload != nil && w.config.IncludePayload {
		payloadJSON, _ = json.Marshal(event.Payload)
	}

	storageEvent := &storage.LineageEvent{
		ID:                uuid.New(),
		TenantID:          event.TenantID,
		ProductID:         event.ProductID,
		CampaignVariantID: event.CampaignVariantID,
		ResourceType:      event.ResourceType,
		ResourceID:        event.ResourceID,
		DocumentSourceID:  event.DocumentSourceID,
		IngestionJobID:    event.IngestionJobID,
		Action:            event.Action,
		Payload:           payloadJSON,
		OccurredAt:        event.OccurredAt,
	}

	return w.store.SaveLineageEvent(ctx, storageEvent)
}

// runFlushLoop periodically flushes buffered events.
func (w *LineageWriter) runFlushLoop() {
	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	var batch []*LineageEvent

	for {
		select {
		case event := <-w.buffer:
			batch = append(batch, event)
			if len(batch) >= 100 {
				w.flushBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flushBatch(batch)
				batch = nil
			}
		case <-w.stopCh:
			// Flush remaining events
			if len(batch) > 0 {
				w.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch writes a batch of events.
func (w *LineageWriter) flushBatch(batch []*LineageEvent) {
	if w.store == nil {
		for _, event := range batch {
			w.logger.Info().
				Str("resource_type", event.ResourceType).
				Str("resource_id", event.ResourceID.String()).
				Str("action", string(event.Action)).
				Msg("Lineage event (batch, no store)")
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	storageEvents := make([]storage.LineageEvent, len(batch))
	for i, event := range batch {
		var payloadJSON json.RawMessage
		if event.Payload != nil && w.config.IncludePayload {
			payloadJSON, _ = json.Marshal(event.Payload)
		}

		storageEvents[i] = storage.LineageEvent{
			ID:                uuid.New(),
			TenantID:          event.TenantID,
			ProductID:         event.ProductID,
			CampaignVariantID: event.CampaignVariantID,
			ResourceType:      event.ResourceType,
			ResourceID:        event.ResourceID,
			DocumentSourceID:  event.DocumentSourceID,
			IngestionJobID:    event.IngestionJobID,
			Action:            event.Action,
			Payload:           payloadJSON,
			OccurredAt:        event.OccurredAt,
		}
	}

	if err := w.store.BatchSaveLineageEvents(ctx, storageEvents); err != nil {
		w.logger.Error().Err(err).Int("count", len(batch)).Msg("Failed to flush lineage batch")
	} else {
		w.logger.Debug().Int("count", len(batch)).Msg("Flushed lineage batch")
	}
}

// Stop stops the lineage writer.
func (w *LineageWriter) Stop() {
	close(w.stopCh)
}

