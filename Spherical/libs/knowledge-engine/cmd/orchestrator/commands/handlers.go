package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/campaign"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/orchestrator_factories"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/startup"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/vector"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// handleQueryCampaign implements the query existing campaign menu option.
func handleQueryCampaign(ctx context.Context, cfg *orcconfig.Config) error {
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer db.Close()

	tenantID, err := getDefaultTenantID(cfg)
	if err != nil {
		return fmt.Errorf("get tenant: %w", err)
	}

	// List campaigns
	campaignMgr := campaign.NewManager(db)
	campaigns, err := campaignMgr.ListCampaigns(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list campaigns: %w", err)
	}

	if len(campaigns) == 0 {
		ui.Info("No campaigns found. Create a new campaign first.")
		_, _ = ui.Prompt("Press Enter to continue...")
		return nil
	}

	// Display campaigns for selection
	displayCampaigns := make([]ui.CampaignDisplay, len(campaigns))
	for i, c := range campaigns {
		displayCampaigns[i] = ui.CampaignDisplay{
			Number:      i + 1,
			ID:          c.ID,
			Name:        c.Name,
			ProductName: c.ProductName,
			Locale:      c.Locale,
			Trim:        c.Trim,
			Market:      c.Market,
			Status:      string(c.Status),
			CreatedAt:   c.CreatedAt,
		}
	}

	campaignID, err := ui.SelectCampaign(displayCampaigns)
	if err != nil {
		if err.Error() == "cancelled" {
			return nil
		}
		return err
	}

	// Enter interactive query mode
	return runQueryMode(ctx, cfg, db, tenantID, campaignID)
}

// handleDeleteCampaign implements the delete campaign menu option.
func handleDeleteCampaign(ctx context.Context, cfg *orcconfig.Config) error {
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer db.Close()

	tenantID, err := getDefaultTenantID(cfg)
	if err != nil {
		return fmt.Errorf("get tenant: %w", err)
	}

	// List campaigns
	campaignMgr := campaign.NewManager(db)
	campaigns, err := campaignMgr.ListCampaigns(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list campaigns: %w", err)
	}

	if len(campaigns) == 0 {
		ui.Info("No campaigns found.")
		_, _ = ui.Prompt("Press Enter to continue...")
		return nil
	}

	// Display campaigns for selection
	displayCampaigns := make([]ui.CampaignDisplay, len(campaigns))
	for i, c := range campaigns {
		displayCampaigns[i] = ui.CampaignDisplay{
			Number:      i + 1,
			ID:          c.ID,
			Name:        c.Name,
			ProductName: c.ProductName,
			Locale:      c.Locale,
			Trim:        c.Trim,
			Market:      c.Market,
			Status:      string(c.Status),
			CreatedAt:   c.CreatedAt,
		}
	}

	campaignID, err := ui.SelectCampaign(displayCampaigns)
	if err != nil {
		if err.Error() == "cancelled" {
			return nil
		}
		return err
	}

	// Get campaign details for confirmation
	campaignInfo, err := campaignMgr.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("get campaign: %w", err)
	}

	// Build display name
	displayName := buildCampaignDisplayName(campaignInfo)

	// Confirm deletion
	confirmMsg := fmt.Sprintf(`Are you sure you want to delete campaign "%s"?

This will permanently delete:
• All campaign data
• All associated vectors
• All knowledge chunks

This action cannot be undone.`, displayName)

	confirmed, err := ui.PromptConfirmation(confirmMsg, "DELETE")
	if err != nil || !confirmed {
		ui.Info("Deletion cancelled.")
		return nil
	}

	// Delete vector store first
	ui.Info("Deleting vector store...")
	vectorMgr := vector.NewStoreManager(cfg.Orchestrator.VectorStoreRoot, cfg.KnowledgeEngine.Embedding.Dimension)
	if err := vectorMgr.DeleteStore(campaignID); err != nil {
		ui.Warning("Failed to delete vector store (may not exist): %v", err)
	}

	// Delete campaign from database
	ui.Info("Deleting campaign from database...")
	if err := campaignMgr.DeleteCampaign(ctx, tenantID, campaignID); err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}

	ui.Success("Campaign deleted successfully.")
	_, _ = ui.Prompt("Press Enter to continue...")

	return nil
}

