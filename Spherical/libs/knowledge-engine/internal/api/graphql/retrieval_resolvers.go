// Package graphql provides GraphQL resolvers for the Knowledge Engine.
package graphql

import (
	"context"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// RetrievalResolver handles retrieval GraphQL queries.
type RetrievalResolver struct {
	logger        *observability.Logger
	router        *retrieval.Router
	lineageWriter *monitoring.LineageWriter
}

// NewRetrievalResolver creates a new retrieval resolver.
func NewRetrievalResolver(logger *observability.Logger, router *retrieval.Router, lineageWriter *monitoring.LineageWriter) *RetrievalResolver {
	return &RetrievalResolver{
		logger:        logger,
		router:        router,
		lineageWriter: lineageWriter,
	}
}

// RetrievalQueryInput represents the GraphQL input for retrieval queries.
type RetrievalQueryInput struct {
	TenantID          string           `json:"tenantId"`
	ProductIDs        []string         `json:"productIds"`
	CampaignVariantID *string          `json:"campaignVariantId,omitempty"`
	Question          string           `json:"question"`
	IntentHint        *string          `json:"intentHint,omitempty"`
	Filters           *FilterInput     `json:"filters,omitempty"`
	MaxChunks         *int             `json:"maxChunks,omitempty"`
	IncludeLineage    *bool            `json:"includeLineage,omitempty"`
	// New: Structured spec name list from LLM
	RequestedSpecs []string `json:"requestedSpecs,omitempty"`
	// New: Request mode (natural language vs structured)
	RequestMode    *string `json:"requestMode,omitempty"`
	// New: Include natural language summary
	IncludeSummary *bool   `json:"includeSummary,omitempty"`
}

// FilterInput represents retrieval filters.
type FilterInput struct {
	Categories []string `json:"categories,omitempty"`
	ChunkTypes []string `json:"chunkTypes,omitempty"`
}

// RetrievalResult represents the GraphQL retrieval response.
type RetrievalResult struct {
	Intent          string            `json:"intent"`
	LatencyMs       int               `json:"latencyMs"`
	StructuredFacts []*SpecFactResult `json:"structuredFacts"`
	SemanticChunks  []*ChunkResult    `json:"semanticChunks"`
	Comparisons     []*CompResult     `json:"comparisons,omitempty"`
	Lineage         []*LineageResult  `json:"lineage,omitempty"`
	// New: Per-spec availability status
	SpecAvailability []*SpecAvailabilityResult `json:"specAvailability,omitempty"`
	// New: Overall confidence score
	OverallConfidence float64 `json:"overallConfidence"`
	// New: Optional natural language summary
	Summary *string `json:"summary,omitempty"`
}

// SpecAvailabilityResult represents availability status for a spec.
type SpecAvailabilityResult struct {
	SpecName        string            `json:"specName"`
	Status          string            `json:"status"`
	Confidence      float64           `json:"confidence"`
	AlternativeNames []string         `json:"alternativeNames,omitempty"`
	MatchedSpecs    []*SpecFactResult `json:"matchedSpecs,omitempty"`
	MatchedChunks   []*ChunkResult    `json:"matchedChunks,omitempty"`
}

// SpecFactResult represents a structured spec fact.
type SpecFactResult struct {
	ID                string        `json:"id"`
	Category          string        `json:"category"`
	Name              string        `json:"name"`
	Value             string        `json:"value"`
	Unit              *string       `json:"unit,omitempty"`
	Confidence        float64       `json:"confidence"`
	CampaignVariantID string        `json:"campaignVariantId"`
	Source            *SourceResult `json:"source,omitempty"`
}

// ChunkResult represents a semantic chunk.
type ChunkResult struct {
	ID        string                 `json:"id"`
	ChunkType string                 `json:"chunkType"`
	Text      string                 `json:"text"`
	Distance  float64                `json:"distance"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Source    *SourceResult          `json:"source,omitempty"`
}

// CompResult represents a comparison result.
type CompResult struct {
	Dimension          string  `json:"dimension"`
	PrimaryProductID   string  `json:"primaryProductId"`
	SecondaryProductID string  `json:"secondaryProductId"`
	PrimaryValue       *string `json:"primaryValue,omitempty"`
	SecondaryValue     *string `json:"secondaryValue,omitempty"`
	Verdict            string  `json:"verdict"`
	Narrative          *string `json:"narrative,omitempty"`
}

// LineageResult represents lineage information.
type LineageResult struct {
	ResourceType     string  `json:"resourceType"`
	ResourceID       string  `json:"resourceId"`
	Action           string  `json:"action"`
	DocumentSourceID *string `json:"documentSourceId,omitempty"`
	OccurredAt       string  `json:"occurredAt"`
}

// SourceResult represents source reference.
type SourceResult struct {
	DocumentSourceID *string `json:"documentSourceId,omitempty"`
	Page             *int    `json:"page,omitempty"`
	URL              *string `json:"url,omitempty"`
}

// Query resolves retrieval queries.
func (r *RetrievalResolver) Query(ctx context.Context, input RetrievalQueryInput) (*RetrievalResult, error) {
	// Parse tenant ID
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, err
	}

	// Parse product IDs
	var productIDs []uuid.UUID
	for _, pidStr := range input.ProductIDs {
		if pid, err := uuid.Parse(pidStr); err == nil {
			productIDs = append(productIDs, pid)
		}
	}

	// Parse campaign variant ID
	var campaignVariantID *uuid.UUID
	if input.CampaignVariantID != nil {
		if cvid, err := uuid.Parse(*input.CampaignVariantID); err == nil {
			campaignVariantID = &cvid
		}
	}

	// Parse intent hint
	var intentHint *retrieval.Intent
	if input.IntentHint != nil {
		intent := retrieval.Intent(*input.IntentHint)
		intentHint = &intent
	}

	// Parse filters
	var filters retrieval.RetrievalFilters
	if input.Filters != nil {
		filters.Categories = input.Filters.Categories
		for _, ct := range input.Filters.ChunkTypes {
			filters.ChunkTypes = append(filters.ChunkTypes, storage.ChunkType(ct))
		}
	}

	// Set defaults
	maxChunks := 6
	if input.MaxChunks != nil && *input.MaxChunks > 0 {
		maxChunks = *input.MaxChunks
	}

	includeLineage := false
	if input.IncludeLineage != nil {
		includeLineage = *input.IncludeLineage
	}

	// Parse request mode
	var requestMode retrieval.RequestMode
	if input.RequestMode != nil {
		requestMode = retrieval.RequestMode(*input.RequestMode)
	} else if len(input.RequestedSpecs) > 0 {
		requestMode = retrieval.RequestModeStructured
	} else {
		requestMode = retrieval.RequestModeNaturalLanguage
	}

	// Build request
	req := retrieval.RetrievalRequest{
		TenantID:          tenantID,
		ProductIDs:        productIDs,
		CampaignVariantID: campaignVariantID,
		Question:          input.Question,
		IntentHint:        intentHint,
		Filters:           filters,
		MaxChunks:         maxChunks,
		IncludeLineage:    includeLineage,
		RequestedSpecs:    input.RequestedSpecs,
		RequestMode:       requestMode,
	}

	// Execute query
	resp, err := r.router.Query(ctx, req)
	if err != nil {
		return nil, err
	}

	// Record audit event
	if r.lineageWriter != nil {
		resultCount := len(resp.StructuredFacts) + len(resp.SemanticChunks)
		if err := r.lineageWriter.RecordRetrievalRequest(ctx, tenantID, productIDs, req.Question, string(resp.Intent), resultCount); err != nil {
			r.logger.Warn().Err(err).Msg("Failed to record retrieval audit event")
		}
	}

	// Convert to GraphQL result
	return r.toGraphQLResult(resp), nil
}

func (r *RetrievalResolver) toGraphQLResult(resp *retrieval.RetrievalResponse) *RetrievalResult {
	result := &RetrievalResult{
		Intent:          string(resp.Intent),
		LatencyMs:       int(resp.LatencyMs),
		StructuredFacts: make([]*SpecFactResult, 0, len(resp.StructuredFacts)),
		SemanticChunks:  make([]*ChunkResult, 0, len(resp.SemanticChunks)),
	}

	for _, fact := range resp.StructuredFacts {
		result.StructuredFacts = append(result.StructuredFacts, &SpecFactResult{
			ID:                fact.SpecItemID.String(),
			Category:          fact.Category,
			Name:              fact.Name,
			Value:             fact.Value,
			Unit:              nilIfEmpty(fact.Unit),
			Confidence:        fact.Confidence,
			CampaignVariantID: fact.CampaignVariantID.String(),
			Source:            r.toSourceResult(fact.Source),
		})
	}

	for _, chunk := range resp.SemanticChunks {
		result.SemanticChunks = append(result.SemanticChunks, &ChunkResult{
			ID:        chunk.ChunkID.String(),
			ChunkType: string(chunk.ChunkType),
			Text:      chunk.Text,
			Distance:  float64(chunk.Distance),
			Metadata:  chunk.Metadata,
			Source:    r.toSourceResult(chunk.Source),
		})
	}

	for _, comp := range resp.Comparisons {
		result.Comparisons = append(result.Comparisons, &CompResult{
			Dimension:          comp.Dimension,
			PrimaryProductID:   comp.PrimaryProductID.String(),
			SecondaryProductID: comp.SecondaryProductID.String(),
			PrimaryValue:       nilIfEmpty(comp.PrimaryValue),
			SecondaryValue:     nilIfEmpty(comp.SecondaryValue),
			Verdict:            string(comp.Verdict),
			Narrative:          nilIfEmpty(comp.Narrative),
		})
	}

	for _, lineage := range resp.Lineage {
		var docID *string
		if lineage.DocumentSourceID != nil {
			s := lineage.DocumentSourceID.String()
			docID = &s
		}
		result.Lineage = append(result.Lineage, &LineageResult{
			ResourceType:     lineage.ResourceType,
			ResourceID:       lineage.ResourceID.String(),
			Action:           string(lineage.Action),
			DocumentSourceID: docID,
			OccurredAt:       lineage.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	// Convert spec availability statuses
	result.SpecAvailability = make([]*SpecAvailabilityResult, 0, len(resp.SpecAvailability))
	for _, status := range resp.SpecAvailability {
		grpcStatus := &SpecAvailabilityResult{
			SpecName:        status.SpecName,
			Status:          string(status.Status),
			Confidence:      status.Confidence,
			AlternativeNames: status.AlternativeNames,
		}

		// Convert matched specs
		grpcStatus.MatchedSpecs = make([]*SpecFactResult, 0, len(status.MatchedSpecs))
		for _, fact := range status.MatchedSpecs {
			grpcStatus.MatchedSpecs = append(grpcStatus.MatchedSpecs, &SpecFactResult{
				ID:                fact.SpecItemID.String(),
				Category:          fact.Category,
				Name:              fact.Name,
				Value:             fact.Value,
				Unit:              nilIfEmpty(fact.Unit),
				Confidence:        fact.Confidence,
				CampaignVariantID: fact.CampaignVariantID.String(),
				Source:            r.toSourceResult(fact.Source),
			})
		}

		// Convert matched chunks
		grpcStatus.MatchedChunks = make([]*ChunkResult, 0, len(status.MatchedChunks))
		for _, chunk := range status.MatchedChunks {
			grpcStatus.MatchedChunks = append(grpcStatus.MatchedChunks, &ChunkResult{
				ID:        chunk.ChunkID.String(),
				ChunkType: string(chunk.ChunkType),
				Text:      chunk.Text,
				Distance:  float64(chunk.Distance),
				Metadata:  chunk.Metadata,
				Source:    r.toSourceResult(chunk.Source),
			})
		}

		result.SpecAvailability = append(result.SpecAvailability, grpcStatus)
	}

	// Set overall confidence and summary
	result.OverallConfidence = resp.OverallConfidence
	if resp.Summary != nil {
		result.Summary = resp.Summary
	}

	return result
}

func (r *RetrievalResolver) toSourceResult(src retrieval.SourceRef) *SourceResult {
	result := &SourceResult{}
	if src.DocumentSourceID != nil {
		s := src.DocumentSourceID.String()
		result.DocumentSourceID = &s
	}
	if src.Page != nil {
		result.Page = src.Page
	}
	if src.URL != nil {
		result.URL = src.URL
	}
	return result
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

