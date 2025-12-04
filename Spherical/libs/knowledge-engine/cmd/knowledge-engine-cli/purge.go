// Package main provides CLI commands for data retention and purging.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// newPurgeCmd creates the purge subcommand.
// This implements T054: Retention/purge tooling that deletes tenant data within 30 days.
func newPurgeCmd() *cobra.Command {
	var (
		tenant        string
		retentionDays int
		dryRun        bool
		operator      string
	)

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge tenant data older than retention period",
		Long: `Purge deletes tenant data that exceeds the retention period (default 30 days).
All deletions are logged to audit trails. Use --dry-run to preview what would be deleted.

Data is deleted in the correct order to respect foreign key constraints:
1. Knowledge chunks (child data)
2. Feature blocks
3. Spec values
4. Comparison rows
5. Campaign variants
6. Products
7. Document sources
8. Ingestion jobs
9. Lineage events (after retention grace period)
10. Drift alerts (after retention grace period)

WARNING: This operation is irreversible. Always use --dry-run first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			if retentionDays <= 0 {
				retentionDays = 30 // Default 30 days
			}

			if operator == "" {
				operator = os.Getenv("USER")
				if operator == "" {
					operator = "cli"
				}
			}

			cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

			logger.Info().
				Str("tenant", tenant).
				Str("tenant_id", tenantID.String()).
				Int("retention_days", retentionDays).
				Str("cutoff_date", cutoffDate.Format(time.RFC3339)).
				Bool("dry_run", dryRun).
				Str("operator", operator).
				Msg("Starting purge operation")

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Create repositories
			repos := storage.NewRepositories(db)

			// Verify tenant exists
			_, err = repos.Tenants.GetByID(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("tenant not found: %w", err)
			}

			// Create audit logger
			auditLogger := monitoring.NewAuditLogger(logger, nil)

			// Create purger
			purger := &dataPurger{
				logger:      logger,
				repos:       repos,
				auditLogger: auditLogger,
				db:          db,
				tenantID:    tenantID,
				cutoffDate:  cutoffDate,
				operator:    operator,
				dryRun:      dryRun,
			}

			// Execute purge
			result, err := purger.Purge(ctx)
			if err != nil {
				return fmt.Errorf("purge failed: %w", err)
			}

			// Output result
			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"tenant_id":      tenantID.String(),
					"retention_days": retentionDays,
					"cutoff_date":    cutoffDate.Format(time.RFC3339),
					"dry_run":        dryRun,
					"deleted": map[string]int{
						"knowledge_chunks": result.KnowledgeChunksDeleted,
						"feature_blocks":   result.FeatureBlocksDeleted,
						"spec_values":      result.SpecValuesDeleted,
						"campaigns":        result.CampaignsDeleted,
						"products":         result.ProductsDeleted,
						"document_sources": result.DocumentSourcesDeleted,
						"ingestion_jobs":   result.IngestionJobsDeleted,
					},
					"total_deleted": result.TotalDeleted,
				})
			}

			if dryRun {
				fmt.Printf("DRY RUN: Would delete data older than %s for tenant %s\n", cutoffDate.Format(time.RFC3339), tenant)
			} else {
				fmt.Printf("✓ Purged data older than %s for tenant %s\n", cutoffDate.Format(time.RFC3339), tenant)
			}
			fmt.Printf("  Knowledge chunks: %d\n", result.KnowledgeChunksDeleted)
			fmt.Printf("  Feature blocks:   %d\n", result.FeatureBlocksDeleted)
			fmt.Printf("  Spec values:      %d\n", result.SpecValuesDeleted)
			fmt.Printf("  Campaigns:        %d\n", result.CampaignsDeleted)
			fmt.Printf("  Products:         %d\n", result.ProductsDeleted)
			fmt.Printf("  Document sources: %d\n", result.DocumentSourcesDeleted)
			fmt.Printf("  Ingestion jobs:   %d\n", result.IngestionJobsDeleted)
			fmt.Printf("  Total deleted:    %d\n", result.TotalDeleted)

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().IntVar(&retentionDays, "retention-days", 30, "retention period in days (default: 30)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview deletions without executing")
	cmd.Flags().StringVar(&operator, "operator", "", "operator name for audit trail")

	_ = cmd.MarkFlagRequired("tenant")

	return cmd
}

// PurgeResult contains the results of a purge operation.
type PurgeResult struct {
	KnowledgeChunksDeleted int
	FeatureBlocksDeleted   int
	SpecValuesDeleted      int
	CampaignsDeleted       int
	ProductsDeleted        int
	DocumentSourcesDeleted int
	IngestionJobsDeleted   int
	TotalDeleted           int
}

// dataPurger handles purging of tenant data.
type dataPurger struct {
	logger      *observability.Logger
	repos       *storage.Repositories
	auditLogger *monitoring.AuditLogger
	db          *sql.DB
	tenantID    uuid.UUID
	cutoffDate  time.Time
	operator    string
	dryRun      bool
}

// Purge executes the purge operation for the tenant.
func (p *dataPurger) Purge(ctx context.Context) (*PurgeResult, error) {
	result := &PurgeResult{}

	// Delete in order to respect foreign key constraints
	// Start with leaf nodes and work up to parent nodes

	// 1. Delete knowledge chunks
	count, err := p.deleteKnowledgeChunks(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete knowledge chunks: %w", err)
	}
	result.KnowledgeChunksDeleted = count

	// 2. Delete feature blocks
	count, err = p.deleteFeatureBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete feature blocks: %w", err)
	}
	result.FeatureBlocksDeleted = count

	// 3. Delete spec values
	count, err = p.deleteSpecValues(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete spec values: %w", err)
	}
	result.SpecValuesDeleted = count

	// 4. Delete comparison rows (if repository exists)
	count, err = p.deleteComparisonRows(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete comparison rows: %w", err)
	}

	// 5. Delete campaigns
	count, err = p.deleteCampaigns(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete campaigns: %w", err)
	}
	result.CampaignsDeleted = count

	// 6. Delete products (only if no active campaigns)
	count, err = p.deleteProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete products: %w", err)
	}
	result.ProductsDeleted = count

	// 7. Delete document sources
	count, err = p.deleteDocumentSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete document sources: %w", err)
	}
	result.DocumentSourcesDeleted = count

	// 8. Delete ingestion jobs
	count, err = p.deleteIngestionJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete ingestion jobs: %w", err)
	}
	result.IngestionJobsDeleted = count

	// 9. Delete lineage events (with longer retention grace period)
	count, err = p.deleteLineageEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete lineage events: %w", err)
	}

	// 10. Delete drift alerts (with longer retention grace period)
	count, err = p.deleteDriftAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete drift alerts: %w", err)
	}

	result.TotalDeleted = result.KnowledgeChunksDeleted +
		result.FeatureBlocksDeleted +
		result.SpecValuesDeleted +
		result.CampaignsDeleted +
		result.ProductsDeleted +
		result.DocumentSourcesDeleted +
		result.IngestionJobsDeleted

	// Log purge completion
	if !p.dryRun {
		if err := p.auditLogger.LogEvent(ctx, monitoring.AuditEvent{
			TenantID:     p.tenantID,
			ResourceType: "purge_operation",
			ResourceID:   uuid.New(),
			Action:       storage.LineageActionDeleted,
			Operator:     p.operator,
			Payload: map[string]interface{}{
				"retention_days":   int(time.Until(p.cutoffDate).Hours() / 24),
				"cutoff_date":      p.cutoffDate.Format(time.RFC3339),
				"knowledge_chunks": result.KnowledgeChunksDeleted,
				"feature_blocks":   result.FeatureBlocksDeleted,
				"spec_values":      result.SpecValuesDeleted,
				"campaigns":        result.CampaignsDeleted,
				"products":         result.ProductsDeleted,
				"document_sources": result.DocumentSourcesDeleted,
				"ingestion_jobs":   result.IngestionJobsDeleted,
				"total_deleted":    result.TotalDeleted,
			},
		}); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to log purge audit event")
		}
	}

	return result, nil
}

// deleteKnowledgeChunks deletes knowledge chunks older than cutoff date.
func (p *dataPurger) deleteKnowledgeChunks(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM knowledge_chunks
		WHERE tenant_id = $1 AND created_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM knowledge_chunks
		WHERE tenant_id = $1 AND created_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted knowledge chunks")
	return int(deleted), nil
}

// deleteFeatureBlocks deletes feature blocks older than cutoff date.
func (p *dataPurger) deleteFeatureBlocks(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM feature_blocks
		WHERE tenant_id = $1 AND created_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM feature_blocks
		WHERE tenant_id = $1 AND created_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted feature blocks")
	return int(deleted), nil
}

// deleteSpecValues deletes spec values older than cutoff date.
func (p *dataPurger) deleteSpecValues(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM spec_values
		WHERE tenant_id = $1 AND created_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM spec_values
		WHERE tenant_id = $1 AND created_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted spec values")
	return int(deleted), nil
}

// deleteComparisonRows deletes comparison rows (if table exists).
func (p *dataPurger) deleteComparisonRows(ctx context.Context) (int, error) {
	// TODO: Implement when ComparisonRepository is available
	return 0, nil
}

// deleteCampaigns deletes campaigns older than cutoff date (only draft/archived).
func (p *dataPurger) deleteCampaigns(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM campaign_variants
		WHERE tenant_id = $1 
		AND created_at < $2
		AND status IN ('draft', 'archived')
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM campaign_variants
		WHERE tenant_id = $1 
		AND created_at < $2
		AND status IN ('draft', 'archived')
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted campaigns")
	return int(deleted), nil
}

// deleteProducts deletes products that have no active campaigns.
func (p *dataPurger) deleteProducts(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM products p
		WHERE p.tenant_id = $1 
		AND p.created_at < $2
		AND NOT EXISTS (
			SELECT 1 FROM campaign_variants cv
			WHERE cv.product_id = p.id
			AND cv.status = 'published'
		)
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM products
		WHERE tenant_id = $1 
		AND created_at < $2
		AND NOT EXISTS (
			SELECT 1 FROM campaign_variants cv
			WHERE cv.product_id = products.id
			AND cv.status = 'published'
		)
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted products")
	return int(deleted), nil
}

// deleteDocumentSources deletes document sources older than cutoff date.
func (p *dataPurger) deleteDocumentSources(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM document_sources
		WHERE tenant_id = $1 AND uploaded_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM document_sources
		WHERE tenant_id = $1 AND uploaded_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted document sources")
	return int(deleted), nil
}

// deleteIngestionJobs deletes ingestion jobs older than cutoff date.
func (p *dataPurger) deleteIngestionJobs(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM ingestion_jobs
		WHERE tenant_id = $1 AND created_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, p.cutoffDate).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM ingestion_jobs
		WHERE tenant_id = $1 AND created_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, p.cutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted ingestion jobs")
	return int(deleted), nil
}

// deleteLineageEvents deletes lineage events older than cutoff date (with grace period).
func (p *dataPurger) deleteLineageEvents(ctx context.Context) (int, error) {
	// Lineage events should be kept longer for audit purposes
	// Use a grace period (e.g., double the retention period)
	retentionDays := int(time.Since(p.cutoffDate).Hours() / 24)
	graceCutoffDate := time.Now().AddDate(0, 0, -(retentionDays * 2))

	query := `
		SELECT COUNT(*) FROM lineage_events
		WHERE tenant_id = $1 AND occurred_at < $2
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, graceCutoffDate).Scan(&count); err != nil {
		// Table might not exist yet
		return 0, nil
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	deleteQuery := `
		DELETE FROM lineage_events
		WHERE tenant_id = $1 AND occurred_at < $2
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, graceCutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted lineage events")
	return int(deleted), nil
}

// deleteDriftAlerts deletes drift alerts older than cutoff date (with grace period).
func (p *dataPurger) deleteDriftAlerts(ctx context.Context) (int, error) {
	// Drift alerts should be kept longer for compliance
	// Use a grace period (e.g., double the retention period)
	retentionDays := int(time.Since(p.cutoffDate).Hours() / 24)
	graceCutoffDate := time.Now().AddDate(0, 0, -(retentionDays * 2))

	query := `
		SELECT COUNT(*) FROM drift_alerts
		WHERE tenant_id = $1 AND detected_at < $2 AND status = 'resolved'
	`
	var count int
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, graceCutoffDate).Scan(&count); err != nil {
		// Table might not exist yet
		return 0, nil
	}

	if count == 0 || p.dryRun {
		return count, nil
	}

	// Only delete resolved alerts
	deleteQuery := `
		DELETE FROM drift_alerts
		WHERE tenant_id = $1 AND detected_at < $2 AND status = 'resolved'
	`
	result, err := p.db.ExecContext(ctx, deleteQuery, p.tenantID, graceCutoffDate)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	p.logger.Info().Int("deleted", int(deleted)).Msg("Deleted drift alerts")
	return int(deleted), nil
}
