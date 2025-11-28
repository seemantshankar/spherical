// Package handlers provides HTTP handlers for the Knowledge Engine API.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// IngestionHandler handles brochure ingestion requests.
type IngestionHandler struct {
	logger    *observability.Logger
	pipeline  *ingest.Pipeline
	publisher *ingest.Publisher
}

// NewIngestionHandler creates a new ingestion handler.
func NewIngestionHandler(logger *observability.Logger, pipeline *ingest.Pipeline, publisher *ingest.Publisher) *IngestionHandler {
	return &IngestionHandler{
		logger:    logger,
		pipeline:  pipeline,
		publisher: publisher,
	}
}

// IngestionRequestDTO represents the API request for ingestion.
type IngestionRequestDTO struct {
	DocumentSource DocumentSourceDTO `json:"documentSource"`
	MarkdownURL    string            `json:"markdownUrl"`
	OverwriteDraft bool              `json:"overwriteDraft,omitempty"`
	AutoPublish    bool              `json:"autoPublish,omitempty"`
	Operator       string            `json:"operator"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

// DocumentSourceDTO represents a document source.
type DocumentSourceDTO struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// IngestionJobDTO represents the API response for ingestion.
type IngestionJobDTO struct {
	ID                string   `json:"id"`
	Status            string   `json:"status"`
	StartedAt         string   `json:"startedAt,omitempty"`
	ETASeconds        int      `json:"etaSeconds,omitempty"`
	ConflictingSpecIDs []string `json:"conflictingSpecIds,omitempty"`
	Error             string   `json:"error,omitempty"`
}

// Ingest handles POST /tenants/{tenantId}/products/{productId}/campaigns/{campaignId}/ingest.
func (h *IngestionHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse path parameters
	tenantIDStr := chi.URLParam(r, "tenantId")
	productIDStr := chi.URLParam(r, "productId")
	campaignIDStr := chi.URLParam(r, "campaignId")

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid productId", err.Error())
		return
	}

	campaignID, err := uuid.Parse(campaignIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid campaignId", err.Error())
		return
	}

	// Parse request body
	var reqDTO IngestionRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Validate required fields
	if reqDTO.MarkdownURL == "" {
		h.writeError(w, http.StatusBadRequest, "markdownUrl is required", "")
		return
	}
	if reqDTO.Operator == "" {
		h.writeError(w, http.StatusBadRequest, "operator is required", "")
		return
	}

	h.logger.Info().
		Str("tenant_id", tenantIDStr).
		Str("product_id", productIDStr).
		Str("campaign_id", campaignIDStr).
		Str("operator", reqDTO.Operator).
		Msg("Starting ingestion")

	// Create job ID
	jobID := uuid.New()
	startedAt := time.Now()

	// Start async ingestion (in production, this would be queued)
	go func() {
		_, err := h.pipeline.Ingest(ctx, ingest.IngestionRequest{
			TenantID:     tenantID,
			ProductID:    productID,
			CampaignID:   campaignID,
			MarkdownPath: reqDTO.MarkdownURL, // Would fetch from URL in production
			Operator:     reqDTO.Operator,
			Overwrite:    reqDTO.OverwriteDraft,
			AutoPublish:  reqDTO.AutoPublish,
		})
		if err != nil {
			h.logger.Error().Err(err).Str("job_id", jobID.String()).Msg("Ingestion failed")
		}
	}()

	// Return accepted response
	resp := IngestionJobDTO{
		ID:         jobID.String(),
		Status:     string(storage.JobStatusPending),
		StartedAt:  startedAt.Format(time.RFC3339),
		ETASeconds: 60, // Estimate
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

// PublishRequestDTO represents the API request for publishing.
type PublishRequestDTO struct {
	Version      int    `json:"version"`
	ApprovedBy   string `json:"approvedBy"`
	ReleaseNotes string `json:"releaseNotes,omitempty"`
}

// CampaignVersionDTO represents the API response for publish.
type CampaignVersionDTO struct {
	CampaignID       string  `json:"campaignId"`
	Version          int     `json:"version"`
	Status           string  `json:"status"`
	EffectiveFrom    string  `json:"effectiveFrom,omitempty"`
	EffectiveThrough *string `json:"effectiveThrough,omitempty"`
	PublishedBy      string  `json:"publishedBy,omitempty"`
}

// Publish handles POST /tenants/{tenantId}/campaigns/{campaignId}/publish.
func (h *IngestionHandler) Publish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse path parameters
	tenantIDStr := chi.URLParam(r, "tenantId")
	campaignIDStr := chi.URLParam(r, "campaignId")

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	campaignID, err := uuid.Parse(campaignIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid campaignId", err.Error())
		return
	}

	// Parse request body
	var reqDTO PublishRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Validate required fields
	if reqDTO.ApprovedBy == "" {
		h.writeError(w, http.StatusBadRequest, "approvedBy is required", "")
		return
	}

	h.logger.Info().
		Str("tenant_id", tenantIDStr).
		Str("campaign_id", campaignIDStr).
		Int("version", reqDTO.Version).
		Str("approved_by", reqDTO.ApprovedBy).
		Msg("Publishing campaign")

	// Execute publish
	result, err := h.publisher.Publish(ctx, ingest.PublishRequest{
		TenantID:     tenantID,
		CampaignID:   campaignID,
		Version:      reqDTO.Version,
		ApprovedBy:   reqDTO.ApprovedBy,
		ReleaseNotes: reqDTO.ReleaseNotes,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if err == ingest.ErrConflictsExist {
			status = http.StatusPreconditionFailed
		} else if err == ingest.ErrCampaignNotFound {
			status = http.StatusNotFound
		} else if err == ingest.ErrCampaignNotDraft {
			status = http.StatusConflict
		}
		h.writeError(w, status, "publish failed", err.Error())
		return
	}

	// Return response
	resp := CampaignVersionDTO{
		CampaignID:    result.CampaignID.String(),
		Version:       result.Version,
		Status:        string(result.Status),
		EffectiveFrom: result.EffectiveFrom.Format(time.RFC3339),
		PublishedBy:   result.PublishedBy,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *IngestionHandler) writeError(w http.ResponseWriter, status int, message, detail string) {
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

