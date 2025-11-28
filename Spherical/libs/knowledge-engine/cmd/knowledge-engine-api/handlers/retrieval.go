// Package handlers provides HTTP handlers for the Knowledge Engine API.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-api/middleware"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// RetrievalHandler handles knowledge retrieval requests.
type RetrievalHandler struct {
	logger        *observability.Logger
	router        *retrieval.Router
	lineageWriter *monitoring.LineageWriter
}

// NewRetrievalHandler creates a new retrieval handler.
func NewRetrievalHandler(logger *observability.Logger, router *retrieval.Router, lineageWriter *monitoring.LineageWriter) *RetrievalHandler {
	return &RetrievalHandler{
		logger:        logger,
		router:        router,
		lineageWriter: lineageWriter,
	}
}

// RetrievalRequestDTO represents the API request for retrieval.
type RetrievalRequestDTO struct {
	TenantID            string                 `json:"tenantId"`
	ProductIDs          []string               `json:"productIds,omitempty"`
	CampaignVariantID   string                 `json:"campaignVariantId,omitempty"`
	Question            string                 `json:"question"`
	IntentHint          string                 `json:"intentHint,omitempty"`
	ConversationContext []ConversationMessage  `json:"conversationContext,omitempty"`
	Filters             *RetrievalFiltersDTO   `json:"filters,omitempty"`
	MaxChunks           int                    `json:"maxChunks,omitempty"`
	IncludeLineage      bool                   `json:"includeLineage,omitempty"`
}

// ConversationMessage represents a conversation turn.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RetrievalFiltersDTO represents filtering options.
type RetrievalFiltersDTO struct {
	Categories []string `json:"categories,omitempty"`
	ChunkTypes []string `json:"chunkTypes,omitempty"`
}

// RetrievalResponseDTO represents the API response.
type RetrievalResponseDTO struct {
	Intent          string           `json:"intent"`
	LatencyMs       int64            `json:"latencyMs"`
	StructuredFacts []SpecFactDTO    `json:"structuredFacts"`
	SemanticChunks  []SemanticChunkDTO `json:"semanticChunks"`
	Comparisons     []ComparisonDTO  `json:"comparisons,omitempty"`
	Lineage         []LineageDTO     `json:"lineage,omitempty"`
}

// SpecFactDTO represents a structured fact.
type SpecFactDTO struct {
	SpecItemID        string    `json:"specItemId"`
	Category          string    `json:"category"`
	Name              string    `json:"name"`
	Value             string    `json:"value"`
	Unit              string    `json:"unit,omitempty"`
	Confidence        float64   `json:"confidence"`
	CampaignVariantID string    `json:"campaignVariantId"`
	Source            SourceDTO `json:"source"`
}

