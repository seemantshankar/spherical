// Package storage provides database models and repositories for the Knowledge Engine.
package storage

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PlanTier represents tenant subscription tiers.
type PlanTier string

const (
	PlanTierSandbox    PlanTier = "sandbox"
	PlanTierPro        PlanTier = "pro"
	PlanTierEnterprise PlanTier = "enterprise"
)

// CampaignStatus represents the publication status of a campaign.
type CampaignStatus string

const (
	CampaignStatusDraft     CampaignStatus = "draft"
	CampaignStatusPublished CampaignStatus = "published"
	CampaignStatusArchived  CampaignStatus = "archived"
)

// SpecStatus represents the validation status of a spec value.
type SpecStatus string

const (
	SpecStatusActive     SpecStatus = "active"
	SpecStatusConflict   SpecStatus = "conflict"
	SpecStatusDeprecated SpecStatus = "deprecated"
)

// BlockType represents the type of feature block.
type BlockType string

const (
	BlockTypeFeature   BlockType = "feature"
	BlockTypeUSP       BlockType = "usp"
	BlockTypeAccessory BlockType = "accessory"
)

// Shareability represents data sharing levels.
type Shareability string

const (
	ShareabilityPrivate Shareability = "private"
	ShareabilityTenant  Shareability = "tenant"
	ShareabilityPublic  Shareability = "public"
)

// ChunkType represents the type of knowledge chunk.
type ChunkType string

const (
	ChunkTypeSpecRow      ChunkType = "spec_row"
	ChunkTypeSpecFact     ChunkType = "spec_fact"
	ChunkTypeFeatureBlock ChunkType = "feature_block"
	ChunkTypeUSP          ChunkType = "usp"
	ChunkTypeFAQ          ChunkType = "faq"
	ChunkTypeComparison   ChunkType = "comparison"
	ChunkTypeGlobal       ChunkType = "global"
)

// Visibility represents chunk visibility levels.
type Visibility string

const (
	VisibilityPrivate   Visibility = "private"
	VisibilityShared    Visibility = "shared"
	VisibilityBenchmark Visibility = "benchmark"
)

// Verdict represents comparison outcomes.
type Verdict string

const (
	VerdictPrimaryBetter   Verdict = "primary_better"
	VerdictSecondaryBetter Verdict = "secondary_better"
	VerdictEqual           Verdict = "equal"
	VerdictCannotCompare   Verdict = "cannot_compare"
)

// JobStatus represents ingestion job status.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusFailed    JobStatus = "failed"
	JobStatusSucceeded JobStatus = "succeeded"
)

// LineageAction represents audit trail actions.
type LineageAction string

const (
	LineageActionCreated    LineageAction = "created"
	LineageActionUpdated    LineageAction = "updated"
	LineageActionDeleted    LineageAction = "deleted"
	LineageActionReconciled LineageAction = "reconciled"
)

// AlertType represents drift alert types.
type AlertType string

const (
	AlertTypeStaleCampaign    AlertType = "stale_campaign"
	AlertTypeConflictDetected AlertType = "conflict_detected"
	AlertTypeHashChanged      AlertType = "hash_changed"
)

// AlertStatus represents alert workflow status.
type AlertStatus string

const (
	AlertStatusOpen         AlertStatus = "open"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
)

// Tenant represents an OEM or customer account.
type Tenant struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	Name         string          `json:"name" db:"name"`
	PlanTier     PlanTier        `json:"plan_tier" db:"plan_tier"`
	ContactEmail *string         `json:"contact_email,omitempty" db:"contact_email"`
	Settings     json.RawMessage `json:"settings" db:"settings"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// Product represents a make/model-year definition.
type Product struct {
	ID                       uuid.UUID       `json:"id" db:"id"`
	TenantID                 uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name                     string          `json:"name" db:"name"`
	Segment                  *string         `json:"segment,omitempty" db:"segment"`
	BodyType                 *string         `json:"body_type,omitempty" db:"body_type"`
	ModelYear                *int16          `json:"model_year,omitempty" db:"model_year"`
	IsPublicBenchmark        bool            `json:"is_public_benchmark" db:"is_public_benchmark"`
	DefaultCampaignVariantID *uuid.UUID      `json:"default_campaign_variant_id,omitempty" db:"default_campaign_variant_id"`
	Metadata                 json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt                time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at" db:"updated_at"`
}

