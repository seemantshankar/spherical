// Package integration provides integration tests for the Knowledge Engine.
package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/comparison"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// TestComparisonMaterializerJob verifies that comparison materializer jobs work correctly.
// This implements T040: Add integration test for comparison materializer job.
func TestComparisonMaterializerJob(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if Docker is not available
	if os.Getenv("CI") == "" && !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx := context.Background()
	setup := SetupTestContainers(t)
	defer setup.Cleanup()

	// Run migrations
	setup.RunMigrations(t)

	// Setup database connection
	db, err := sql.Open("postgres", setup.PostgresConnStr)
	require.NoError(t, err)
	defer db.Close()

	// Create repositories
	repos := storage.NewRepositories(db)

	// Create test tenant and products
	tenantID := uuid.New()
	primaryProductID := uuid.New()
	secondaryProductID := uuid.New()
	campaignID1 := uuid.New()
	campaignID2 := uuid.New()

	err = createTestTenant(t, ctx, db, tenantID)
	require.NoError(t, err)

	err = createTestProduct(t, ctx, db, tenantID, primaryProductID)
	require.NoError(t, err)

	err = createTestProduct(t, ctx, db, tenantID, secondaryProductID)
	require.NoError(t, err)

	err = createTestCampaign(t, ctx, db, tenantID, primaryProductID, campaignID1)
	require.NoError(t, err)

	err = createTestCampaign(t, ctx, db, tenantID, secondaryProductID, campaignID2)
	require.NoError(t, err)

	// Create logger
	logger := observability.DefaultLogger()

	// Create comparison cache
	comparisonCache := comparison.NewMemoryComparisonCache()

	// Create comparison store adapter
	comparisonStore := &testComparisonStore{
		repos: repos,
	}

	// Create materializer
	materializer := comparison.NewMaterializer(
		logger,
		comparisonCache,
		comparisonStore,
		comparison.Config{
			CacheTTL: 1 * time.Hour,
		},
	)

	// Create test comparison rows
	comparisonRows := []comparison.ComparisonRow{
		{
			ID:                 uuid.New(),
			TenantID:           tenantID,
			PrimaryProductID:   primaryProductID,
			SecondaryProductID: secondaryProductID,
			Dimension:          "Fuel Efficiency",
			PrimaryValue:       "52 mpg",
			SecondaryValue:     "48 mpg",
			Verdict:            storage.VerdictPrimaryBetter,
			Narrative:          "Primary product has better fuel efficiency",
			Shareability:       storage.ShareabilityPrivate,
		},
		{
			ID:                 uuid.New(),
			TenantID:           tenantID,
			PrimaryProductID:   primaryProductID,
			SecondaryProductID: secondaryProductID,
			Dimension:          "Price",
			PrimaryValue:       "$28,000",
			SecondaryValue:     "$30,000",
			Verdict:            storage.VerdictPrimaryBetter,
			Narrative:          "Primary product is more affordable",
			Shareability:       storage.ShareabilityPrivate,
		},
	}

	// Materialize comparisons
	err = materializer.Materialize(ctx, tenantID, primaryProductID, secondaryProductID, comparisonRows)
	require.NoError(t, err)

	// Verify comparisons can be retrieved
	result, err := materializer.Compare(ctx, comparison.ComparisonRequest{
		TenantID:           tenantID,
		PrimaryProductID:   primaryProductID,
		SecondaryProductID: secondaryProductID,
		MaxRows:            10,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)

	// TODO: Verify comparison rows are stored in database
	// This requires ComparisonRepository implementation

	t.Log("Comparison materializer job test completed")
}