// SemanticChunkDTO represents a semantic chunk.
type SemanticChunkDTO struct {
	ChunkID   string                 `json:"chunkId"`
	ChunkType string                 `json:"chunkType"`
	Text      string                 `json:"text"`
	Distance  float32                `json:"distance"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Source    SourceDTO              `json:"source"`
}

// ComparisonDTO represents a comparison result.
type ComparisonDTO struct {
	Dimension          string `json:"dimension"`
	PrimaryProductID   string `json:"primaryProductId"`
	SecondaryProductID string `json:"secondaryProductId"`
	PrimaryValue       string `json:"primaryValue,omitempty"`
	SecondaryValue     string `json:"secondaryValue,omitempty"`
	Verdict            string `json:"verdict"`
	Narrative          string `json:"narrative,omitempty"`
}

// LineageDTO represents a lineage event.
type LineageDTO struct {
	ResourceType     string `json:"resourceType"`
	ResourceID       string `json:"resourceId"`
	Action           string `json:"action"`
	DocumentSourceID string `json:"documentSourceId,omitempty"`
	OccurredAt       string `json:"occurredAt"`
}

// SourceDTO represents a source reference.
type SourceDTO struct {
	DocumentSourceID string `json:"documentSourceId,omitempty"`
	Page             int    `json:"page,omitempty"`
	URL              string `json:"url,omitempty"`
}

// Query handles POST /retrieval/query.
func (h *RetrievalHandler) Query(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var reqDTO RetrievalRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Validate required fields
	if reqDTO.Question == "" {
		h.writeError(w, http.StatusBadRequest, "question is required", "")
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

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		// Try to look up by name
		tenantID = uuid.Nil // Would need to query by name
	}

	// Parse product IDs
	var productIDs []uuid.UUID
	for _, pidStr := range reqDTO.ProductIDs {
		if pid, err := uuid.Parse(pidStr); err == nil {
			productIDs = append(productIDs, pid)
		}
	}

	// Parse campaign variant ID
	var campaignVariantID *uuid.UUID
	if reqDTO.CampaignVariantID != "" {
		if cvid, err := uuid.Parse(reqDTO.CampaignVariantID); err == nil {
			campaignVariantID = &cvid
		}
	}

	// Parse intent hint
	var intentHint *retrieval.Intent
	if reqDTO.IntentHint != "" {
		intent := retrieval.Intent(reqDTO.IntentHint)
		intentHint = &intent
	}

	// Parse filters
	var filters retrieval.RetrievalFilters
	if reqDTO.Filters != nil {
		filters.Categories = reqDTO.Filters.Categories
		for _, ct := range reqDTO.Filters.ChunkTypes {
			filters.ChunkTypes = append(filters.ChunkTypes, storage.ChunkType(ct))
		}
	}

	// Build request
	req := retrieval.RetrievalRequest{
		TenantID:          tenantID,
		ProductIDs:        productIDs,
		CampaignVariantID: campaignVariantID,
		Question:          reqDTO.Question,
		IntentHint:        intentHint,
		Filters:           filters,
		MaxChunks:         reqDTO.MaxChunks,
		IncludeLineage:    reqDTO.IncludeLineage,
	}

	// Execute query
	resp, err := h.router.Query(ctx, req)
	if err != nil {
		h.logger.Error().Err(err).Msg("Query failed")
		h.writeError(w, http.StatusInternalServerError, "query failed", err.Error())
		return
	}

	// Record audit event for retrieval request (T038)
	if h.lineageWriter != nil {
		resultCount := len(resp.StructuredFacts) + len(resp.SemanticChunks)
		if err := h.lineageWriter.RecordRetrievalRequest(ctx, tenantID, productIDs, req.Question, string(resp.Intent), resultCount); err != nil {
			h.logger.Warn().Err(err).Msg("Failed to record retrieval audit event")
		}
	}

	// Convert to DTO
	respDTO := h.toResponseDTO(resp)

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(respDTO); err != nil {
		h.logger.Error().Err(err).Msg("Failed to encode response")
	}
}

func (h *RetrievalHandler) toResponseDTO(resp *retrieval.RetrievalResponse) RetrievalResponseDTO {
	dto := RetrievalResponseDTO{
		Intent:          string(resp.Intent),
		LatencyMs:       resp.LatencyMs,
		StructuredFacts: make([]SpecFactDTO, 0, len(resp.StructuredFacts)),
		SemanticChunks:  make([]SemanticChunkDTO, 0, len(resp.SemanticChunks)),
	}

	for _, fact := range resp.StructuredFacts {
		dto.StructuredFacts = append(dto.StructuredFacts, SpecFactDTO{
			SpecItemID:        fact.SpecItemID.String(),
			Category:          fact.Category,
			Name:              fact.Name,
			Value:             fact.Value,
			Unit:              fact.Unit,
			Confidence:        fact.Confidence,
			CampaignVariantID: fact.CampaignVariantID.String(),
			Source:            h.toSourceDTO(fact.Source),
		})
	}

	for _, chunk := range resp.SemanticChunks {
		dto.SemanticChunks = append(dto.SemanticChunks, SemanticChunkDTO{
			ChunkID:   chunk.ChunkID.String(),
			ChunkType: string(chunk.ChunkType),
			Text:      chunk.Text,
			Distance:  chunk.Distance,
			Metadata:  chunk.Metadata,
			Source:    h.toSourceDTO(chunk.Source),
		})
	}

	for _, comp := range resp.Comparisons {
		dto.Comparisons = append(dto.Comparisons, ComparisonDTO{
			Dimension:          comp.Dimension,
			PrimaryProductID:   comp.PrimaryProductID.String(),
			SecondaryProductID: comp.SecondaryProductID.String(),
			PrimaryValue:       comp.PrimaryValue,
			SecondaryValue:     comp.SecondaryValue,
			Verdict:            string(comp.Verdict),
			Narrative:          comp.Narrative,
		})
	}

	for _, lineage := range resp.Lineage {
		docID := ""
		if lineage.DocumentSourceID != nil {
			docID = lineage.DocumentSourceID.String()
		}
		dto.Lineage = append(dto.Lineage, LineageDTO{
			ResourceType:     lineage.ResourceType,
			ResourceID:       lineage.ResourceID.String(),
			Action:           string(lineage.Action),
			DocumentSourceID: docID,
			OccurredAt:       lineage.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return dto
}

func (h *RetrievalHandler) toSourceDTO(src retrieval.SourceRef) SourceDTO {
	dto := SourceDTO{}
	if src.DocumentSourceID != nil {
		dto.DocumentSourceID = src.DocumentSourceID.String()
	}
	if src.Page != nil {
		dto.Page = *src.Page
	}
	if src.URL != nil {
		dto.URL = *src.URL
	}
	return dto
}

func (h *RetrievalHandler) writeError(w http.ResponseWriter, status int, message, detail string) {
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

