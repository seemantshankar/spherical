// Package storage provides spec view queries with cache hints.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SpecViewRepository provides access to the materialized spec view.
type SpecViewRepository struct {
	db DB
}

// NewSpecViewRepository creates a new spec view repository.
func NewSpecViewRepository(db DB) *SpecViewRepository {
	return &SpecViewRepository{db: db}
}

// SpecViewQuery represents a query against the spec view.
type SpecViewQuery struct {
	TenantID          uuid.UUID
	ProductIDs        []uuid.UUID
	CampaignVariantID *uuid.UUID
	Categories        []string
	SpecNames         []string
	Locale            *string
	Trim              *string
	Market            *string
	Limit             int
	Offset            int
}

// SpecViewResult contains the query result with cache hints.
type SpecViewResult struct {
	Specs       []SpecViewLatest
	TotalCount  int
	CacheHint   CacheHint
	ComputedAt  time.Time
}

// CacheHint provides caching guidance for the result.
type CacheHint struct {
	// Cacheable indicates if the result can be cached
	Cacheable bool
	// TTL is the recommended cache duration
	TTL time.Duration
	// Key is the cache key for this result
	Key string
	// Version tracks data freshness
	Version int64
}

// Query executes a spec view query with cache hints.
func (r *SpecViewRepository) Query(ctx context.Context, q SpecViewQuery) (*SpecViewResult, error) {
	// Build the query dynamically
	query := `
		SELECT 
			sv.id, sv.tenant_id, sv.product_id, sv.campaign_variant_id, sv.spec_item_id,
			sv.spec_name, sv.category_name, sv.value, sv.unit, sv.confidence,
			sv.source_doc_id, sv.source_page, sv.version, sv.locale, sv.trim,
			sv.market, sv.product_name
		FROM spec_view_latest sv
		WHERE sv.tenant_id = $1
	`
	args := []interface{}{q.TenantID}
	argIdx := 2

	// Filter by product IDs
	if len(q.ProductIDs) > 0 {
		query += " AND sv.product_id = ANY($" + string('0'+byte(argIdx)) + ")"
		args = append(args, q.ProductIDs)
		argIdx++
	}

	// Filter by campaign variant
	if q.CampaignVariantID != nil {
		query += " AND sv.campaign_variant_id = $" + string('0'+byte(argIdx))
		args = append(args, *q.CampaignVariantID)
		argIdx++
	}

	// Filter by categories
	if len(q.Categories) > 0 {
		query += " AND sv.category_name = ANY($" + string('0'+byte(argIdx)) + ")"
		args = append(args, q.Categories)
		argIdx++
	}

	// Filter by spec names
	if len(q.SpecNames) > 0 {
		query += " AND sv.spec_name = ANY($" + string('0'+byte(argIdx)) + ")"
		args = append(args, q.SpecNames)
		argIdx++
	}

	// Filter by locale
	if q.Locale != nil {
		query += " AND sv.locale = $" + string('0'+byte(argIdx))
		args = append(args, *q.Locale)
		argIdx++
	}

	// Filter by trim
	if q.Trim != nil {
		query += " AND sv.trim = $" + string('0'+byte(argIdx))
		args = append(args, *q.Trim)
		argIdx++
	}

	// Filter by market
	if q.Market != nil {
		query += " AND sv.market = $" + string('0'+byte(argIdx))
		args = append(args, *q.Market)
		argIdx++
	}

	// Add ordering
	query += " ORDER BY sv.category_name, sv.spec_name"

	// Add limit
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	query += " LIMIT $" + string('0'+byte(argIdx))
	args = append(args, limit)
	argIdx++

	// Add offset
	if q.Offset > 0 {
		query += " OFFSET $" + string('0'+byte(argIdx))
		args = append(args, q.Offset)
	}

	// Execute query
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []SpecViewLatest
	var maxVersion int
	for rows.Next() {
		var sv SpecViewLatest
		if err := rows.Scan(
			&sv.ID, &sv.TenantID, &sv.ProductID, &sv.CampaignVariantID, &sv.SpecItemID,
			&sv.SpecName, &sv.CategoryName, &sv.Value, &sv.Unit, &sv.Confidence,
			&sv.SourceDocID, &sv.SourcePage, &sv.Version, &sv.Locale, &sv.Trim,
			&sv.Market, &sv.ProductName,
		); err != nil {
			return nil, err
		}
		specs = append(specs, sv)
		if sv.Version > maxVersion {
			maxVersion = sv.Version
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute cache hint
	cacheHint := r.computeCacheHint(q, maxVersion)

	return &SpecViewResult{
		Specs:      specs,
		TotalCount: len(specs),
		CacheHint:  cacheHint,
		ComputedAt: time.Now(),
	}, nil
}

// GetBySpecItem retrieves a specific spec value by item ID.
func (r *SpecViewRepository) GetBySpecItem(ctx context.Context, tenantID, productID, campaignVariantID, specItemID uuid.UUID) (*SpecViewLatest, error) {
	query := `
		SELECT 
			sv.id, sv.tenant_id, sv.product_id, sv.campaign_variant_id, sv.spec_item_id,
			sv.spec_name, sv.category_name, sv.value, sv.unit, sv.confidence,
			sv.source_doc_id, sv.source_page, sv.version, sv.locale, sv.trim,
			sv.market, sv.product_name
		FROM spec_view_latest sv
		WHERE sv.tenant_id = $1 AND sv.product_id = $2 
			AND sv.campaign_variant_id = $3 AND sv.spec_item_id = $4
	`
	var sv SpecViewLatest
	err := r.db.QueryRowContext(ctx, query, tenantID, productID, campaignVariantID, specItemID).Scan(
		&sv.ID, &sv.TenantID, &sv.ProductID, &sv.CampaignVariantID, &sv.SpecItemID,
		&sv.SpecName, &sv.CategoryName, &sv.Value, &sv.Unit, &sv.Confidence,
		&sv.SourceDocID, &sv.SourcePage, &sv.Version, &sv.Locale, &sv.Trim,
		&sv.Market, &sv.ProductName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sv, nil
}

// SearchByKeyword performs a keyword search across spec names and values.
func (r *SpecViewRepository) SearchByKeyword(ctx context.Context, tenantID uuid.UUID, keyword string, limit int) ([]SpecViewLatest, error) {
	query := `
		SELECT 
			sv.id, sv.tenant_id, sv.product_id, sv.campaign_variant_id, sv.spec_item_id,
			sv.spec_name, sv.category_name, sv.value, sv.unit, sv.confidence,
			sv.source_doc_id, sv.source_page, sv.version, sv.locale, sv.trim,
			sv.market, sv.product_name
		FROM spec_view_latest sv
		WHERE sv.tenant_id = $1 
			AND (UPPER(sv.spec_name) LIKE '%' || UPPER($2) || '%' 
			     OR UPPER(sv.value) LIKE '%' || UPPER($2) || '%'
			     OR UPPER(sv.category_name) LIKE '%' || UPPER($2) || '%')
		ORDER BY sv.confidence DESC, sv.spec_name
		LIMIT $3
	`
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, query, tenantID, keyword, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []SpecViewLatest
	for rows.Next() {
		var sv SpecViewLatest
		if err := rows.Scan(
			&sv.ID, &sv.TenantID, &sv.ProductID, &sv.CampaignVariantID, &sv.SpecItemID,
			&sv.SpecName, &sv.CategoryName, &sv.Value, &sv.Unit, &sv.Confidence,
			&sv.SourceDocID, &sv.SourcePage, &sv.Version, &sv.Locale, &sv.Trim,
			&sv.Market, &sv.ProductName,
		); err != nil {
			return nil, err
		}
		specs = append(specs, sv)
	}
	return specs, rows.Err()
}

// GetCategories returns all unique categories for a tenant.
func (r *SpecViewRepository) GetCategories(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	query := `
		SELECT DISTINCT category_name 
		FROM spec_view_latest 
		WHERE tenant_id = $1 
		ORDER BY category_name
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	return categories, rows.Err()
}

// computeCacheHint determines caching recommendations for a query.
func (r *SpecViewRepository) computeCacheHint(q SpecViewQuery, maxVersion int) CacheHint {
	// Generate cache key based on query parameters
	key := "spec_view:" + q.TenantID.String()
	if len(q.ProductIDs) > 0 {
		key += ":products"
		for _, pid := range q.ProductIDs {
			key += ":" + pid.String()[:8]
		}
	}
	if q.CampaignVariantID != nil {
		key += ":campaign:" + q.CampaignVariantID.String()[:8]
	}

	// Base TTL of 5 minutes for published data
	ttl := 5 * time.Minute

	// Shorter TTL for filtered queries (more specific)
	if len(q.Categories) > 0 || len(q.SpecNames) > 0 {
		ttl = 2 * time.Minute
	}

	return CacheHint{
		Cacheable: true,
		TTL:       ttl,
		Key:       key,
		Version:   int64(maxVersion),
	}
}

// RefreshView triggers a refresh of the materialized view.
func (r *SpecViewRepository) RefreshView(ctx context.Context) error {
	// This would trigger a materialized view refresh in PostgreSQL
	// For SQLite, this is a no-op as we query the base tables
	_, err := r.db.ExecContext(ctx, "SELECT 1") // Placeholder
	return err
}