// CampaignVariant represents a market/trim-specific product slice.
type CampaignVariant struct {
	ID               uuid.UUID      `json:"id" db:"id"`
	ProductID        uuid.UUID      `json:"product_id" db:"product_id"`
	TenantID         uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Locale           string         `json:"locale" db:"locale"`
	Trim             *string        `json:"trim,omitempty" db:"trim"`
	Market           *string        `json:"market,omitempty" db:"market"`
	Status           CampaignStatus `json:"status" db:"status"`
	Version          int            `json:"version" db:"version"`
	EffectiveFrom    *time.Time     `json:"effective_from,omitempty" db:"effective_from"`
	EffectiveThrough *time.Time     `json:"effective_through,omitempty" db:"effective_through"`
	IsDraft          bool           `json:"is_draft" db:"is_draft"`
	LastPublishedBy  *string        `json:"last_published_by,omitempty" db:"last_published_by"`
	CreatedAt        time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" db:"updated_at"`
}

// DocumentSource represents an ingested brochure or document.
type DocumentSource struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	TenantID          uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProductID         uuid.UUID  `json:"product_id" db:"product_id"`
	CampaignVariantID *uuid.UUID `json:"campaign_variant_id,omitempty" db:"campaign_variant_id"`
	StorageURI        string     `json:"storage_uri" db:"storage_uri"`
	SHA256            string     `json:"sha256" db:"sha256"`
	ExtractorVersion  *string    `json:"extractor_version,omitempty" db:"extractor_version"`
	UploadedBy        *string    `json:"uploaded_by,omitempty" db:"uploaded_by"`
	UploadedAt        time.Time  `json:"uploaded_at" db:"uploaded_at"`
}

// SpecCategory represents a spec category (e.g., Engine, Dimensions).
type SpecCategory struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	Description  *string   `json:"description,omitempty" db:"description"`
	DisplayOrder int       `json:"display_order" db:"display_order"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// SpecItem represents a canonical spec item (e.g., Fuel Efficiency).
type SpecItem struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	CategoryID      uuid.UUID       `json:"category_id" db:"category_id"`
	DisplayName     string          `json:"display_name" db:"display_name"`
	Unit            *string         `json:"unit,omitempty" db:"unit"`
	DataType        string          `json:"data_type" db:"data_type"`
	ValidationRules json.RawMessage `json:"validation_rules" db:"validation_rules"`
	Aliases         []string        `json:"aliases" db:"aliases"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}

// SpecValue represents a concrete spec measurement for a campaign.
type SpecValue struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	TenantID            uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProductID           uuid.UUID  `json:"product_id" db:"product_id"`
	CampaignVariantID   uuid.UUID  `json:"campaign_variant_id" db:"campaign_variant_id"`
	SpecItemID          uuid.UUID  `json:"spec_item_id" db:"spec_item_id"`
	ValueNumeric        *float64   `json:"value_numeric,omitempty" db:"value_numeric"`
	ValueText           *string    `json:"value_text,omitempty" db:"value_text"`
	Unit                *string    `json:"unit,omitempty" db:"unit"`
	KeyFeatures         *string    `json:"key_features,omitempty" db:"key_features"`
	VariantAvailability *string    `json:"variant_availability,omitempty" db:"variant_availability"`
	Explanation         *string    `json:"explanation,omitempty" db:"explanation"`
	ExplanationFailed   bool       `json:"explanation_failed" db:"explanation_failed"`
	Confidence          float64    `json:"confidence" db:"confidence"`
	Status              SpecStatus `json:"status" db:"status"`
	SourceDocID         *uuid.UUID `json:"source_doc_id,omitempty" db:"source_doc_id"`
	SourcePage          *int       `json:"source_page,omitempty" db:"source_page"`
	Version             int        `json:"version" db:"version"`
	EffectiveFrom       *time.Time `json:"effective_from,omitempty" db:"effective_from"`
	EffectiveThrough    *time.Time `json:"effective_through,omitempty" db:"effective_through"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// FeatureBlock represents a marketing bullet or USP.
type FeatureBlock struct {
	ID                uuid.UUID    `json:"id" db:"id"`
	TenantID          uuid.UUID    `json:"tenant_id" db:"tenant_id"`
	ProductID         uuid.UUID    `json:"product_id" db:"product_id"`
	CampaignVariantID uuid.UUID    `json:"campaign_variant_id" db:"campaign_variant_id"`
	BlockType         BlockType    `json:"block_type" db:"block_type"`
	Body              string       `json:"body" db:"body"`
	Priority          int16        `json:"priority" db:"priority"`
	Tags              []string     `json:"tags" db:"tags"`
	Shareability      Shareability `json:"shareability" db:"shareability"`
	SourceDocID       *uuid.UUID   `json:"source_doc_id,omitempty" db:"source_doc_id"`
	SourcePage        *int         `json:"source_page,omitempty" db:"source_page"`
	EmbeddingVector   []float32    `json:"embedding_vector,omitempty" db:"embedding_vector"`
	EmbeddingVersion  *string      `json:"embedding_version,omitempty" db:"embedding_version"`
	CreatedAt         time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at" db:"updated_at"`
}

