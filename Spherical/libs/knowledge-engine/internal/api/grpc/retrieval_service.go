// Package grpc provides gRPC/Connect service implementations for the Knowledge Engine.
package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// RetrievalService implements the gRPC/Connect retrieval service.
type RetrievalService struct {
	logger        *observability.Logger
	router        *retrieval.Router
	lineageWriter *monitoring.LineageWriter
}

// NewRetrievalService creates a new retrieval service.
func NewRetrievalService(logger *observability.Logger, router *retrieval.Router, lineageWriter *monitoring.LineageWriter) *RetrievalService {
	return &RetrievalService{
		logger:        logger,
		router:        router,
		lineageWriter: lineageWriter,
	}
}

// RetrievalRequest represents the gRPC request message.
type RetrievalRequest struct {
	TenantID          string   `json:"tenant_id"`
	ProductIDs        []string `json:"product_ids"`
	CampaignVariantID string   `json:"campaign_variant_id,omitempty"`
	Question          string   `json:"question"`
	IntentHint        string   `json:"intent_hint,omitempty"`
	Categories        []string `json:"categories,omitempty"`
	ChunkTypes        []string `json:"chunk_types,omitempty"`
	MaxChunks         int32    `json:"max_chunks,omitempty"`
	IncludeLineage    bool     `json:"include_lineage,omitempty"`
	// New: Structured spec name list from LLM
	RequestedSpecs []string `json:"requested_specs,omitempty"`
	// New: Request mode (natural language vs structured)
	RequestMode string `json:"request_mode,omitempty"`
	// New: Include natural language summary
	IncludeSummary bool `json:"include_summary,omitempty"`
}

// RetrievalResponse represents the gRPC response message.
type RetrievalResponse struct {
	Intent          string        `json:"intent"`
	LatencyMs       int64         `json:"latency_ms"`
	StructuredFacts []*SpecFact   `json:"structured_facts"`
	SemanticChunks  []*Chunk      `json:"semantic_chunks"`
	Comparisons     []*Comparison `json:"comparisons,omitempty"`
	Lineage         []*Lineage    `json:"lineage,omitempty"`
	// New: Per-spec availability status
	SpecAvailability []*SpecAvailability `json:"spec_availability,omitempty"`
	// New: Overall confidence score
	OverallConfidence float64 `json:"overall_confidence"`
	// New: Optional natural language summary
	Summary *string `json:"summary,omitempty"`
}

// SpecFact represents a structured spec fact in gRPC.
type SpecFact struct {
	SpecItemID          string  `json:"spec_item_id"`
	Category            string  `json:"category"`
	Name                string  `json:"name"`
	Value               string  `json:"value"`
	Unit                string  `json:"unit,omitempty"`
	KeyFeatures         string  `json:"key_features,omitempty"`
	VariantAvailability string  `json:"variant_availability,omitempty"`
	Explanation         string  `json:"explanation,omitempty"`
	Provenance          string  `json:"provenance,omitempty"`
	Confidence          float64 `json:"confidence"`
	CampaignVariantID   string  `json:"campaign_variant_id"`
	Source              *Source `json:"source,omitempty"`
}

