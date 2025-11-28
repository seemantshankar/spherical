// Package handlers provides HTTP handlers for the Knowledge Engine API.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/comparison"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

// ComparisonHandler handles product comparison requests.
type ComparisonHandler struct {
	logger        *observability.Logger
	materializer  *comparison.Materializer
	lineageWriter *monitoring.LineageWriter
}

// NewComparisonHandler creates a new comparison handler.
func NewComparisonHandler(logger *observability.Logger, materializer *comparison.Materializer, lineageWriter *monitoring.LineageWriter) *ComparisonHandler {
	return &ComparisonHandler{
		logger:        logger,
		materializer:  materializer,
		lineageWriter: lineageWriter,
	}
}

// ComparisonRequestDTO represents the API request for comparison.
type ComparisonRequestDTO struct {
	TenantID           string   `json:"tenantId"`
	PrimaryProductID   string   `json:"primaryProductId"`
	SecondaryProductID string   `json:"secondaryProductId"`
	Dimensions         []string `json:"dimensions,omitempty"`
	MaxRows            int      `json:"maxRows,omitempty"`
}

// ComparisonResponseDTO represents the API response for comparison.
type ComparisonResponseDTO struct {
	Comparisons []ComparisonRowDTO `json:"comparisons"`
}

// ComparisonRowDTO represents a comparison row.
type ComparisonRowDTO struct {
	Dimension          string `json:"dimension"`
	PrimaryProductID   string `json:"primaryProductId"`
	SecondaryProductID string `json:"secondaryProductId"`
	PrimaryValue       string `json:"primaryValue,omitempty"`
	SecondaryValue     string `json:"secondaryValue,omitempty"`
	Verdict            string `json:"verdict"`
	Narrative          string `json:"narrative,omitempty"`
	Shareability       string `json:"shareability"`
}

// Query handles POST /comparisons/query.
func (h *ComparisonHandler) Query(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var reqDTO ComparisonRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Get tenant from context or request
	tenantIDStr := reqDTO.TenantID
	if tenantIDStr == "" {
		tenantIDStr = middleware.TenantFromContext(ctx)
	}
	if tenantIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "tenantId is required", "")
		return
	}

	// Validate required fields
	if reqDTO.PrimaryProductID == "" {
		h.writeError(w, http.StatusBadRequest, "primaryProductId is required", "")
		return
	}
	if reqDTO.SecondaryProductID == "" {
		h.writeError(w, http.StatusBadRequest, "secondaryProductId is required", "")
		return
	}

	// Parse UUIDs
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tenantId", err.Error())
		return
	}

	primaryID, err := uuid.Parse(reqDTO.PrimaryProductID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid primaryProductId", err.Error())
		return
	}

	secondaryID, err := uuid.Parse(reqDTO.SecondaryProductID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid secondaryProductId", err.Error())
		return
	}

	h.logger.Info().
		Str("tenant_id", tenantIDStr).
		Str("primary_product", reqDTO.PrimaryProductID).
		Str("secondary_product", reqDTO.SecondaryProductID).
		Msg("Processing comparison query")

	// Execute comparison
	result, err := h.materializer.Compare(ctx, comparison.ComparisonRequest{
		TenantID:           tenantID,
		PrimaryProductID:   primaryID,
		SecondaryProductID: secondaryID,
		Dimensions:         reqDTO.Dimensions,
		MaxRows:            reqDTO.MaxRows,
	})
	if err != nil {
		if err == comparison.ErrProductNotAccessible {
			h.writeError(w, http.StatusNotFound, "comparison data unavailable", "Product not accessible for comparison")
			return
		}
		h.logger.Error().Err(err).Msg("Comparison failed")
		h.writeError(w, http.StatusInternalServerError, "comparison failed", err.Error())
		return
	}

	// Record audit event for comparison request (T047)
	if h.lineageWriter != nil {
		if err := h.lineageWriter.RecordComparisonRequest(ctx, tenantID, primaryID, secondaryID, len(result.Comparisons)); err != nil {
			h.logger.Warn().Err(err).Msg("Failed to record comparison audit event")
		}
	}

	// Convert to DTO
	resp := ComparisonResponseDTO{
		Comparisons: make([]ComparisonRowDTO, 0, len(result.Comparisons)),
	}

	for _, comp := range result.Comparisons {
		resp.Comparisons = append(resp.Comparisons, ComparisonRowDTO{
			Dimension:          comp.Dimension,
			PrimaryProductID:   comp.PrimaryProductID.String(),
			SecondaryProductID: comp.SecondaryProductID.String(),
			PrimaryValue:       comp.PrimaryValue,
			SecondaryValue:     comp.SecondaryValue,
			Verdict:            string(comp.Verdict),
			Narrative:          comp.Narrative,
			Shareability:       string(comp.Shareability),
		})
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ComparisonHandler) writeError(w http.ResponseWriter, status int, message, detail string) {
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