// handleDeleteDatabase implements the delete entire database menu option.
func handleDeleteDatabase(ctx context.Context, cfg *orcconfig.Config) error {
	ui.WarningBox("CRITICAL WARNING", `You are about to DELETE ALL DATA!

This action will permanently delete:
✗ ALL campaigns
✗ ALL products
✗ ALL vector stores
✗ ALL knowledge chunks
✗ ALL query history

⚠️  This action CANNOT be undone!`)

	confirmed, err := ui.PromptConfirmation("", "DELETE ALL DATA")
	if err != nil || !confirmed {
		ui.Info("Deletion cancelled.")
		return nil
	}

	ui.Info("Deleting all data...")

	// Delete vector stores first
	ui.Info("Deleting all vector stores...")
	vectorMgr := vector.NewStoreManager(cfg.Orchestrator.VectorStoreRoot, cfg.KnowledgeEngine.Embedding.Dimension)
	if err := vectorMgr.DeleteAllStores(); err != nil {
		ui.Warning("Failed to delete some vector stores: %v", err)
	} else {
		ui.Success("Vector stores deleted.")
	}

	// Delete database file (SQLite) or drop tables (Postgres)
	keCfg := cfg.KnowledgeEngine
	if keCfg.Database.Driver == "sqlite" {
		dbPath := keCfg.Database.SQLite.Path
		if dbPath == "" {
			dbPath = keCfg.DatabaseDSN()
		}

		// Close any existing connections first
		db, err := openDatabase(cfg)
		if err == nil {
			db.Close()
		}

		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete database file: %w", err)
		}
		ui.Success("Database file deleted: %s", dbPath)
	} else {
		// For Postgres, we'd need to drop all tables
		// This is a simplified approach - in production, use proper migration rollback
		ui.Warning("Postgres database deletion requires manual table dropping.")
		ui.Info("Please use database administration tools to drop all tables.")
		return nil
	}

	// Re-run migrations to recreate empty schema
	ui.Info("Recreating database schema...")
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer db.Close()

	migrationDir := findMigrationDir(cfg)
	driver := cfg.KnowledgeEngine.Database.Driver
	migrationMgr := startup.NewMigrationManager(db, migrationDir, driver)
	status, err := migrationMgr.CheckMigrations(ctx)
	if err == nil && len(status.Pending) > 0 {
		if err := migrationMgr.RunMigrations(ctx, status); err != nil {
			return fmt.Errorf("recreate schema: %w", err)
		}
	}

	ui.Success("Database reset complete. Starting fresh!")
	_, _ = ui.Prompt("Press Enter to continue...")

	return nil
}

// Helper functions

func buildCampaignDisplayName(campaign *kestorage.CampaignVariant) string {
	parts := []string{}
	if campaign.Market != nil && *campaign.Market != "" {
		parts = append(parts, fmt.Sprintf("%s Market", *campaign.Market))
	} else {
		parts = append(parts, campaign.Locale)
	}
	return strings.Join(parts, " ")
}