// Chunk represents a semantic chunk in gRPC.
type Chunk struct {
	ChunkID   string            `json:"chunk_id"`
	ChunkType string            `json:"chunk_type"`
	Text      string            `json:"text"`
	Distance  float32           `json:"distance"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Source    *Source           `json:"source,omitempty"`
}

// Comparison represents a comparison result in gRPC.
type Comparison struct {
	Dimension          string `json:"dimension"`
	PrimaryProductID   string `json:"primary_product_id"`
	SecondaryProductID string `json:"secondary_product_id"`
	PrimaryValue       string `json:"primary_value,omitempty"`
	SecondaryValue     string `json:"secondary_value,omitempty"`
	Verdict            string `json:"verdict"`
	Narrative          string `json:"narrative,omitempty"`
}

// Lineage represents lineage information in gRPC.
type Lineage struct {
	ResourceType     string `json:"resource_type"`
	ResourceID       string `json:"resource_id"`
	Action           string `json:"action"`
	DocumentSourceID string `json:"document_source_id,omitempty"`
	OccurredAt       string `json:"occurred_at"`
}

// Source represents source reference in gRPC.
type Source struct {
	DocumentSourceID string `json:"document_source_id,omitempty"`
	Page             int32  `json:"page,omitempty"`
	URL              string `json:"url,omitempty"`
}

// SpecAvailability represents availability status for a spec in gRPC.
type SpecAvailability struct {
	SpecName         string   `json:"spec_name"`
	Status           string   `json:"status"`
	Confidence       float64  `json:"confidence"`
	AlternativeNames []string `json:"alternative_names,omitempty"`
	// Include matched specs/chunks if found
	MatchedSpecs  []*SpecFact `json:"matched_specs,omitempty"`
	MatchedChunks []*Chunk    `json:"matched_chunks,omitempty"`
}

// Query handles gRPC/Connect retrieval queries.
func (s *RetrievalService) Query(ctx context.Context, req *connect.Request[RetrievalRequest]) (*connect.Response[RetrievalResponse], error) {
	msg := req.Msg

	// Validate required fields
	if msg.TenantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tenant_id is required"))
	}
	if msg.Question == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("question is required"))
	}

	// Parse tenant ID
	tenantID, err := uuid.Parse(msg.TenantID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid tenant_id format"))
	}

	// Parse product IDs
	var productIDs []uuid.UUID
	for _, pidStr := range msg.ProductIDs {
		if pid, err := uuid.Parse(pidStr); err == nil {
			productIDs = append(productIDs, pid)
		}
	}

	// Parse campaign variant ID
	var campaignVariantID *uuid.UUID
	if msg.CampaignVariantID != "" {
		if cvid, err := uuid.Parse(msg.CampaignVariantID); err == nil {
			campaignVariantID = &cvid
		}
	}

	// Parse intent hint
	var intentHint *retrieval.Intent
	if msg.IntentHint != "" {
		intent := retrieval.Intent(msg.IntentHint)
		intentHint = &intent
	}

	// Parse filters
	var filters retrieval.RetrievalFilters
	filters.Categories = msg.Categories
	for _, ct := range msg.ChunkTypes {
		filters.ChunkTypes = append(filters.ChunkTypes, storage.ChunkType(ct))
	}

	// Set defaults
	maxChunks := int(msg.MaxChunks)
	if maxChunks <= 0 {
		maxChunks = 6
	}

	// Parse request mode
	var requestMode retrieval.RequestMode
	if msg.RequestMode != "" {
		requestMode = retrieval.RequestMode(msg.RequestMode)
	} else if len(msg.RequestedSpecs) > 0 {
		requestMode = retrieval.RequestModeStructured
	} else {
		requestMode = retrieval.RequestModeNaturalLanguage
	}

	// Build internal request
	internalReq := retrieval.RetrievalRequest{
		TenantID:          tenantID,
		ProductIDs:        productIDs,
		CampaignVariantID: campaignVariantID,
		Question:          msg.Question,
		IntentHint:        intentHint,
		Filters:           filters,
		MaxChunks:         maxChunks,
		IncludeLineage:    msg.IncludeLineage,
		RequestedSpecs:    msg.RequestedSpecs,
		RequestMode:       requestMode,
	}

	// Execute query
	resp, err := s.router.Query(ctx, internalReq)
	if err != nil {
		s.logger.Error().Err(err).Msg("Query failed")
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Record audit event
	if s.lineageWriter != nil {
		resultCount := len(resp.StructuredFacts) + len(resp.SemanticChunks)
		if err := s.lineageWriter.RecordRetrievalRequest(ctx, tenantID, productIDs, internalReq.Question, string(resp.Intent), resultCount); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to record retrieval audit event")
		}
	}

	// Convert to gRPC response
	grpcResp := s.toGRPCResponse(resp)

	return connect.NewResponse(grpcResp), nil
}

func (s *RetrievalService) toGRPCResponse(resp *retrieval.RetrievalResponse) *RetrievalResponse {
	grpcResp := &RetrievalResponse{
		Intent:          string(resp.Intent),
		LatencyMs:       resp.LatencyMs,
		StructuredFacts: make([]*SpecFact, 0, len(resp.StructuredFacts)),
		SemanticChunks:  make([]*Chunk, 0, len(resp.SemanticChunks)),
	}

	for _, fact := range resp.StructuredFacts {
		grpcResp.StructuredFacts = append(grpcResp.StructuredFacts, &SpecFact{
			SpecItemID:          fact.SpecItemID.String(),
			Category:            fact.Category,
			Name:                fact.Name,
			Value:               fact.Value,
			Unit:                fact.Unit,
			KeyFeatures:         fact.KeyFeatures,
			VariantAvailability: fact.VariantAvailability,
			Explanation:         fact.Explanation,
			Provenance:          fact.Provenance,
			Confidence:          fact.Confidence,
			CampaignVariantID:   fact.CampaignVariantID.String(),
			Source:              s.toGRPCSource(fact.Source),
		})
	}

	for _, chunk := range resp.SemanticChunks {
		metadata := make(map[string]string)
		for k, v := range chunk.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}
		grpcResp.SemanticChunks = append(grpcResp.SemanticChunks, &Chunk{
			ChunkID:   chunk.ChunkID.String(),
			ChunkType: string(chunk.ChunkType),
			Text:      chunk.Text,
			Distance:  chunk.Distance,
			Metadata:  metadata,
			Source:    s.toGRPCSource(chunk.Source),
		})
	}

	for _, comp := range resp.Comparisons {
		grpcResp.Comparisons = append(grpcResp.Comparisons, &Comparison{
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
		grpcResp.Lineage = append(grpcResp.Lineage, &Lineage{
			ResourceType:     lineage.ResourceType,
			ResourceID:       lineage.ResourceID.String(),
			Action:           string(lineage.Action),
			DocumentSourceID: docID,
			OccurredAt:       lineage.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	// Convert spec availability statuses
	grpcResp.SpecAvailability = make([]*SpecAvailability, 0, len(resp.SpecAvailability))
	for _, status := range resp.SpecAvailability {
		grpcStatus := &SpecAvailability{
			SpecName:         status.SpecName,
			Status:           string(status.Status),
			Confidence:       status.Confidence,
			AlternativeNames: status.AlternativeNames,
		}

		// Convert matched specs
		grpcStatus.MatchedSpecs = make([]*SpecFact, 0, len(status.MatchedSpecs))
		for _, fact := range status.MatchedSpecs {
			grpcStatus.MatchedSpecs = append(grpcStatus.MatchedSpecs, &SpecFact{
				SpecItemID:          fact.SpecItemID.String(),
				Category:            fact.Category,
				Name:                fact.Name,
				Value:               fact.Value,
				Unit:                fact.Unit,
				KeyFeatures:         fact.KeyFeatures,
				VariantAvailability: fact.VariantAvailability,
				Explanation:         fact.Explanation,
				Provenance:          fact.Provenance,
				Confidence:          fact.Confidence,
				CampaignVariantID:   fact.CampaignVariantID.String(),
				Source:              s.toGRPCSource(fact.Source),
			})
		}

		// Convert matched chunks
		grpcStatus.MatchedChunks = make([]*Chunk, 0, len(status.MatchedChunks))
		for _, chunk := range status.MatchedChunks {
			metadata := make(map[string]string)
			for k, v := range chunk.Metadata {
				if str, ok := v.(string); ok {
					metadata[k] = str
				}
			}
			grpcStatus.MatchedChunks = append(grpcStatus.MatchedChunks, &Chunk{
				ChunkID:   chunk.ChunkID.String(),
				ChunkType: string(chunk.ChunkType),
				Text:      chunk.Text,
				Distance:  chunk.Distance,
				Metadata:  metadata,
				Source:    s.toGRPCSource(chunk.Source),
			})
		}

		grpcResp.SpecAvailability = append(grpcResp.SpecAvailability, grpcStatus)
	}

	// Set overall confidence and summary
	grpcResp.OverallConfidence = resp.OverallConfidence
	if resp.Summary != nil {
		grpcResp.Summary = resp.Summary
	}

	return grpcResp
}

func (s *RetrievalService) toGRPCSource(src retrieval.SourceRef) *Source {
	grpcSrc := &Source{}
	if src.DocumentSourceID != nil {
		grpcSrc.DocumentSourceID = src.DocumentSourceID.String()
	}
	if src.Page != nil {
		grpcSrc.Page = int32(*src.Page)
	}
	if src.URL != nil {
		grpcSrc.URL = *src.URL
	}
	return grpcSrc
}
