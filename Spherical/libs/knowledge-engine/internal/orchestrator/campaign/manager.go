// Package campaign provides campaign management functionality.
package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// parseDBTime parses timestamps returned by SQLite or RFC3339 formats.
func parseDBTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time: %s", s)
}

// Manager handles campaign CRUD operations.
type Manager struct {
	db    *sql.DB
	repo  *kestorage.CampaignRepository
	prepo *kestorage.ProductRepository
}

// NewManager creates a new campaign manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db:    db,
		repo:  kestorage.NewCampaignRepository(db),
		prepo: kestorage.NewProductRepository(db),
	}
}

// CampaignInfo represents campaign information for display.
type CampaignInfo struct {
	ID          uuid.UUID
	Name        string
	ProductName string
	Locale      string
	Trim        *string
	Market      *string
	Status      kestorage.CampaignStatus
	CreatedAt   time.Time
}

// ListCampaigns lists all campaigns for a tenant.
func (m *Manager) ListCampaigns(ctx context.Context, tenantID uuid.UUID) ([]CampaignInfo, error) {
	query := `
		SELECT 
			cv.id, cv.locale, cv.trim, cv.market, cv.status, cv.created_at,
			p.name as product_name
		FROM campaign_variants cv
		JOIN products p ON cv.product_id = p.id
		WHERE cv.tenant_id = $1
		ORDER BY cv.created_at DESC
	`
	
	rows, err := m.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query campaigns: %w", err)
	}
	defer rows.Close()
	
	var campaigns []CampaignInfo
	for rows.Next() {
		var info CampaignInfo
		var productName string
		var createdAtStr string
		
		err := rows.Scan(
			&info.ID, &info.Locale, &info.Trim, &info.Market,
			&info.Status, &createdAtStr, &productName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		if info.CreatedAt, err = parseDBTime(createdAtStr); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		
		info.ProductName = productName
		info.Name = buildCampaignName(productName, info.Locale, info.Trim, info.Market)
		
		campaigns = append(campaigns, info)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaigns: %w", err)
	}
	
	return campaigns, nil
}

// buildCampaignName creates a friendly display name for a campaign.
func buildCampaignName(product, locale string, trim, market *string) string {
	parts := []string{product}
	
	if market != nil && *market != "" {
		parts = append(parts, fmt.Sprintf("(%s Market)", *market))
	} else if locale != "" {
		parts = append(parts, fmt.Sprintf("(%s)", locale))
	}
	
	if trim != nil && *trim != "" {
		parts = append(parts, *trim)
	}
	
	return strings.Join(parts, " ")
}

// GetCampaign retrieves a campaign by ID.
func (m *Manager) GetCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) (*kestorage.CampaignVariant, error) {
	return m.repo.GetByID(ctx, tenantID, campaignID)
}

// CreateCampaign creates a new campaign.
func (m *Manager) CreateCampaign(ctx context.Context, tenantID uuid.UUID, productIDOrName string, locale string, trim, market *string) (*kestorage.CampaignVariant, error) {
	// Resolve product ID
	productID, err := m.resolveProductID(ctx, tenantID, productIDOrName)
	if err != nil {
		return nil, fmt.Errorf("resolve product: %w", err)
	}
	
	// Create campaign
	campaign := &kestorage.CampaignVariant{
		ID:        uuid.New(),
		TenantID:  tenantID,
		ProductID: productID,
		Locale:    locale,
		Trim:      trim,
		Market:    market,
		Status:    kestorage.CampaignStatusDraft,
		Version:   1,
		IsDraft:   true,
	}
	
	err = m.repo.Create(ctx, campaign)
	if err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}
	
	return campaign, nil
}

// DeleteCampaign deletes a campaign and all its associated data.
func (m *Manager) DeleteCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) error {
	// Verify campaign exists
	_, err := m.repo.GetByID(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("campaign not found: %w", err)
	}
	
	// Delete campaign (cascade will handle related data if foreign keys are set up)
	// Otherwise, we need to delete related data manually
	// For now, assume cascade deletion
	deleteQuery := `DELETE FROM campaign_variants WHERE id = $1 AND tenant_id = $2`
	_, err = m.db.ExecContext(ctx, deleteQuery, campaignID, tenantID)
	if err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}
	
	return nil
}

// resolveProductID resolves a product ID or name to a UUID.
func (m *Manager) resolveProductID(ctx context.Context, tenantID uuid.UUID, productIDOrName string) (uuid.UUID, error) {
	// Try to parse as UUID
	if id, err := uuid.Parse(productIDOrName); err == nil {
		// Verify it exists - if it's a UUID, we should find it by ID
		product, err := m.prepo.GetByID(ctx, tenantID, id)
		if err == nil {
			return product.ID, nil
		}
		// If it's a UUID but not found, return the error immediately
		// Don't fall through to name lookup for UUIDs
		return uuid.Nil, fmt.Errorf("product not found: %s", productIDOrName)
	}
	
	// Not a UUID, try to find by name
	query := `SELECT id FROM products WHERE tenant_id = $1 AND (name = $2 OR name LIKE $3) LIMIT 1`
	var productID uuid.UUID
	err := m.db.QueryRowContext(ctx, query, tenantID, productIDOrName, productIDOrName+"%").Scan(&productID)
	if err == nil {
		return productID, nil
	}
	
	// Try case-insensitive
	err = m.db.QueryRowContext(ctx, 
		`SELECT id FROM products WHERE tenant_id = $1 AND (LOWER(name) = LOWER($2) OR LOWER(name) LIKE LOWER($3)) LIMIT 1`,
		tenantID, productIDOrName, productIDOrName+"%").Scan(&productID)
	if err == nil {
		return productID, nil
	}
	
	return uuid.Nil, fmt.Errorf("product not found: %s", productIDOrName)
}

