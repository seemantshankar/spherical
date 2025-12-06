package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	pdfextractor "github.com/spherical/pdf-extractor/pkg/extractor"
)

// FindCampaignByMetadata searches for existing campaigns matching the PDF metadata.
// It matches on: make, model, model year, and country code/locale.
func (m *Manager) FindCampaignByMetadata(ctx context.Context, tenantID uuid.UUID, metadata *pdfextractor.DocumentMetadata, locale string) ([]CampaignInfo, error) {
	// First, try to find products by matching name pattern
	query := `
		SELECT 
			cv.id, cv.locale, cv.trim, cv.market, cv.status, cv.created_at,
			p.id as product_id, p.name as product_name, p.metadata as product_metadata, p.model_year
		FROM campaign_variants cv
		JOIN products p ON cv.product_id = p.id
		WHERE cv.tenant_id = $1
	`
	
	args := []interface{}{tenantID}
	conditions := []string{}
	
	// Match by product name (contains make and model)
	if metadata.Make != "Unknown" && metadata.Model != "Unknown" {
		namePattern := fmt.Sprintf("%%%s %s%%", metadata.Make, metadata.Model)
		conditions = append(conditions, "p.name LIKE $"+fmt.Sprintf("%d", len(args)+1))
		args = append(args, namePattern)
	}
	
	// Match by model year if available
	if metadata.ModelYear > 0 {
		conditions = append(conditions, "p.model_year = $"+fmt.Sprintf("%d", len(args)+1))
		args = append(args, metadata.ModelYear)
	}
	
	// Match by locale if provided
	if locale != "" {
		conditions = append(conditions, "cv.locale = $"+fmt.Sprintf("%d", len(args)+1))
		args = append(args, locale)
	}
	
	// Match by country code from product metadata
	if metadata.CountryCode != "Unknown" && metadata.CountryCode != "" {
		// We'll need to check JSON metadata - this is approximate matching
		// For better matching, we'd parse the JSON metadata
		conditions = append(conditions, "p.metadata LIKE $"+fmt.Sprintf("%d", len(args)+1))
		args = append(args, fmt.Sprintf("%%%s%%", metadata.CountryCode))
	}
	
	if len(conditions) > 0 {
		query += " AND (" + strings.Join(conditions, " OR ") + ")"
	}
	
	query += " ORDER BY cv.created_at DESC"
	
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query campaigns by metadata: %w", err)
	}
	defer rows.Close()
	
	var campaigns []CampaignInfo
	for rows.Next() {
		var info CampaignInfo
		var productName string
		var productID uuid.UUID
		var productMetadataJSON sql.NullString
		var modelYear sql.NullInt16
		var createdAtStr string
		
		err := rows.Scan(
			&info.ID, &info.Locale, &info.Trim, &info.Market,
			&info.Status, &createdAtStr, &productID, &productName,
			&productMetadataJSON, &modelYear,
		)
		if err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		if info.CreatedAt, err = parseDBTime(createdAtStr); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		
		info.ProductName = productName
		info.Name = buildCampaignName(productName, info.Locale, info.Trim, info.Market)
		
		// Verify metadata match by checking product metadata JSON
		if productMetadataJSON.Valid && metadataMatch(metadata, productMetadataJSON.String, modelYear) {
			campaigns = append(campaigns, info)
		} else if !productMetadataJSON.Valid {
			// If no metadata JSON, do fuzzy matching on name
			if fuzzyMatch(metadata, productName, modelYear) {
				campaigns = append(campaigns, info)
			}
		}
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaigns: %w", err)
	}
	
	return campaigns, nil
}

// metadataMatch checks if the PDF metadata matches the product metadata stored in JSON.
func metadataMatch(metadata *pdfextractor.DocumentMetadata, productMetadataJSON string, modelYear sql.NullInt16) bool {
	var productMetadata map[string]interface{}
	if err := json.Unmarshal([]byte(productMetadataJSON), &productMetadata); err != nil {
		return false
	}
	
	// Check make
	if makeVal, ok := productMetadata["make"].(string); ok {
		if metadata.Make != "Unknown" && !strings.EqualFold(makeVal, metadata.Make) {
			return false
		}
	}
	
	// Check model
	if modelVal, ok := productMetadata["model"].(string); ok {
		if metadata.Model != "Unknown" && !strings.EqualFold(modelVal, metadata.Model) {
			return false
		}
	}
	
	// Check model year
	if metadata.ModelYear > 0 {
		if modelYear.Valid && int(modelYear.Int16) != metadata.ModelYear {
			return false
		}
		if yearVal, ok := productMetadata["model_year"].(float64); ok {
			if int(yearVal) != metadata.ModelYear {
				return false
			}
		}
	}
	
	// Check country code
	if countryCode, ok := productMetadata["country_code"].(string); ok {
		if metadata.CountryCode != "Unknown" && metadata.CountryCode != "" {
			// Normalize country codes (handle formats like "en-IN" vs "IN")
			normalizedPDF := normalizeCountryCode(metadata.CountryCode)
			normalizedDB := normalizeCountryCode(countryCode)
			if normalizedPDF != normalizedDB {
				return false
			}
		}
	}
	
	return true
}

// fuzzyMatch performs fuzzy matching when metadata JSON is not available.
func fuzzyMatch(metadata *pdfextractor.DocumentMetadata, productName string, modelYear sql.NullInt16) bool {
	productNameLower := strings.ToLower(productName)
	
	// Check if make and model are in product name
	if metadata.Make != "Unknown" {
		if !strings.Contains(productNameLower, strings.ToLower(metadata.Make)) {
			return false
		}
	}
	
	if metadata.Model != "Unknown" {
		if !strings.Contains(productNameLower, strings.ToLower(metadata.Model)) {
			return false
		}
	}
	
	// Check model year
	if metadata.ModelYear > 0 {
		yearStr := fmt.Sprintf("%d", metadata.ModelYear)
		if !strings.Contains(productName, yearStr) {
			if modelYear.Valid && int(modelYear.Int16) != metadata.ModelYear {
				return false
			}
		}
	}
	
	return true
}

// normalizeCountryCode normalizes country codes for comparison (e.g., "en-IN" -> "IN").
func normalizeCountryCode(code string) string {
	if code == "" || code == "Unknown" {
		return ""
	}
	
	// Handle format like "en-IN" -> extract "IN"
	parts := strings.Split(code, "-")
	if len(parts) > 1 {
		return strings.ToUpper(parts[len(parts)-1])
	}
	
	return strings.ToUpper(code)
}

