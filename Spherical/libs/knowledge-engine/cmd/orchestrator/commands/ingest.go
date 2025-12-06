package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/campaign"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
)

var (
	ingestCampaignID   string
	ingestMarkdownPath string
	ingestProductID    string
	ingestTenantID     string
)

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingest markdown content into a campaign",
	Long:  "Ingest structured markdown content into a campaign's knowledge base.",
	RunE:  runIngest,
}

func init() {
	ingestCmd.Flags().StringVar(&ingestCampaignID, "campaign", "", "Campaign ID (required)")
	ingestCmd.Flags().StringVar(&ingestMarkdownPath, "markdown", "", "Path to markdown file (required)")
	ingestCmd.Flags().StringVar(&ingestProductID, "product", "", "Product ID (optional, will be inferred from campaign)")
	ingestCmd.Flags().StringVar(&ingestTenantID, "tenant", "", "Tenant ID (optional, defaults to config)")
	ingestCmd.MarkFlagRequired("campaign")
	ingestCmd.MarkFlagRequired("markdown")
	rootCmd.AddCommand(ingestCmd)
}

func runIngest(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// Load configuration
	cfg, err := orcconfig.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	
	// Initialize UI
	ui.InitUI(noColor, verbose)
	defer ui.Close()
	
	ui.Section("Content Ingestion")
	
	// Parse campaign ID
	campaignUUID, err := uuid.Parse(ingestCampaignID)
	if err != nil {
		return fmt.Errorf("invalid campaign ID: %w", err)
	}
	
	// Get tenant ID
	var tenantUUID uuid.UUID
	if ingestTenantID != "" {
		tenantUUID, err = uuid.Parse(ingestTenantID)
		if err != nil {
			return fmt.Errorf("invalid tenant ID: %w", err)
		}
	} else {
		tenantUUID, err = getDefaultTenantID(cfg)
		if err != nil {
			return fmt.Errorf("get tenant: %w", err)
		}
	}
	
	// Open database connection
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer db.Close()
	
	// Get product ID from campaign if not provided
	productUUID := uuid.Nil
	if ingestProductID != "" {
		productUUID, err = uuid.Parse(ingestProductID)
		if err != nil {
			return fmt.Errorf("invalid product ID: %w", err)
		}
	} else {
		// Get product ID from campaign
		campaignMgr := campaign.NewManager(db)
		campaignInfo, err := campaignMgr.GetCampaign(ctx, tenantUUID, campaignUUID)
		if err != nil {
			return fmt.Errorf("get campaign: %w", err)
		}
		productUUID = campaignInfo.ProductID
	}
	
	ui.Info("Campaign: %s", campaignUUID.String())
	ui.Info("Product: %s", productUUID.String())
	ui.Info("Markdown file: %s", ingestMarkdownPath)
	ui.Newline()
	
	// Run ingestion using the helper function
	if err := runIngestion(ctx, cfg, db, tenantUUID, productUUID, campaignUUID, ingestMarkdownPath); err != nil {
		return fmt.Errorf("ingestion failed: %w", err)
	}
	
	return nil
}

