// Package engine provides the public Go SDK for the Knowledge Engine.
package engine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Client is the public SDK client for the Knowledge Engine.
type Client struct {
	baseURL string
	apiKey  string
}

// ClientConfig holds client configuration.
type ClientConfig struct {
	BaseURL string
	APIKey  string
}

// NewClient creates a new Knowledge Engine client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8085"
	}

	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
	}, nil
}

// QueryRequest represents a retrieval query request.
type QueryRequest struct {
	TenantID    string   `json:"tenantId"`
	ProductIDs  []string `json:"productIds,omitempty"`
	CampaignID  string   `json:"campaignVariantId,omitempty"`
	Question    string   `json:"question"`
	IntentHint  string   `json:"intentHint,omitempty"`
	MaxChunks   int      `json:"maxChunks,omitempty"`
	IncludeLineage bool  `json:"includeLineage,omitempty"`
}

// QueryResponse represents a retrieval query response.
type QueryResponse struct {
	Intent          string        `json:"intent"`
	LatencyMs       int64         `json:"latencyMs"`
	StructuredFacts []SpecFact    `json:"structuredFacts"`
	SemanticChunks  []Chunk       `json:"semanticChunks"`
	Comparisons     []Comparison  `json:"comparisons,omitempty"`
	Lineage         []LineageItem `json:"lineage,omitempty"`
}

// SpecFact represents a structured specification fact.
type SpecFact struct {
	SpecItemID string  `json:"specItemId"`
	Category   string  `json:"category"`
	Name       string  `json:"name"`
	Value      string  `json:"value"`
	Unit       string  `json:"unit,omitempty"`
	Confidence float64 `json:"confidence"`
	CampaignID string  `json:"campaignVariantId"`
	Source     Source  `json:"source"`
}

// Chunk represents a semantic chunk.
type Chunk struct {
	ChunkID   string                 `json:"chunkId"`
	ChunkType string                 `json:"chunkType"`
	Text      string                 `json:"text"`
	Distance  float32                `json:"distance"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Source    Source                 `json:"source"`
}

// Comparison represents a comparison row.
type Comparison struct {
	Dimension          string `json:"dimension"`
	PrimaryProductID   string `json:"primaryProductId"`
	SecondaryProductID string `json:"secondaryProductId"`
	PrimaryValue       string `json:"primaryValue,omitempty"`
	SecondaryValue     string `json:"secondaryValue,omitempty"`
	Verdict            string `json:"verdict"`
	Narrative          string `json:"narrative,omitempty"`
}

// LineageItem represents a lineage event.
type LineageItem struct {
	ResourceType     string `json:"resourceType"`
	ResourceID       string `json:"resourceId"`
	Action           string `json:"action"`
	DocumentSourceID string `json:"documentSourceId,omitempty"`
	OccurredAt       string `json:"occurredAt"`
}

// Source represents a document source reference.
type Source struct {
	DocumentSourceID string `json:"documentSourceId,omitempty"`
	Page             int    `json:"page,omitempty"`
	URL              string `json:"url,omitempty"`
}

// Query executes a retrieval query against the Knowledge Engine.
func (c *Client) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	// TODO: Implement HTTP client call to /retrieval/query
	// For now, return a placeholder

	return &QueryResponse{
		Intent:          "spec_lookup",
		LatencyMs:       50,
		StructuredFacts: []SpecFact{},
		SemanticChunks:  []Chunk{},
	}, nil
}

// CompareRequest represents a comparison query request.
type CompareRequest struct {
	TenantID           string   `json:"tenantId"`
	PrimaryProductID   string   `json:"primaryProductId"`
	SecondaryProductID string   `json:"secondaryProductId"`
	Dimensions         []string `json:"dimensions,omitempty"`
	MaxRows            int      `json:"maxRows,omitempty"`
}

// CompareResponse represents a comparison query response.
type CompareResponse struct {
	Comparisons []Comparison `json:"comparisons"`
}

// Compare executes a comparison query.
func (c *Client) Compare(ctx context.Context, req CompareRequest) (*CompareResponse, error) {
	// TODO: Implement HTTP client call to /comparisons/query

	return &CompareResponse{
		Comparisons: []Comparison{},
	}, nil
}

// GetLineage retrieves lineage for a resource.
func (c *Client) GetLineage(ctx context.Context, tenantID, resourceType, resourceID string) ([]LineageItem, error) {
	// TODO: Implement HTTP client call to /lineage/{resourceType}/{resourceId}

	return nil, nil
}

// IngestRequest represents an ingestion request.
type IngestRequest struct {
	TenantID     string `json:"tenantId"`
	ProductID    string `json:"productId"`
	CampaignID   string `json:"campaignId"`
	MarkdownURL  string `json:"markdownUrl"`
	OverwriteDraft bool `json:"overwriteDraft,omitempty"`
	AutoPublish  bool   `json:"autoPublish,omitempty"`
	Operator     string `json:"operator"`
}

// IngestResponse represents an ingestion response.
type IngestResponse struct {
	JobID     string `json:"id"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt,omitempty"`
}

// Ingest triggers a brochure ingestion job.
func (c *Client) Ingest(ctx context.Context, req IngestRequest) (*IngestResponse, error) {
	// TODO: Implement HTTP client call

	return &IngestResponse{
		JobID:  uuid.New().String(),
		Status: "pending",
	}, nil
}

// PublishRequest represents a publish request.
type PublishRequest struct {
	TenantID     string `json:"tenantId"`
	CampaignID   string `json:"campaignId"`
	Version      int    `json:"version"`
	ApprovedBy   string `json:"approvedBy"`
	ReleaseNotes string `json:"releaseNotes,omitempty"`
}

// PublishResponse represents a publish response.
type PublishResponse struct {
	CampaignID  string `json:"campaignId"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	EffectiveFrom string `json:"effectiveFrom"`
}

// Publish promotes a campaign draft to published.
func (c *Client) Publish(ctx context.Context, req PublishRequest) (*PublishResponse, error) {
	// TODO: Implement HTTP client call

	return nil, fmt.Errorf("not implemented")
}

// HealthResponse represents a health check response.
type HealthResponse struct {
	Status    string `json:"status"`
	Database  string `json:"database"`
	Vector    string `json:"vector"`
	Cache     string `json:"cache"`
}

// Health checks the service health.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	// TODO: Implement HTTP client call to /health

	return &HealthResponse{
		Status: "ok",
	}, nil
}

