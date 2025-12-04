// Package storage provides database models and repositories for the Knowledge Engine.
package storage

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Common errors
var (
	ErrNotFound      = errors.New("record not found")
	ErrConflict      = errors.New("record conflict")
	ErrInvalidTenant = errors.New("invalid tenant")
)

// DB represents a database connection interface.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// TenantRepository handles tenant CRUD operations.
type TenantRepository struct {
	db DB
}

// NewTenantRepository creates a new tenant repository.
func NewTenantRepository(db DB) *TenantRepository {
	return &TenantRepository{db: db}
}

// Create creates a new tenant.
func (r *TenantRepository) Create(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == uuid.Nil {
		tenant.ID = uuid.New()
	}
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()

	query := `
		INSERT INTO tenants (id, name, plan_tier, contact_email, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		tenant.ID, tenant.Name, tenant.PlanTier, tenant.ContactEmail,
		tenant.Settings, tenant.CreatedAt, tenant.UpdatedAt,
	)
	return err
}

// GetByID retrieves a tenant by ID.
func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	query := `
		SELECT id, name, plan_tier, contact_email, settings, created_at, updated_at
		FROM tenants WHERE id = $1
	`
	tenant := &Tenant{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&tenant.ID, &tenant.Name, &tenant.PlanTier, &tenant.ContactEmail,
		&tenant.Settings, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return tenant, err
}

// GetByName retrieves a tenant by name.
func (r *TenantRepository) GetByName(ctx context.Context, name string) (*Tenant, error) {
	query := `
		SELECT id, name, plan_tier, contact_email, settings, created_at, updated_at
		FROM tenants WHERE name = $1
	`
	tenant := &Tenant{}
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&tenant.ID, &tenant.Name, &tenant.PlanTier, &tenant.ContactEmail,
		&tenant.Settings, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return tenant, err
}

// ProductRepository handles product CRUD operations.
type ProductRepository struct {
	db DB
}

// NewProductRepository creates a new product repository.
func NewProductRepository(db DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// Create creates a new product.
func (r *ProductRepository) Create(ctx context.Context, product *Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	product.CreatedAt = time.Now()
	product.UpdatedAt = time.Now()

	query := `
		INSERT INTO products (id, tenant_id, name, segment, body_type, model_year, 
			is_public_benchmark, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		product.ID, product.TenantID, product.Name, product.Segment, product.BodyType,
		product.ModelYear, product.IsPublicBenchmark, product.Metadata,
		product.CreatedAt, product.UpdatedAt,
	)
	return err
}