// KnowledgeChunk represents a vectorized text chunk for semantic retrieval.
type KnowledgeChunk struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProductID         uuid.UUID       `json:"product_id" db:"product_id"`
	CampaignVariantID *uuid.UUID      `json:"campaign_variant_id,omitempty" db:"campaign_variant_id"`
	ChunkType         ChunkType       `json:"chunk_type" db:"chunk_type"`
	Text              string          `json:"text" db:"text"`
	Metadata          json.RawMessage `json:"metadata" db:"metadata"`
	ContentHash       *string         `json:"content_hash,omitempty" db:"content_hash"`
	CompletionStatus  string          `json:"completion_status" db:"completion_status"`
	EmbeddingVector   []float32       `json:"embedding_vector,omitempty" db:"embedding_vector"`
	EmbeddingModel    *string         `json:"embedding_model,omitempty" db:"embedding_model"`
	EmbeddingVersion  *string         `json:"embedding_version,omitempty" db:"embedding_version"`
	SourceDocID       *uuid.UUID      `json:"source_doc_id,omitempty" db:"source_doc_id"`
	SourcePage        *int            `json:"source_page,omitempty" db:"source_page"`
	Visibility        Visibility      `json:"visibility" db:"visibility"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
}

// SpecFactChunk represents a semantic spec fact chunk and its embedding.
type SpecFactChunk struct {
	ID                uuid.UUID `json:"id" db:"id"`
	TenantID          uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ProductID         uuid.UUID `json:"product_id" db:"product_id"`
	CampaignVariantID uuid.UUID `json:"campaign_variant_id" db:"campaign_variant_id"`
	SpecValueID       uuid.UUID `json:"spec_value_id" db:"spec_value_id"`
	ChunkText         string    `json:"chunk_text" db:"chunk_text"`
	Gloss             *string   `json:"gloss,omitempty" db:"gloss"`
	EmbeddingVector   []float32 `json:"embedding_vector,omitempty" db:"embedding_vector"`
	EmbeddingModel    *string   `json:"embedding_model,omitempty" db:"embedding_model"`
	EmbeddingVersion  *string   `json:"embedding_version,omitempty" db:"embedding_version"`
	Source            string    `json:"source" db:"source"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// ComparisonRow represents a pre-computed comparison between products.
type ComparisonRow struct {
	ID                    uuid.UUID    `json:"id" db:"id"`
	PrimaryProductID      uuid.UUID    `json:"primary_product_id" db:"primary_product_id"`
	SecondaryProductID    uuid.UUID    `json:"secondary_product_id" db:"secondary_product_id"`
	Dimension             string       `json:"dimension" db:"dimension"`
	PrimaryValue          *string      `json:"primary_value,omitempty" db:"primary_value"`
	SecondaryValue        *string      `json:"secondary_value,omitempty" db:"secondary_value"`
	Verdict               Verdict      `json:"verdict" db:"verdict"`
	Narrative             *string      `json:"narrative,omitempty" db:"narrative"`
	Shareability          Shareability `json:"shareability" db:"shareability"`
	SourcePrimarySpecID   *uuid.UUID   `json:"source_primary_spec_id,omitempty" db:"source_primary_spec_id"`
	SourceSecondarySpecID *uuid.UUID   `json:"source_secondary_spec_id,omitempty" db:"source_secondary_spec_id"`
	ComputedAt            time.Time    `json:"computed_at" db:"computed_at"`
}