// runQueryMode enters interactive query mode for a campaign.
func runQueryMode(ctx context.Context, cfg *orcconfig.Config, db *sql.DB, tenantID, campaignID uuid.UUID) error {
	ui.Section("Interactive Query Mode")
	ui.Info("Campaign: %s", campaignID.String())
	ui.Newline()

	// Create query orchestrator
	queryOrch, err := orchestrator_factories.NewQueryOrchestrator(cfg, db)
	if err != nil {
		return fmt.Errorf("create query orchestrator: %w", err)
	}

	// Load vector store for this campaign
	vectorMgr := vector.NewStoreManager(cfg.Orchestrator.VectorStoreRoot, cfg.KnowledgeEngine.Embedding.Dimension)
	vectorStore, err := vectorMgr.GetOrCreateStore(campaignID)
	if err != nil {
		return fmt.Errorf("load vector store: %w", err)
	}

	// Get campaign to find product ID
	campaignMgr := campaign.NewManager(db)
	campaignInfo, err := campaignMgr.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("get campaign: %w", err)
	}

	// Interactive query loop
	for {
		question, err := ui.Prompt("Ask a question (or type 'quit' to exit)")
		if err != nil {
			return fmt.Errorf("prompt error: %w", err)
		}

		question = strings.TrimSpace(question)
		if strings.ToLower(question) == "quit" || strings.ToLower(question) == "exit" || strings.ToLower(question) == "q" {
			break
		}

		if question == "" {
			continue
		}

		ui.Info("Searching for answers...")
		spinner := ui.NewSpinner("Thinking...")
		spinner.Start()

		// Execute query using the orchestrator
		result, err := queryOrch.Query(ctx, tenantID, campaignInfo.ProductID, campaignID, vectorStore, question)
		spinner.Stop()

		if err != nil {
			ui.Error("Query failed: %v", err)
			continue
		}

		// Display results
		ui.Newline()
		ui.Section("Answer")

		if len(result.StructuredFacts) > 0 {
			ui.Info("Structured Facts:")
			for i, fact := range result.StructuredFacts {
				confidencePct := fact.Confidence * 100
				value := fact.Value
				if fact.Unit != "" {
					value = fmt.Sprintf("%s %s", value, fact.Unit)
				}
				spec := fmt.Sprintf("%s > %s", fact.Category, fact.Name)
				keyFeatures := strings.TrimSpace(fact.KeyFeatures)
				if keyFeatures == "" {
					keyFeatures = "-"
				}
				varAvail := strings.TrimSpace(fact.VariantAvailability)
				if varAvail == "" {
					varAvail = "-"
				}
				spec = strings.ReplaceAll(spec, "\t", " ")
				value = strings.ReplaceAll(value, "\t", " ")
				keyFeatures = strings.ReplaceAll(keyFeatures, "\t", " ")
				varAvail = strings.ReplaceAll(varAvail, "\t", " ")

				fmt.Printf("--- Fact %d ---\n", i+1)
				fmt.Printf("Spec: %s\n", spec)
				fmt.Printf("Value: %s\n", value)
				fmt.Printf("Key Features: %s\n", keyFeatures)
				fmt.Printf("Variant Availability: %s\n", varAvail)
				fmt.Printf("Confidence: %.1f%%\n\n", confidencePct)
			}
			ui.Newline()
		}

		if len(result.SemanticChunks) > 0 {
			ui.Info("Relevant Information:")
			for i, chunk := range result.SemanticChunks {
				similarityPct := (1.0 - chunk.Distance) * 100
				ui.Box(fmt.Sprintf("Source %d (Similarity: %.1f%%)", i+1, similarityPct), chunk.Text)
				ui.Newline()
			}
		} else if len(result.StructuredFacts) == 0 {
			ui.Warning("No relevant information found for your question.")
		}

		ui.Newline()
	}

	return nil
}

// runIngestion runs the ingestion pipeline for a markdown file.
func runIngestion(ctx context.Context, cfg *orcconfig.Config, db *sql.DB, tenantID, productID, campaignID uuid.UUID, markdownPath string) error {
	ui.Section("Ingesting Content")

	// Create ingestion orchestrator
	ingestionOrch, err := orchestrator_factories.NewIngestionOrchestrator(cfg, db)
	if err != nil {
		return fmt.Errorf("create ingestion orchestrator: %w", err)
	}

	// Show embedding model/provider being used
	embeddingModel := cfg.KnowledgeEngine.Embedding.Model
	if embeddingModel == "" {
		embeddingModel = "google/gemini-embedding-001"
	}
	ui.Info("Using embeddings via OpenRouter: %s", embeddingModel)

	// Create vector store for this campaign
	vectorMgr := vector.NewStoreManager(cfg.Orchestrator.VectorStoreRoot, cfg.KnowledgeEngine.Embedding.Dimension)
	vectorStore, err := vectorMgr.GetOrCreateStore(campaignID)
	if err != nil {
		return fmt.Errorf("create vector store: %w", err)
	}

	// Run ingestion
	spinner := ui.NewSpinner("Processing content and generating embeddings...")
	spinner.Start()

	result, err := ingestionOrch.IngestMarkdown(ctx, tenantID, productID, campaignID, markdownPath, vectorStore)
	spinner.Stop()

	if err != nil {
		return fmt.Errorf("ingestion failed: %w", err)
	}

	// Display results
	ui.Success("✓ Ingestion completed successfully!")
	ui.Newline()
	ui.Section("Ingestion Summary")
	ui.Table([]string{"Metric", "Value"}, [][]string{
		{"Job ID", result.JobID.String()},
		{"Specs Created", fmt.Sprintf("%d", result.SpecsCreated)},
		{"Features Created", fmt.Sprintf("%d", result.FeaturesCreated)},
		{"USPs Created", fmt.Sprintf("%d", result.USPsCreated)},
		{"Chunks Created", fmt.Sprintf("%d", result.ChunksCreated)},
		{"Duration", ui.FormatDuration(result.Duration)},
	})

	return nil
}

