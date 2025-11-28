// Package storage provides database models and repositories for the Knowledge Engine.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

	query := `
		INSERT INTO feature_blocks (id, tenant_id, product_id, campaign_variant_id, block_type,
			body, priority, tags, shareability, source_doc_id, source_page, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		block.ID, block.TenantID, block.ProductID, block.CampaignVariantID, block.BlockType,
		block.Body, block.Priority, block.Tags, block.Shareability,
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
		if err := rows.Scan(
			&block.ID, &block.TenantID, &block.ProductID, &block.CampaignVariantID, &block.BlockType,
			&block.Body, &block.Priority, &block.Tags, &block.Shareability,
			&block.SourceDocID, &block.SourcePage, &block.CreatedAt, &block.UpdatedAt,
		); err != nil {
			return nil, err
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

	query := `
		INSERT INTO knowledge_chunks (id, tenant_id, product_id, campaign_variant_id, chunk_type,
			text, metadata, embedding_model, embedding_version, source_doc_id, source_page,
			visibility, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	_, err := r.db.ExecContext(ctx, query,
		chunk.ID, chunk.TenantID, chunk.ProductID, chunk.CampaignVariantID, chunk.ChunkType,
		chunk.Text, chunk.Metadata, chunk.EmbeddingModel, chunk.EmbeddingVersion,
		chunk.SourceDocID, chunk.SourcePage, chunk.Visibility, chunk.CreatedAt, chunk.UpdatedAt,
	)
	return err
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

// Repositories bundles all repositories together.
type Repositories struct {
	Tenants        *TenantRepository
	Products       *ProductRepository
	Campaigns      *CampaignRepository
	SpecValues     *SpecValueRepository
	FeatureBlocks  *FeatureBlockRepository
	KnowledgeChunks *KnowledgeChunkRepository
	Lineage        *LineageRepository
	DriftAlerts    *DriftAlertRepository
}

// NewRepositories creates all repositories with the given database connection.
func NewRepositories(db DB) *Repositories {
	return &Repositories{
		Tenants:        NewTenantRepository(db),
		Products:       NewProductRepository(db),
		Campaigns:      NewCampaignRepository(db),
		SpecValues:     NewSpecValueRepository(db),
		FeatureBlocks:  NewFeatureBlockRepository(db),
		KnowledgeChunks: NewKnowledgeChunkRepository(db),
		Lineage:        NewLineageRepository(db),
		DriftAlerts:    NewDriftAlertRepository(db),
	}
}

// WithTenantScope returns a query helper that enforces tenant scoping.
func WithTenantScope(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant_id = '%s'", tenantID)
}

