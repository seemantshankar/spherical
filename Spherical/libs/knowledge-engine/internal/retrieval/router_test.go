package retrieval

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSpecViewRepo struct {
	results []storage.SpecViewLatest
}

func (m *mockSpecViewRepo) SearchByKeyword(ctx context.Context, tenantID uuid.UUID, keyword string, limit int) ([]storage.SpecViewLatest, error) {
	return m.results, nil
}

func TestRouter_KeywordConfidenceGating(t *testing.T) {
	tenantID := uuid.New()
	productID := uuid.New()
	campaignID := uuid.New()

	highConfSpec := storage.SpecViewLatest{
		SpecItemID:        uuid.New(),
		CategoryName:      "Engine",
		SpecName:          "Power",
		Value:             "200",
		Unit:              stringPtr("hp"),
		Confidence:        0.95,
		CampaignVariantID: campaignID,
	}

	lowConfSpec := highConfSpec
	lowConfSpec.Confidence = 0.5

	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Minimal schema for spec_view_latest path
	schema := []string{
		`CREATE TABLE spec_view_latest (id TEXT, tenant_id TEXT, product_id TEXT, campaign_variant_id TEXT, spec_item_id TEXT, spec_name TEXT, category_name TEXT, value TEXT, unit TEXT, confidence REAL, key_features TEXT, variant_availability TEXT, explanation TEXT, explanation_failed INTEGER, source_doc_id TEXT, source_page INTEGER, version INTEGER, locale TEXT, trim TEXT, market TEXT, product_name TEXT);`,
		`CREATE TABLE spec_values (id TEXT, tenant_id TEXT, product_id TEXT, campaign_variant_id TEXT, spec_item_id TEXT, value_numeric REAL, value_text TEXT, unit TEXT, confidence REAL, explanation TEXT, explanation_failed INTEGER, status TEXT, source_doc_id TEXT, source_page INTEGER, version INTEGER, effective_from TEXT, effective_through TEXT, created_at TEXT, updated_at TEXT, key_features TEXT, variant_availability TEXT);`,
		`CREATE TABLE spec_items (id TEXT, category_id TEXT, display_name TEXT);`,
		`CREATE TABLE spec_categories (id TEXT, name TEXT);`,
		`CREATE TABLE campaign_variants (id TEXT, product_id TEXT, tenant_id TEXT, locale TEXT, trim TEXT, market TEXT, status TEXT);`,
		`CREATE TABLE products (id TEXT, name TEXT);`,
	}
	for _, stmt := range schema {
		_, err = db.Exec(stmt)
		require.NoError(t, err)
	}

	repo := storage.NewSpecViewRepository(db)
	vector := &mockVectorAdapter{}

	run := func(threshold float64, expectVector bool) {
		_, err = db.Exec(`DELETE FROM spec_view_latest; DELETE FROM spec_values; DELETE FROM spec_items; DELETE FROM spec_categories; DELETE FROM campaign_variants; DELETE FROM products;`)
		require.NoError(t, err)

		row := highConfSpec
		if expectVector {
			row = lowConfSpec
		}

		categoryID := uuid.New().String()
		specItemID := row.SpecItemID.String()
		_, err = db.Exec(`INSERT INTO spec_categories (id, name) VALUES (?, ?)`, categoryID, row.CategoryName)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO spec_items (id, category_id, display_name) VALUES (?, ?, ?)`, specItemID, categoryID, row.SpecName)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO products (id, name) VALUES (?, ?)`, productID.String(), "Test Product")
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO campaign_variants (id, product_id, tenant_id, locale, trim, market, status) VALUES (?, ?, ?, 'en-US', '', '', 'published')`, campaignID.String(), productID.String(), tenantID.String())
		require.NoError(t, err)

		_, err = db.Exec(`INSERT INTO spec_values (id, tenant_id, product_id, campaign_variant_id, spec_item_id, value_text, unit, confidence, explanation, explanation_failed, status, version, created_at, updated_at, key_features, variant_availability) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 0, 'active', 1, datetime('now'), datetime('now'), '', '')`,
			row.SpecItemID.String(), tenantID.String(), productID.String(), campaignID.String(), specItemID, row.Value, *row.Unit, row.Confidence)
		require.NoError(t, err)

		_, err = db.Exec(`INSERT INTO spec_view_latest (id, tenant_id, product_id, campaign_variant_id, spec_item_id, spec_name, category_name, value, unit, confidence, key_features, variant_availability, explanation, explanation_failed, source_doc_id, source_page, version, locale, trim, market, product_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', 0, NULL, NULL, 1, 'en-US', '', '', '')`,
			row.SpecItemID.String(), tenantID.String(), productID.String(), campaignID.String(), specItemID, row.SpecName, row.CategoryName, row.Value, *row.Unit, row.Confidence)
		require.NoError(t, err)

		router := NewRouter(
			observability.DefaultLogger(),
			nil,
			vector,
			embedding.NewMockClient(8),
			repo,
			RouterConfig{
				StructuredFirst:            true,
				SemanticFallback:           true,
				KeywordConfidenceThreshold: threshold,
				MaxChunks:                  3,
			},
		)

		vector.called = false
		_, err := router.Query(context.Background(), RetrievalRequest{
			TenantID:          tenantID,
			ProductIDs:        []uuid.UUID{productID},
			CampaignVariantID: &campaignID,
			Question:          "What is the power?",
			MaxChunks:         3,
		})
		assert.NoError(t, err)
		assert.Equal(t, expectVector, vector.called)
	}

	// Lower threshold -> keyword path considered sufficient
	run(0.2, false)

	// Higher threshold -> vector fallback should be invoked
	run(0.8, true)
}