// IngestionJob represents a brochure ingestion job.
type IngestionJob struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProductID         uuid.UUID       `json:"product_id" db:"product_id"`
	CampaignVariantID *uuid.UUID      `json:"campaign_variant_id,omitempty" db:"campaign_variant_id"`
	DocumentSourceID  *uuid.UUID      `json:"document_source_id,omitempty" db:"document_source_id"`
	Status            JobStatus       `json:"status" db:"status"`
	ErrorPayload      json.RawMessage `json:"error_payload,omitempty" db:"error_payload"`
	StartedAt         *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	RunBy             *string         `json:"run_by,omitempty" db:"run_by"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
}

// LineageEvent represents an audit trail event.
type LineageEvent struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProductID         *uuid.UUID      `json:"product_id,omitempty" db:"product_id"`
	CampaignVariantID *uuid.UUID      `json:"campaign_variant_id,omitempty" db:"campaign_variant_id"`
	ResourceType      string          `json:"resource_type" db:"resource_type"`
	ResourceID        uuid.UUID       `json:"resource_id" db:"resource_id"`
	DocumentSourceID  *uuid.UUID      `json:"document_source_id,omitempty" db:"document_source_id"`
	IngestionJobID    *uuid.UUID      `json:"ingestion_job_id,omitempty" db:"ingestion_job_id"`
	Action            LineageAction   `json:"action" db:"action"`
	Payload           json.RawMessage `json:"payload" db:"payload"`
	OccurredAt        time.Time       `json:"occurred_at" db:"occurred_at"`
}

// DriftAlert represents a drift monitoring alert.
type DriftAlert struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProductID         *uuid.UUID      `json:"product_id,omitempty" db:"product_id"`
	CampaignVariantID *uuid.UUID      `json:"campaign_variant_id,omitempty" db:"campaign_variant_id"`
	AlertType         AlertType       `json:"alert_type" db:"alert_type"`
	Details           json.RawMessage `json:"details" db:"details"`
	Status            AlertStatus     `json:"status" db:"status"`
	DetectedAt        time.Time       `json:"detected_at" db:"detected_at"`
	ResolvedAt        *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
}

// SpecViewLatest represents the materialized view of latest spec values.
type SpecViewLatest struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	TenantID            uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProductID           uuid.UUID  `json:"product_id" db:"product_id"`
	CampaignVariantID   uuid.UUID  `json:"campaign_variant_id" db:"campaign_variant_id"`
	SpecItemID          uuid.UUID  `json:"spec_item_id" db:"spec_item_id"`
	SpecName            string     `json:"spec_name" db:"spec_name"`
	CategoryName        string     `json:"category_name" db:"category_name"`
	Value               string     `json:"value" db:"value"`
	Unit                *string    `json:"unit,omitempty" db:"unit"`
	KeyFeatures         *string    `json:"key_features,omitempty" db:"key_features"`
	VariantAvailability *string    `json:"variant_availability,omitempty" db:"variant_availability"`
	Explanation         *string    `json:"explanation,omitempty" db:"explanation"`
	ExplanationFailed   bool       `json:"explanation_failed" db:"explanation_failed"`
	Confidence          float64    `json:"confidence" db:"confidence"`
	SourceDocID         *uuid.UUID `json:"source_doc_id,omitempty" db:"source_doc_id"`
	SourcePage          *int       `json:"source_page,omitempty" db:"source_page"`
	Version             int        `json:"version" db:"version"`
	Locale              string     `json:"locale" db:"locale"`
	Trim                *string    `json:"trim,omitempty" db:"trim"`
	Market              *string    `json:"market,omitempty" db:"market"`
	ProductName         string     `json:"product_name" db:"product_name"`
}
