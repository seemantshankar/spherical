// Package main provides CLI commands for comparison management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/comparison"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// newComparisonRecomputeCmd creates the comparison recompute subcommand.
// This implements T045: CLI/ADMIN triggers for recomputing comparisons.
func newComparisonRecomputeCmd() *cobra.Command {
	var (
		tenant    string
		primary   string
		secondary string
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "recompute",
		Short: "Recompute comparisons between two products",
		Long: `Recompute triggers the materialization of comparison data between two products.
This command retrieves current product specs and features, computes comparison rows,
and stores them for fast retrieval. Use --force to recompute even if data exists.

Infrastructure Requirements:
- ComparisonRepository for storing/retrieving comparison rows
- GetByProductID method in CampaignRepository
- GetByID methods in SpecItemRepository and SpecCategoryRepository`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			primaryID, err := resolveID(primary)
			if err != nil {
				return fmt.Errorf("invalid primary product: %w", err)
			}

			secondaryID, err := resolveID(secondary)
			if err != nil {
				return fmt.Errorf("invalid secondary product: %w", err)
			}

			logger.Info().
				Str("tenant", tenant).
				Str("primary", primary).
				Str("secondary", secondary).
				Bool("force", force).
				Msg("Recomputing comparisons")

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Create repositories
			repos := storage.NewRepositories(db)

			// Create comparison store adapter
			// TODO: Replace with proper ComparisonRepository when implemented
			comparisonStore := &comparisonStoreAdapter{repos: repos, db: db}

			// Create in-memory cache for comparisons
			comparisonCache := comparison.NewMemoryComparisonCache()

			// Create materializer
			materializer := comparison.NewMaterializer(
				logger,
				comparisonCache,
				comparisonStore,
				comparison.Config{
					CacheTTL: 1 * time.Hour,
				},
			)

			// Check if comparison already exists
			if !force {
				existing, err := comparisonStore.GetComparison(ctx, tenantID, primaryID, secondaryID)
				if err == nil && len(existing) > 0 {
					if outputJSON {
						enc := json.NewEncoder(os.Stdout)
						enc.SetIndent("", "  ")
						return enc.Encode(map[string]interface{}{
							"status":    "exists",
							"message":   "Comparison already exists. Use --force to recompute.",
							"row_count": len(existing),
						})
					}
					fmt.Printf("Comparison already exists (%d rows). Use --force to recompute.\n", len(existing))
					return nil
				}
			}

			// Compute comparison rows from product data
			// TODO: Implement computeComparisonRows when repository methods are available
			comparisonRows, err := computeComparisonRows(ctx, logger, repos, tenantID, primaryID, secondaryID)
			if err != nil {
				return fmt.Errorf("compute comparisons: %w", err)
			}

			if len(comparisonRows) == 0 {
				if outputJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]interface{}{
						"status":  "no_data",
						"message": "No comparable data found between products",
					})
				}
				fmt.Printf("No comparable data found between products.\n")
				return nil
			}

			// Materialize (store) the comparisons
			if err := materializer.Materialize(ctx, tenantID, primaryID, secondaryID, comparisonRows); err != nil {
				return fmt.Errorf("materialize comparisons: %w", err)
			}

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"status":    "success",
					"row_count": len(comparisonRows),
					"primary":   primaryID.String(),
					"secondary": secondaryID.String(),
				})
			}

			fmt.Printf("✓ Successfully recomputed %d comparison rows\n", len(comparisonRows))
			fmt.Printf("  Primary:   %s\n", primaryID.String())
			fmt.Printf("  Secondary: %s\n", secondaryID.String())

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&primary, "primary", "", "primary product ID (required)")
	cmd.Flags().StringVar(&secondary, "secondary", "", "secondary product ID (required)")
	cmd.Flags().BoolVar(&force, "force", false, "force recomputation even if data exists")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("primary")
	_ = cmd.MarkFlagRequired("secondary")

	return cmd
}

// comparisonStoreAdapter adapts Repositories to ComparisonStore interface.
// TODO: Replace with proper ComparisonRepository implementation.
type comparisonStoreAdapter struct {
	repos *storage.Repositories
	db    interface{} // Store DB for future direct queries if needed
}

// GetComparison retrieves comparison rows from storage.
func (a *comparisonStoreAdapter) GetComparison(ctx context.Context, tenantID, primaryID, secondaryID uuid.UUID) ([]storage.ComparisonRow, error) {
	// TODO: Implement comparison retrieval from database
	// This requires:
	// 1. ComparisonRepository with GetComparison method
	// 2. Database schema for comparison_rows table (if not already exists)
	return nil, fmt.Errorf("comparison retrieval not yet implemented - ComparisonRepository needed")
}

// SaveComparison saves comparison rows to storage.
func (a *comparisonStoreAdapter) SaveComparison(ctx context.Context, rows []storage.ComparisonRow) error {
	// TODO: Implement comparison storage in database
	// This requires:
	// 1. ComparisonRepository with SaveComparison method
	// 2. Database schema for comparison_rows table (if not already exists)
	
	logger.Info().
		Int("row_count", len(rows)).
		Msg("TODO: Save comparison rows to database (ComparisonRepository needed)")
	
	return fmt.Errorf("comparison storage not yet implemented - ComparisonRepository needed")
}

// computeComparisonRows computes comparison rows from product specs and features.
// TODO: Complete implementation when repository methods are available.
func computeComparisonRows(
	ctx context.Context,
	logger *observability.Logger,
	repos *storage.Repositories,
	tenantID, primaryID, secondaryID uuid.UUID,
) ([]comparison.ComparisonRow, error) {
	logger.Info().
		Str("primary_product", primaryID.String()).
		Str("secondary_product", secondaryID.String()).
		Msg("Computing comparison rows")

	// TODO: Implement full comparison computation
	// This requires:
	// 1. CampaignRepository.GetByProductID method
	// 2. SpecItemRepository.GetByID method
	// 3. SpecCategoryRepository.GetByID method
	// 4. Logic to retrieve and compare product specs

	// Placeholder: return empty for now
	logger.Warn().Msg("Comparison computation not yet fully implemented - infrastructure methods needed")
	return []comparison.ComparisonRow{}, nil
}