func stringPtr(s string) *string { return &s }

func TestIntentClassifier_Classify(t *testing.T) {
	classifier := NewIntentClassifier()

	tests := []struct {
		question       string
		expectedIntent Intent
		minConfidence  float64
	}{
		// Spec lookup patterns
		{"What is the fuel efficiency of the Camry?", IntentSpecLookup, 0.4},
		{"How much horsepower does it have?", IntentSpecLookup, 0.4},
		{"What's the engine displacement?", IntentSpecLookup, 0.4},
		{"Tell me about the mileage", IntentSpecLookup, 0.4},

		// Comparison patterns
		{"How does Camry compare to Accord?", IntentComparison, 0.8},
		{"Compare the fuel efficiency", IntentComparison, 0.8},
		{"Camry vs Accord", IntentComparison, 0.8},
		{"Which is better, Camry or Accord?", IntentComparison, 0.8},

		// USP patterns
		{"What makes this car unique?", IntentUSPLookup, 0.7},
		{"Why should I buy this?", IntentUSPLookup, 0.7},
		{"What's the best feature?", IntentUSPLookup, 0.7},

		// FAQ patterns
		{"How do I connect my phone?", IntentFAQ, 0.6},
		{"Can I charge wirelessly?", IntentFAQ, 0.6},

		// Fallback/spec-like small-talk patterns now map to spec lookup
		{"Tell me more", IntentSpecLookup, 0.7},
		{"Interesting", IntentSpecLookup, 0.7},
		{"Hello there", IntentSpecLookup, 0.4},
	}

	for _, tc := range tests {
		t.Run(tc.question, func(t *testing.T) {
			intent, confidence := classifier.Classify(tc.question)
			assert.Equal(t, tc.expectedIntent, intent, "Intent mismatch for: %s", tc.question)
			assert.GreaterOrEqual(t, confidence, tc.minConfidence,
				"Confidence too low for: %s (got %f)", tc.question, confidence)
		})
	}
}

func TestVectorFilters_Matching(t *testing.T) {
	// This tests the matchesFilters helper indirectly through the adapter

	t.Run("matches by tenant", func(t *testing.T) {
		// Setup would involve creating entries and filters
		// For now this is a placeholder for when the adapter is more complete
	})
}
