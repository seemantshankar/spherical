// Package handlers provides HTTP handlers for the Knowledge Engine API.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

// LineageHandler handles lineage query requests.
type LineageHandler struct {
	logger      *observability.Logger
	auditLogger *monitoring.AuditLogger
}

// NewLineageHandler creates a new lineage handler.
func NewLineageHandler(logger *observability.Logger, auditLogger *monitoring.AuditLogger) *LineageHandler {
	return &LineageHandler{
		logger:      logger,
		auditLogger: auditLogger,
	}
}

// LineageResponseDTO represents the API response for lineage.
type LineageResponseDTO struct {
	ResourceID string             `json:"resourceId"`
	Events     []LineageEventDTO  `json:"events"`
}

// LineageEventDTO represents a lineage event.
type LineageEventDTO struct {
	ID               string                 `json:"id"`
	ResourceType     string                 `json:"resourceType"`
	ResourceID       string                 `json:"resourceId"`
	Action           string                 `json:"action"`
	Operator         string                 `json:"operator,omitempty"`
	OccurredAt       string                 `json:"occurredAt"`
	DocumentSourceID string                 `json:"documentSourceId,omitempty"`
	IngestionJobID   string                 `json:"ingestionJobId,omitempty"`
	Diff             map[string]interface{} `json:"diff,omitempty"`
}

// GetLineage handles GET /lineage/{resourceType}/{resourceId}.
func (h *LineageHandler) GetLineage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse path parameters
	resourceType := chi.URLParam(r, "resourceType")
	resourceIDStr := chi.URLParam(r, "resourceId")

	// Validate resource type
	validTypes := map[string]bool{
		"spec_value":      true,
		"feature_block":   true,
		"knowledge_chunk": true,
		"comparison":      true,
	}
	if !validTypes[resourceType] {
		h.writeError(w, http.StatusBadRequest, "invalid resourceType", 
			"Must be one of: spec_value, feature_block, knowledge_chunk, comparison")
		return
	}

	resourceID, err := uuid.Parse(resourceIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid resourceId", err.Error())
		return
	}

	// Get tenant from context or query param
	tenantIDStr := r.URL.Query().Get("tenantId")
	if tenantIDStr == "" {
		tenantIDStr = middleware.TenantFromContext(ctx)
	}
	if tenantIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "tenantId is required", "")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	h.logger.Info().
		Str("tenant_id", tenantIDStr).
		Str("resource_type", resourceType).
		Str("resource_id", resourceIDStr).
		Msg("Querying lineage")

	// Query lineage events
	events, err := h.auditLogger.QueryLineage(ctx, tenantID, resourceType, resourceID)
	if err != nil {
		h.logger.Error().Err(err).Msg("Lineage query failed")
		h.writeError(w, http.StatusInternalServerError, "lineage query failed", err.Error())
		return
	}

	// Convert to DTO
	resp := LineageResponseDTO{
		ResourceID: resourceIDStr,
		Events:     make([]LineageEventDTO, 0),
	}

	// Convert events to DTOs
	if events != nil {
		for _, event := range events {
			dto := LineageEventDTO{
				ID:           event.ID.String(),
				ResourceType: event.ResourceType,
				ResourceID:   event.ResourceID.String(),
				Action:       string(event.Action),
				OccurredAt:   event.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
			}

			// Set optional fields
			if event.DocumentSourceID != nil {
				dto.DocumentSourceID = event.DocumentSourceID.String()
			}
			if event.IngestionJobID != nil {
				dto.IngestionJobID = event.IngestionJobID.String()
			}

			// Parse payload to extract operator and diff if present
			if len(event.Payload) > 0 {
				var payload map[string]interface{}
				if err := json.Unmarshal(event.Payload, &payload); err == nil {
					if operator, ok := payload["operator"].(string); ok {
						dto.Operator = operator
					}
					if diff, ok := payload["diff"].(map[string]interface{}); ok {
						dto.Diff = diff
					}
				}
			}

			resp.Events = append(resp.Events, dto)
		}
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *LineageHandler) writeError(w http.ResponseWriter, status int, message, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{
		"error":   message,
		"message": message,
	}
	if detail != "" {
		resp["detail"] = detail
	}
	json.NewEncoder(w).Encode(resp)
}