// GetByID retrieves a product by ID with tenant scoping.
func (r *ProductRepository) GetByID(ctx context.Context, tenantID, productID uuid.UUID) (*Product, error) {
	query := `
		SELECT id, tenant_id, name, segment, body_type, model_year, 
			is_public_benchmark, default_campaign_variant_id, metadata, created_at, updated_at
		FROM products 
		WHERE id = $1 AND tenant_id = $2
	`
	product := &Product{}
	err := r.db.QueryRowContext(ctx, query, productID, tenantID).Scan(
		&product.ID, &product.TenantID, &product.Name, &product.Segment, &product.BodyType,
		&product.ModelYear, &product.IsPublicBenchmark, &product.DefaultCampaignVariantID,
		&product.Metadata, &product.CreatedAt, &product.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return product, err
}

// ListByTenant lists all products for a tenant.
func (r *ProductRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*Product, error) {
	query := `
		SELECT id, tenant_id, name, segment, body_type, model_year,
			is_public_benchmark, default_campaign_variant_id, metadata, created_at, updated_at
		FROM products
		WHERE tenant_id = $1
		ORDER BY name
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		product := &Product{}
		if err := rows.Scan(
			&product.ID, &product.TenantID, &product.Name, &product.Segment, &product.BodyType,
			&product.ModelYear, &product.IsPublicBenchmark, &product.DefaultCampaignVariantID,
			&product.Metadata, &product.CreatedAt, &product.UpdatedAt,
		); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

// CampaignRepository handles campaign variant CRUD operations.
type CampaignRepository struct {
	db DB
}

// NewCampaignRepository creates a new campaign repository.
func NewCampaignRepository(db DB) *CampaignRepository {
	return &CampaignRepository{db: db}
}

// Create creates a new campaign variant.
func (r *CampaignRepository) Create(ctx context.Context, campaign *CampaignVariant) error {
	if campaign.ID == uuid.Nil {
		campaign.ID = uuid.New()
	}
	campaign.CreatedAt = time.Now()
	campaign.UpdatedAt = time.Now()

	query := `
		INSERT INTO campaign_variants (id, product_id, tenant_id, locale, trim, market, 
			status, version, effective_from, effective_through, is_draft, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		campaign.ID, campaign.ProductID, campaign.TenantID, campaign.Locale,
		campaign.Trim, campaign.Market, campaign.Status, campaign.Version,
		campaign.EffectiveFrom, campaign.EffectiveThrough, campaign.IsDraft,
		campaign.CreatedAt, campaign.UpdatedAt,
	)
	return err
}

// GetByID retrieves a campaign by ID with tenant scoping.
func (r *CampaignRepository) GetByID(ctx context.Context, tenantID, campaignID uuid.UUID) (*CampaignVariant, error) {
	query := `
		SELECT id, product_id, tenant_id, locale, trim, market, status, version,
			effective_from, effective_through, is_draft, last_published_by, created_at, updated_at
		FROM campaign_variants
		WHERE id = $1 AND tenant_id = $2
	`
	campaign := &CampaignVariant{}
	err := r.db.QueryRowContext(ctx, query, campaignID, tenantID).Scan(
		&campaign.ID, &campaign.ProductID, &campaign.TenantID, &campaign.Locale,
		&campaign.Trim, &campaign.Market, &campaign.Status, &campaign.Version,
		&campaign.EffectiveFrom, &campaign.EffectiveThrough, &campaign.IsDraft,
		&campaign.LastPublishedBy, &campaign.CreatedAt, &campaign.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return campaign, err
}

// Update updates a campaign variant.
func (r *CampaignRepository) Update(ctx context.Context, campaign *CampaignVariant) error {
	campaign.UpdatedAt = time.Now()

	query := `
		UPDATE campaign_variants SET
			status = $1, version = $2, effective_from = $3, effective_through = $4,
			is_draft = $5, last_published_by = $6, updated_at = $7
		WHERE id = $8 AND tenant_id = $9
	`
	result, err := r.db.ExecContext(ctx, query,
		campaign.Status, campaign.Version, campaign.EffectiveFrom, campaign.EffectiveThrough,
		campaign.IsDraft, campaign.LastPublishedBy, campaign.UpdatedAt,
		campaign.ID, campaign.TenantID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SpecValueRepository handles spec value CRUD operations.
type SpecValueRepository struct {
	db DB
}

// NewSpecValueRepository creates a new spec value repository.
func NewSpecValueRepository(db DB) *SpecValueRepository {
	return &SpecValueRepository{db: db}
}

// Create creates a new spec value.
func (r *SpecValueRepository) Create(ctx context.Context, spec *SpecValue) error {
	if spec.ID == uuid.Nil {
		spec.ID = uuid.New()
	}
	spec.CreatedAt = time.Now()
	spec.UpdatedAt = time.Now()

	query := `
		INSERT INTO spec_values (id, tenant_id, product_id, campaign_variant_id, spec_item_id,
			value_numeric, value_text, unit, confidence, status, source_doc_id, source_page,
			version, effective_from, effective_through, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	_, err := r.db.ExecContext(ctx, query,
		spec.ID, spec.TenantID, spec.ProductID, spec.CampaignVariantID, spec.SpecItemID,
		spec.ValueNumeric, spec.ValueText, spec.Unit, spec.Confidence, spec.Status,
		spec.SourceDocID, spec.SourcePage, spec.Version, spec.EffectiveFrom, spec.EffectiveThrough,
		spec.CreatedAt, spec.UpdatedAt,
	)
	return err
}

// GetByCampaign retrieves all spec values for a campaign.
func (r *SpecValueRepository) GetByCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) ([]*SpecValue, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, spec_item_id,
			value_numeric, value_text, unit, confidence, status, source_doc_id, source_page,
			version, effective_from, effective_through, created_at, updated_at
		FROM spec_values
		WHERE tenant_id = $1 AND campaign_variant_id = $2 AND status = 'active'
		ORDER BY spec_item_id
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []*SpecValue
	for rows.Next() {
		spec := &SpecValue{}
		if err := rows.Scan(
			&spec.ID, &spec.TenantID, &spec.ProductID, &spec.CampaignVariantID, &spec.SpecItemID,
			&spec.ValueNumeric, &spec.ValueText, &spec.Unit, &spec.Confidence, &spec.Status,
			&spec.SourceDocID, &spec.SourcePage, &spec.Version, &spec.EffectiveFrom, &spec.EffectiveThrough,
			&spec.CreatedAt, &spec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

// GetConflicts retrieves spec values with conflict status for a campaign.
func (r *SpecValueRepository) GetConflicts(ctx context.Context, tenantID, campaignID uuid.UUID) ([]*SpecValue, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, spec_item_id,
			value_numeric, value_text, unit, confidence, status, source_doc_id, source_page,
			version, effective_from, effective_through, created_at, updated_at
		FROM spec_values
		WHERE tenant_id = $1 AND campaign_variant_id = $2 AND status = 'conflict'
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []*SpecValue
	for rows.Next() {
		spec := &SpecValue{}
		if err := rows.Scan(
			&spec.ID, &spec.TenantID, &spec.ProductID, &spec.CampaignVariantID, &spec.SpecItemID,
			&spec.ValueNumeric, &spec.ValueText, &spec.Unit, &spec.Confidence, &spec.Status,
			&spec.SourceDocID, &spec.SourcePage, &spec.Version, &spec.EffectiveFrom, &spec.EffectiveThrough,
			&spec.CreatedAt, &spec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

// FeatureBlockRepository handles feature block CRUD operations.
type FeatureBlockRepository struct {
	db DB
}

// NewFeatureBlockRepository creates a new feature block repository.
func NewFeatureBlockRepository(db DB) *FeatureBlockRepository {
	return &FeatureBlockRepository{db: db}
}

// Create creates a new feature block.
func (r *FeatureBlockRepository) Create(ctx context.Context, block *FeatureBlock) error {
	if block.ID == uuid.Nil {
		block.ID = uuid.New()
	}
	block.CreatedAt = time.Now()
	block.UpdatedAt = time.Now()

	// Serialize tags to JSON for SQLite storage
	var tagsJSON string
	if len(block.Tags) > 0 {
		tagsBytes, err := json.Marshal(block.Tags)
		if err != nil {
			return fmt.Errorf("failed to serialize tags: %w", err)
		}
		tagsJSON = string(tagsBytes)
	} else {
		tagsJSON = "[]"
	}

	query := `
		INSERT INTO feature_blocks (id, tenant_id, product_id, campaign_variant_id, block_type,
			body, priority, tags, shareability, source_doc_id, source_page, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		block.ID, block.TenantID, block.ProductID, block.CampaignVariantID, block.BlockType,
		block.Body, block.Priority, tagsJSON, block.Shareability,
		block.SourceDocID, block.SourcePage, block.CreatedAt, block.UpdatedAt,
	)
	return err
}

// GetByCampaign retrieves all feature blocks for a campaign.
func (r *FeatureBlockRepository) GetByCampaign(ctx context.Context, tenantID, campaignID uuid.UUID, blockType *BlockType) ([]*FeatureBlock, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, block_type,
			body, priority, tags, shareability, source_doc_id, source_page, created_at, updated_at
		FROM feature_blocks
		WHERE tenant_id = $1 AND campaign_variant_id = $2
	`
	args := []interface{}{tenantID, campaignID}

	if blockType != nil {
		query += " AND block_type = $3"
		args = append(args, *blockType)
	}

	query += " ORDER BY priority"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*FeatureBlock
	for rows.Next() {
		block := &FeatureBlock{}
		// Tags are stored as JSON TEXT in SQLite, so scan as string first
		var tagsJSON string
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(
			&block.ID, &block.TenantID, &block.ProductID, &block.CampaignVariantID, &block.BlockType,
			&block.Body, &block.Priority, &tagsJSON, &block.Shareability,
			&block.SourceDocID, &block.SourcePage, &createdAtStr, &updatedAtStr,
		); err != nil {
			return nil, err
		}
		// Deserialize tags from JSON
		if tagsJSON != "" && tagsJSON != "[]" {
			if err := json.Unmarshal([]byte(tagsJSON), &block.Tags); err != nil {
				// If deserialization fails, use empty slice
				block.Tags = []string{}
			}
		} else {
			block.Tags = []string{}
		}
		// Parse date strings to time.Time
		if createdAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
				block.CreatedAt = t
			} else if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				block.CreatedAt = t
			} else {
				block.CreatedAt = time.Now()
			}
		}
		if updatedAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
				block.UpdatedAt = t
			} else if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
				block.UpdatedAt = t
			} else {
				block.UpdatedAt = time.Now()
			}
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

// KnowledgeChunkRepository handles knowledge chunk CRUD operations.
type KnowledgeChunkRepository struct {
	db DB
}

// NewKnowledgeChunkRepository creates a new knowledge chunk repository.
func NewKnowledgeChunkRepository(db DB) *KnowledgeChunkRepository {
	return &KnowledgeChunkRepository{db: db}
}

// Create creates a new knowledge chunk.
func (r *KnowledgeChunkRepository) Create(ctx context.Context, chunk *KnowledgeChunk) error {
	if chunk.ID == uuid.Nil {
		chunk.ID = uuid.New()
	}
	chunk.CreatedAt = time.Now()
	chunk.UpdatedAt = time.Now()

	// Serialize embedding vector to JSON BLOB if present
	var embeddingBlob []byte
	if len(chunk.EmbeddingVector) > 0 {
		// Convert []float32 to []float64 for JSON serialization (JSON doesn't have float32)
		floats64 := make([]float64, len(chunk.EmbeddingVector))
		for i, f := range chunk.EmbeddingVector {
			floats64[i] = float64(f)
		}
		var err error
		embeddingBlob, err = json.Marshal(floats64)
		if err != nil {
			return fmt.Errorf("failed to serialize embedding vector: %w", err)
		}
	}

	query := `
		INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, content_hash, completion_status, embedding_vector, embedding_model, embedding_version, source_doc_id, source_page,
			visibility, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	
	// Set default completion_status if not set
	if chunk.CompletionStatus == "" {
		if len(chunk.EmbeddingVector) > 0 {
			chunk.CompletionStatus = "complete"
		} else {
			chunk.CompletionStatus = "incomplete"
		}
	}
	
	_, err := r.db.ExecContext(ctx, query,
		chunk.ID, chunk.TenantID, chunk.ProductID, chunk.CampaignVariantID, chunk.ChunkType,
		chunk.Text, chunk.Metadata, chunk.ContentHash, chunk.CompletionStatus, embeddingBlob, chunk.EmbeddingModel, chunk.EmbeddingVersion,
		chunk.SourceDocID, chunk.SourcePage, chunk.Visibility, chunk.CreatedAt, chunk.UpdatedAt,
	)
	return err
}

// FindByContentHash finds a chunk by content hash for deduplication lookups.
func (r *KnowledgeChunkRepository) FindByContentHash(ctx context.Context, tenantID uuid.UUID, contentHash string) (*KnowledgeChunk, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, content_hash, completion_status, embedding_vector, embedding_model, embedding_version,
			source_doc_id, source_page, visibility, created_at, updated_at
		FROM knowledge_chunks
		WHERE tenant_id = $1 AND content_hash = $2
		LIMIT 1
	`
	
	chunk := &KnowledgeChunk{}
	var embeddingBlob []byte
	var metadataBlob sql.NullString
	var contentHashPtr sql.NullString
	
	err := r.db.QueryRowContext(ctx, query, tenantID, contentHash).Scan(
		&chunk.ID, &chunk.TenantID, &chunk.ProductID, &chunk.CampaignVariantID, &chunk.ChunkType,
		&chunk.Text, &metadataBlob, &contentHashPtr, &chunk.CompletionStatus, &embeddingBlob,
		&chunk.EmbeddingModel, &chunk.EmbeddingVersion, &chunk.SourceDocID, &chunk.SourcePage,
		&chunk.Visibility, &chunk.CreatedAt, &chunk.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	// Handle NULL metadata
	if metadataBlob.Valid && len(metadataBlob.String) > 0 {
		chunk.Metadata = json.RawMessage(metadataBlob.String)
	} else {
		chunk.Metadata = json.RawMessage("{}")
	}
	
	// Handle NULL content_hash
	if contentHashPtr.Valid {
		chunk.ContentHash = &contentHashPtr.String
	}
	
	// Convert BLOB to []float32
	if len(embeddingBlob) > 0 {
		chunk.EmbeddingVector = blobToFloat32Slice(embeddingBlob)
	}
	
	return chunk, nil
}

// UpdateChunkMetadata updates the metadata of an existing chunk, appending to parsed_spec_ids array.
func (r *KnowledgeChunkRepository) UpdateChunkMetadata(ctx context.Context, chunkID uuid.UUID, metadata json.RawMessage) error {
	query := `
		UPDATE knowledge_chunks
		SET metadata = $1, updated_at = $2
		WHERE id = $3
	`
	_, err := r.db.ExecContext(ctx, query, metadata, time.Now(), chunkID)
	return err
}

// FindIncompleteChunks finds chunks with completion_status='incomplete' for retry processing.
func (r *KnowledgeChunkRepository) FindIncompleteChunks(ctx context.Context, tenantID uuid.UUID, limit int) ([]*KnowledgeChunk, error) {
	if limit <= 0 {
		limit = 100
	}
	
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, content_hash, completion_status, embedding_vector, embedding_model, embedding_version,
			source_doc_id, source_page, visibility, created_at, updated_at
		FROM knowledge_chunks
		WHERE tenant_id = $1 AND completion_status != 'complete'
		ORDER BY created_at ASC
		LIMIT $2
	`
	
	rows, err := r.db.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var chunks []*KnowledgeChunk
	for rows.Next() {
		chunk := &KnowledgeChunk{}
		var embeddingBlob []byte
		var metadataBlob sql.NullString
		var contentHashPtr sql.NullString
		
		if err := rows.Scan(
			&chunk.ID, &chunk.TenantID, &chunk.ProductID, &chunk.CampaignVariantID, &chunk.ChunkType,
			&chunk.Text, &metadataBlob, &contentHashPtr, &chunk.CompletionStatus, &embeddingBlob,
			&chunk.EmbeddingModel, &chunk.EmbeddingVersion, &chunk.SourceDocID, &chunk.SourcePage,
			&chunk.Visibility, &chunk.CreatedAt, &chunk.UpdatedAt,
		); err != nil {
			return nil, err
		}
		
		// Handle NULL metadata
		if metadataBlob.Valid && len(metadataBlob.String) > 0 {
			chunk.Metadata = json.RawMessage(metadataBlob.String)
		} else {
			chunk.Metadata = json.RawMessage("{}")
		}
		
		// Handle NULL content_hash
		if contentHashPtr.Valid {
			chunk.ContentHash = &contentHashPtr.String
		}
		
		// Convert BLOB to []float32
		if len(embeddingBlob) > 0 {
			chunk.EmbeddingVector = blobToFloat32Slice(embeddingBlob)
		}
		
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

// GetByCampaign retrieves all chunks for a campaign.
func (r *KnowledgeChunkRepository) GetByCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) ([]*KnowledgeChunk, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, embedding_model, embedding_version, source_doc_id, source_page,
			visibility, created_at, updated_at
		FROM knowledge_chunks
		WHERE tenant_id = $1 AND campaign_variant_id = $2
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []*KnowledgeChunk
	for rows.Next() {
		chunk := &KnowledgeChunk{}
		if err := rows.Scan(
			&chunk.ID, &chunk.TenantID, &chunk.ProductID, &chunk.CampaignVariantID, &chunk.ChunkType,
			&chunk.Text, &chunk.Metadata, &chunk.EmbeddingModel, &chunk.EmbeddingVersion,
			&chunk.SourceDocID, &chunk.SourcePage, &chunk.Visibility, &chunk.CreatedAt, &chunk.UpdatedAt,
		); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

// GetWithEmbeddingsByTenantAndProducts retrieves chunks with embeddings for a tenant and optional products.
// This is used to load vectors into the FAISS adapter for querying.
func (r *KnowledgeChunkRepository) GetWithEmbeddingsByTenantAndProducts(ctx context.Context, tenantID uuid.UUID, productIDs []uuid.UUID) ([]*KnowledgeChunk, error) {
	var query string
	var args []interface{}
	
	if len(productIDs) == 0 {
		// Get all chunks for tenant
		query = `
			SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
				text, metadata, embedding_vector, embedding_model, embedding_version, 
				source_doc_id, source_page, visibility, created_at, updated_at
			FROM knowledge_chunks
			WHERE tenant_id = $1 
				AND embedding_vector IS NOT NULL 
				AND LENGTH(embedding_vector) > 0
		`
		args = []interface{}{tenantID}
	} else {
		// Get chunks for specific products
		query = `
			SELECT id, tenant_id, product_id, campaign_variant_id, chunk_type,
				text, metadata, embedding_vector, embedding_model, embedding_version,
				source_doc_id, source_page, visibility, created_at, updated_at
			FROM knowledge_chunks
			WHERE tenant_id = $1 
				AND product_id IN (` + buildInClause(len(productIDs)) + `)
				AND embedding_vector IS NOT NULL 
				AND LENGTH(embedding_vector) > 0
		`
		args = make([]interface{}, 1+len(productIDs))
		args[0] = tenantID
		for i, pid := range productIDs {
			args[i+1] = pid
		}
	}
	
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []*KnowledgeChunk
	for rows.Next() {
		chunk := &KnowledgeChunk{}
		var embeddingBlob []byte
		var metadataBlob sql.NullString // Handle NULL metadata
		
		if err := rows.Scan(
			&chunk.ID, &chunk.TenantID, &chunk.ProductID, &chunk.CampaignVariantID, &chunk.ChunkType,
			&chunk.Text, &metadataBlob, &embeddingBlob, &chunk.EmbeddingModel, &chunk.EmbeddingVersion,
			&chunk.SourceDocID, &chunk.SourcePage, &chunk.Visibility, &chunk.CreatedAt, &chunk.UpdatedAt,
		); err != nil {
			return nil, err
		}
		
		// Handle NULL metadata
		if metadataBlob.Valid && len(metadataBlob.String) > 0 {
			chunk.Metadata = json.RawMessage(metadataBlob.String)
		} else {
			chunk.Metadata = json.RawMessage("{}") // Default to empty JSON object
		}
		
		// Convert BLOB to []float32
		if len(embeddingBlob) > 0 {
			chunk.EmbeddingVector = blobToFloat32Slice(embeddingBlob)
		}
		
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

// buildInClause builds a parameterized IN clause string.
func buildInClause(count int) string {
	if count == 0 {
		return ""
	}
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = fmt.Sprintf("$%d", i+2) // Start from $2 since $1 is tenant_id
	}
	return strings.Join(parts, ", ")
}

// blobToFloat32Slice converts a BLOB (byte array) to []float32.
// The BLOB is expected to be a JSON array of floats or binary encoded floats.
func blobToFloat32Slice(blob []byte) []float32 {
	if len(blob) == 0 {
		return nil
	}
	
	// Try JSON first (most common format)
	var floats []float32
	if err := json.Unmarshal(blob, &floats); err == nil {
		return floats
	}
	
	// Try JSON with float64 and convert
	var floats64 []float64
	if err := json.Unmarshal(blob, &floats64); err == nil {
		floats = make([]float32, len(floats64))
		for i, f := range floats64 {
			floats[i] = float32(f)
		}
		return floats
	}
	
	// If JSON fails, try binary format (4 bytes per float32)
	if len(blob)%4 == 0 {
		floats = make([]float32, len(blob)/4)
		for i := 0; i < len(floats); i++ {
			// Read 4 bytes as float32 (little-endian)
			bits := binary.LittleEndian.Uint32(blob[i*4 : (i+1)*4])
			floats[i] = math.Float32frombits(bits)
		}
		return floats
	}
	
	return nil
}

// LineageRepository handles lineage event operations.
type LineageRepository struct {
	db DB
}

// NewLineageRepository creates a new lineage repository.
func NewLineageRepository(db DB) *LineageRepository {
	return &LineageRepository{db: db}
}

// Create creates a new lineage event.
func (r *LineageRepository) Create(ctx context.Context, event *LineageEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}

	query := `
		INSERT INTO lineage_events (id, tenant_id, product_id, campaign_variant_id, resource_type,
			resource_id, document_source_id, ingestion_job_id, action, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.ID, event.TenantID, event.ProductID, event.CampaignVariantID, event.ResourceType,
		event.ResourceID, event.DocumentSourceID, event.IngestionJobID, event.Action,
		event.Payload, event.OccurredAt,
	)
	return err
}

// GetByResource retrieves lineage events for a resource.
func (r *LineageRepository) GetByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) ([]*LineageEvent, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, resource_type,
			resource_id, document_source_id, ingestion_job_id, action, payload, occurred_at
		FROM lineage_events
		WHERE tenant_id = $1 AND resource_type = $2 AND resource_id = $3
		ORDER BY occurred_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*LineageEvent
	for rows.Next() {
		event := &LineageEvent{}
		if err := rows.Scan(
			&event.ID, &event.TenantID, &event.ProductID, &event.CampaignVariantID, &event.ResourceType,
			&event.ResourceID, &event.DocumentSourceID, &event.IngestionJobID, &event.Action,
			&event.Payload, &event.OccurredAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// SaveLineageEvent saves a single lineage event (implements monitoring.LineageStore).
func (r *LineageRepository) SaveLineageEvent(ctx context.Context, event *LineageEvent) error {
	return r.Create(ctx, event)
}

// BatchSaveLineageEvents saves multiple lineage events (implements monitoring.LineageStore).
func (r *LineageRepository) BatchSaveLineageEvents(ctx context.Context, events []LineageEvent) error {
	query := `
		INSERT INTO lineage_events (id, tenant_id, product_id, campaign_variant_id, resource_type,
			resource_id, document_source_id, ingestion_job_id, action, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	
	for _, event := range events {
		if event.ID == uuid.Nil {
			event.ID = uuid.New()
		}
		if event.OccurredAt.IsZero() {
			event.OccurredAt = time.Now()
		}
		
		_, err := r.db.ExecContext(ctx, query,
			event.ID, event.TenantID, event.ProductID, event.CampaignVariantID, event.ResourceType,
			event.ResourceID, event.DocumentSourceID, event.IngestionJobID, event.Action,
			event.Payload, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("batch save lineage event %s: %w", event.ID, err)
		}
	}
	return nil
}

// DriftAlertRepository handles drift alert operations.
type DriftAlertRepository struct {
	db DB
}

// NewDriftAlertRepository creates a new drift alert repository.
func NewDriftAlertRepository(db DB) *DriftAlertRepository {
	return &DriftAlertRepository{db: db}
}

// Create creates a new drift alert.
func (r *DriftAlertRepository) Create(ctx context.Context, alert *DriftAlert) error {
	if alert.ID == uuid.Nil {
		alert.ID = uuid.New()
	}
	if alert.DetectedAt.IsZero() {
		alert.DetectedAt = time.Now()
	}

	query := `
		INSERT INTO drift_alerts (id, tenant_id, product_id, campaign_variant_id, alert_type,
			details, status, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		alert.ID, alert.TenantID, alert.ProductID, alert.CampaignVariantID, alert.AlertType,
		alert.Details, alert.Status, alert.DetectedAt,
	)
	return err
}

// GetOpenByTenant retrieves open alerts for a tenant.
func (r *DriftAlertRepository) GetOpenByTenant(ctx context.Context, tenantID uuid.UUID) ([]*DriftAlert, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, alert_type,
			details, status, detected_at, resolved_at
		FROM drift_alerts
		WHERE tenant_id = $1 AND status = 'open'
		ORDER BY detected_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*DriftAlert
	for rows.Next() {
		alert := &DriftAlert{}
		if err := rows.Scan(
			&alert.ID, &alert.TenantID, &alert.ProductID, &alert.CampaignVariantID, &alert.AlertType,
			&alert.Details, &alert.Status, &alert.DetectedAt, &alert.ResolvedAt,
		); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

// Resolve resolves a drift alert.
func (r *DriftAlertRepository) Resolve(ctx context.Context, alertID uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE drift_alerts SET status = 'resolved', resolved_at = $1
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, now, alertID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DocumentSourceRepository handles document source CRUD operations.
type DocumentSourceRepository struct {
	db DB
}

// NewDocumentSourceRepository creates a new document source repository.
func NewDocumentSourceRepository(db DB) *DocumentSourceRepository {
	return &DocumentSourceRepository{db: db}
}

// Create creates a new document source.
func (r *DocumentSourceRepository) Create(ctx context.Context, doc *DocumentSource) error {
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}

	query := `
		INSERT INTO document_sources (id, tenant_id, product_id, campaign_variant_id,
			storage_uri, sha256, extractor_version, uploaded_by, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query,
		doc.ID, doc.TenantID, doc.ProductID, doc.CampaignVariantID,
		doc.StorageURI, doc.SHA256, doc.ExtractorVersion, doc.UploadedBy, doc.UploadedAt,
	)
	return err
}

// GetByID retrieves a document source by ID.
func (r *DocumentSourceRepository) GetByID(ctx context.Context, id uuid.UUID) (*DocumentSource, error) {
	query := `
		SELECT id, tenant_id, product_id, campaign_variant_id, storage_uri, sha256,
			extractor_version, uploaded_by, uploaded_at
		FROM document_sources WHERE id = $1
	`
	doc := &DocumentSource{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&doc.ID, &doc.TenantID, &doc.ProductID, &doc.CampaignVariantID,
		&doc.StorageURI, &doc.SHA256, &doc.ExtractorVersion, &doc.UploadedBy, &doc.UploadedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

// SpecCategoryRepository handles spec category CRUD operations.
type SpecCategoryRepository struct {
	db DB
}

// NewSpecCategoryRepository creates a new spec category repository.
func NewSpecCategoryRepository(db DB) *SpecCategoryRepository {
	return &SpecCategoryRepository{db: db}
}

// Create creates a new spec category.
func (r *SpecCategoryRepository) Create(ctx context.Context, category *SpecCategory) error {
	if category.ID == uuid.Nil {
		category.ID = uuid.New()
	}
	if category.CreatedAt.IsZero() {
		category.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO spec_categories (id, name, description, display_order, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query,
		category.ID, category.Name, category.Description, category.DisplayOrder, category.CreatedAt,
	)
	return err
}

// GetByName retrieves a spec category by name (case-insensitive lookup).
func (r *SpecCategoryRepository) GetByName(ctx context.Context, name string) (*SpecCategory, error) {
	query := `
		SELECT id, name, description, display_order, created_at
		FROM spec_categories WHERE LOWER(name) = LOWER($1)
	`
	category := &SpecCategory{}
	// SQLite stores dates as TEXT, so we need to scan as string and parse
	var createdAtStr string
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&category.ID, &category.Name, &category.Description, &category.DisplayOrder, &createdAtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Parse the TEXT date string to time.Time
	if createdAtStr != "" {
		// Try multiple date formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		parsed := false
		for _, format := range formats {
			if t, err := time.Parse(format, createdAtStr); err == nil {
				category.CreatedAt = t
				parsed = true
				break
			}
		}
		if !parsed {
			// If parsing fails, use current time as fallback
			category.CreatedAt = time.Now()
		}
	}
	return category, nil
}

// GetOrCreate gets an existing category by name or creates a new one.
func (r *SpecCategoryRepository) GetOrCreate(ctx context.Context, name string) (*SpecCategory, error) {
	category, err := r.GetByName(ctx, name)
	if err == nil {
		return category, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Create new category
	category = &SpecCategory{
		Name:         name,
		DisplayOrder: 0, // Default order
	}
	if err := r.Create(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

// SpecItemRepository handles spec item CRUD operations.
type SpecItemRepository struct {
	db DB
}

// NewSpecItemRepository creates a new spec item repository.
func NewSpecItemRepository(db DB) *SpecItemRepository {
	return &SpecItemRepository{db: db}
}

// Create creates a new spec item.
func (r *SpecItemRepository) Create(ctx context.Context, item *SpecItem) error {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}

	// Serialize aliases to JSON for SQLite storage
	var aliasesJSON string
	if len(item.Aliases) > 0 {
		aliasesBytes, err := json.Marshal(item.Aliases)
		if err != nil {
			return fmt.Errorf("failed to serialize aliases: %w", err)
		}
		aliasesJSON = string(aliasesBytes)
	} else {
		aliasesJSON = "[]"
	}

	query := `
		INSERT INTO spec_items (id, category_id, display_name, unit, data_type, validation_rules, aliases, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		item.ID, item.CategoryID, item.DisplayName, item.Unit, item.DataType,
		item.ValidationRules, aliasesJSON, item.CreatedAt,
	)
	return err
}

// GetByCategoryAndName retrieves a spec item by category and display name.
func (r *SpecItemRepository) GetByCategoryAndName(ctx context.Context, categoryID uuid.UUID, displayName string) (*SpecItem, error) {
	query := `
		SELECT id, category_id, display_name, unit, data_type, validation_rules, aliases, created_at
		FROM spec_items WHERE category_id = $1 AND LOWER(display_name) = LOWER($2)
	`
	item := &SpecItem{}
	err := r.db.QueryRowContext(ctx, query, categoryID, displayName).Scan(
		&item.ID, &item.CategoryID, &item.DisplayName, &item.Unit, &item.DataType,
		&item.ValidationRules, &item.Aliases, &item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return item, err
}

// GetOrCreate gets an existing spec item or creates a new one.
func (r *SpecItemRepository) GetOrCreate(ctx context.Context, categoryID uuid.UUID, displayName string, unit *string) (*SpecItem, error) {
	item, err := r.GetByCategoryAndName(ctx, categoryID, displayName)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Create new spec item
	dataType := "text"
	if unit != nil {
		// If unit suggests numeric, use numeric type
		if *unit != "" {
			dataType = "numeric"
		}
	}

	item = &SpecItem{
		CategoryID:  categoryID,
		DisplayName: displayName,
		Unit:        unit,
		DataType:    dataType,
		Aliases:     []string{},
	}
	if err := r.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// Repositories bundles all repositories together.
type Repositories struct {
	Tenants         *TenantRepository
	Products        *ProductRepository
	Campaigns       *CampaignRepository
	DocumentSources *DocumentSourceRepository
	SpecCategories  *SpecCategoryRepository
	SpecItems       *SpecItemRepository
	SpecValues      *SpecValueRepository
	FeatureBlocks   *FeatureBlockRepository
	KnowledgeChunks *KnowledgeChunkRepository
	Lineage         *LineageRepository
	DriftAlerts     *DriftAlertRepository
}

// NewRepositories creates all repositories with the given database connection.
func NewRepositories(db DB) *Repositories {
	return &Repositories{
		Tenants:         NewTenantRepository(db),
		Products:        NewProductRepository(db),
		Campaigns:       NewCampaignRepository(db),
		DocumentSources: NewDocumentSourceRepository(db),
		SpecCategories:  NewSpecCategoryRepository(db),
		SpecItems:       NewSpecItemRepository(db),
		SpecValues:      NewSpecValueRepository(db),
		FeatureBlocks:   NewFeatureBlockRepository(db),
		KnowledgeChunks: NewKnowledgeChunkRepository(db),
		Lineage:         NewLineageRepository(db),
		DriftAlerts:     NewDriftAlertRepository(db),
	}
}

// WithTenantScope returns a query helper that enforces tenant scoping.
func WithTenantScope(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant_id = '%s'", tenantID)
}