// DriftHandler handles drift alert requests.
type DriftHandler struct {
	logger      *observability.Logger
	driftRunner *monitoring.DriftRunner
}

// NewDriftHandler creates a new drift handler.
func NewDriftHandler(logger *observability.Logger, driftRunner *monitoring.DriftRunner) *DriftHandler {
	return &DriftHandler{
		logger:      logger,
		driftRunner: driftRunner,
	}
}

// DriftAlertDTO represents a drift alert.
type DriftAlertDTO struct {
	ID                string                 `json:"id"`
	TenantID          string                 `json:"tenantId"`
	ProductID         string                 `json:"productId,omitempty"`
	CampaignVariantID string                 `json:"campaignVariantId,omitempty"`
	AlertType         string                 `json:"alertType"`
	Details           map[string]interface{} `json:"details,omitempty"`
	Status            string                 `json:"status"`
	DetectedAt        string                 `json:"detectedAt"`
	ResolvedAt        string                 `json:"resolvedAt,omitempty"`
}

// ListAlerts handles GET /drift/alerts.
func (h *DriftHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get tenant from context or query param
	tenantIDStr := r.URL.Query().Get("tenantId")
	if tenantIDStr == "" {
		tenantIDStr = middleware.TenantFromContext(ctx)
	}
	if tenantIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "tenantId is required", "")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	// Get open alerts
	alerts, err := h.driftRunner.ListOpenAlerts(ctx, tenantID)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to list drift alerts")
		h.writeError(w, http.StatusInternalServerError, "failed to list alerts", err.Error())
		return
	}

	// Convert to DTOs (alerts is []storage.DriftAlert)
	resp := make([]DriftAlertDTO, 0)
	if alerts != nil {
		for _, alert := range alerts {
			dto := DriftAlertDTO{
				ID:         alert.ID.String(),
				TenantID:   alert.TenantID.String(),
				AlertType:  string(alert.AlertType),
				Status:     string(alert.Status),
				DetectedAt: alert.DetectedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if alert.ProductID != nil {
				dto.ProductID = alert.ProductID.String()
			}
			if alert.CampaignVariantID != nil {
				dto.CampaignVariantID = alert.CampaignVariantID.String()
			}
			if alert.ResolvedAt != nil {
				dto.ResolvedAt = alert.ResolvedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			resp = append(resp, dto)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// TriggerCheck handles POST /drift/check.
func (h *DriftHandler) TriggerCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get tenant from context or query param
	tenantIDStr := r.URL.Query().Get("tenantId")
	if tenantIDStr == "" {
		tenantIDStr = middleware.TenantFromContext(ctx)
	}
	if tenantIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "tenantId is required", "")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	// Run drift check
	result, err := h.driftRunner.RunCheck(ctx, tenantID)
	if err != nil {
		h.logger.Error().Err(err).Msg("Drift check failed")
		h.writeError(w, http.StatusInternalServerError, "drift check failed", err.Error())
		return
	}

	resp := map[string]interface{}{
		"checkedAt":       result.CheckedAt.Format("2006-01-02T15:04:05Z07:00"),
		"staleCampaigns":  len(result.StaleCampaigns),
		"hashMismatches":  len(result.HashMismatches),
		"embeddingDrift":  len(result.EmbeddingDrift),
		"totalAlerts":     result.TotalAlerts,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *DriftHandler) writeError(w http.ResponseWriter, status int, message, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{
		"error":   message,
		"message": message,
	}
	if detail != "" {
		resp["detail"] = detail
	}
	json.NewEncoder(w).Encode(resp)
}


