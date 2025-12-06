package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	pdfextractor "github.com/spherical/pdf-extractor/pkg/extractor"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/campaign"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/extraction"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// handleCreateCampaign implements the create new campaign menu option.
func handleCreateCampaign(ctx context.Context, cfg *orcconfig.Config) error {
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer db.Close()

	tenantID, err := getDefaultTenantID(cfg)
	if err != nil {
		return fmt.Errorf("get tenant: %w", err)
	}

	ui.Section("Create New Campaign")

	// Step 1: Prompt for PDF file
	pdfPath, err := ui.PromptFilePath("Enter path to PDF brochure file")
	if err != nil {
		return fmt.Errorf("PDF path input: %w", err)
	}

	ui.Newline()
	ui.Step("Step 1: Extracting PDF content...")

	// Step 2: Extract PDF
	// Create a context with a longer timeout for extraction (60 minutes)
	// PDF extraction can take a long time, especially for large PDFs with complex pages
	// Each page can take 5-10 minutes, so we allow up to 10 minutes per page
	// Use Background() instead of ctx to avoid inheriting the parent context's timeout
	extractCtx, extractCancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer extractCancel()

	apiKey, err := cfg.GetOpenRouterAPIKey()
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}
	model := cfg.GetLLMModel()
	ui.Info("Using LLM via OpenRouter: %s", model)

	extractor := extraction.NewOrchestrator(apiKey, model)
	outputPath := filepath.Join(cfg.Orchestrator.TempDir, fmt.Sprintf("extracted-%d.md", time.Now().Unix()))

	// Ensure temp directory exists
	if err := ensureDir(cfg.Orchestrator.TempDir); err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	// Show a spinner during extraction to indicate progress
	extractSpinner := ui.NewSpinner("Processing PDF pages...")
	extractSpinner.Start()
	extractResult, err := extractor.Extract(extractCtx, pdfPath, outputPath)
	extractSpinner.Stop()
	if err != nil {
		return fmt.Errorf("PDF extraction failed: %w", err)
	}

	ui.Success("✓ PDF extraction completed in %v", extractResult.Duration.Round(time.Second))
	ui.Info("Extracted markdown saved to: %s", extractResult.MarkdownPath)

	// Step 3: Complete metadata
	ui.Newline()
	ui.Step("Step 2: Collecting product information...")

	completedMetadata, err := campaign.CompleteMetadata(extractResult.Metadata)
	if err != nil {
		return fmt.Errorf("complete metadata: %w", err)
	}

	// Step 4: Check for existing campaigns based on metadata
	ui.Newline()
	ui.Info("Checking for existing campaigns...")

	campaignMgr := campaign.NewManager(db)
	locale := campaign.BuildLocaleFromCountryCode(completedMetadata.CountryCode)
	if locale == "" {
		locale, err = ui.PromptRequired("Enter locale (e.g., en-US, en-IN)")
		if err != nil {
			return fmt.Errorf("locale input: %w", err)
		}
	}

	existingCampaigns, err := campaignMgr.FindCampaignByMetadata(ctx, tenantID, completedMetadata, locale)
	if err != nil {
		ui.Warning("Could not check for existing campaigns: %v. Proceeding with new campaign creation.", err)
		existingCampaigns = []campaign.CampaignInfo{}
	}

	var newCampaign *kestorage.CampaignVariant
	var productID uuid.UUID

	if len(existingCampaigns) > 0 {
		// Existing campaign(s) found - prompt user for action
		ui.Newline()
		ui.Warning("Found %d existing campaign(s) matching this product:", len(existingCampaigns))
		ui.Newline()

		// Display existing campaigns
		for i, camp := range existingCampaigns {
			ui.Info("%d. %s", i+1, camp.Name)
			ui.Info("   Product: %s | Locale: %s | Status: %s | Created: %s",
				camp.ProductName, camp.Locale, camp.Status, camp.CreatedAt.Format("2006-01-02 15:04:05"))
			if camp.Trim != nil && *camp.Trim != "" {
				ui.Info("   Trim: %s", *camp.Trim)
			}
			if camp.Market != nil && *camp.Market != "" {
				ui.Info("   Market: %s", *camp.Market)
			}
		}
		ui.Newline()

		// Prompt user for action
		options := []string{
			"Use existing campaign and skip extraction/ingestion (go directly to query)",
			"Overwrite existing campaign (delete old campaign and redo extraction/ingestion)",
			"Cancel and return to main menu",
		}

		choice, err := ui.PromptChoice("What would you like to do?", options)
		if err != nil {
			return fmt.Errorf("prompt choice: %w", err)
		}

		switch choice {
		case 0: // Use existing campaign
			if len(existingCampaigns) == 1 {
				selectedCampaign := existingCampaigns[0]
				ui.Success("Using existing campaign: %s", selectedCampaign.Name)

				// Get the campaign object
				campaignObj, err := campaignMgr.GetCampaign(ctx, tenantID, selectedCampaign.ID)
				if err != nil {
					return fmt.Errorf("get existing campaign: %w", err)
				}

				// Go directly to query mode
				ui.Newline()
				ui.Success("Skipping extraction and ingestion. Opening query mode...")
				return runQueryMode(ctx, cfg, db, tenantID, campaignObj.ID)
			} else {
				// Multiple campaigns - let user select one
				campaignNames := make([]string, len(existingCampaigns))
				for i, camp := range existingCampaigns {
					campaignNames[i] = camp.Name
				}
				selectedIdx, err := ui.PromptChoice("Select which campaign to use:", campaignNames)
				if err != nil {
					return fmt.Errorf("select campaign: %w", err)
				}

				selectedCampaign := existingCampaigns[selectedIdx]
				ui.Success("Using existing campaign: %s", selectedCampaign.Name)

				campaignObj, err := campaignMgr.GetCampaign(ctx, tenantID, selectedCampaign.ID)
				if err != nil {
					return fmt.Errorf("get existing campaign: %w", err)
				}

				ui.Newline()
				ui.Success("Skipping extraction and ingestion. Opening query mode...")
				return runQueryMode(ctx, cfg, db, tenantID, campaignObj.ID)
			}

		case 1: // Overwrite existing campaign
			ui.Warning("You selected to overwrite the existing campaign(s).")
			confirmed, err := ui.PromptConfirmation(
				fmt.Sprintf("This will DELETE all data for %d campaign(s) and recreate them. This action cannot be undone.", len(existingCampaigns)),
				"OVERWRITE")
			if err != nil || !confirmed {
				ui.Info("Overwrite cancelled. Returning to main menu.")
				return nil
			}

			// Delete all existing campaigns
			ui.Info("Deleting existing campaign(s)...")
			for _, camp := range existingCampaigns {
				if err := campaignMgr.DeleteCampaign(ctx, tenantID, camp.ID); err != nil {
					ui.Warning("Failed to delete campaign %s: %v", camp.Name, err)
				} else {
					ui.Success("Deleted campaign: %s", camp.Name)
				}
			}

			// Continue with normal flow below
			ui.Newline()
			ui.Info("Proceeding with new campaign creation...")

		case 2: // Cancel
			ui.Info("Cancelled. Returning to main menu.")
			return nil

		default:
			return fmt.Errorf("invalid choice: %d", choice)
		}
	}

	// Step 5: Create or find product
	ui.Newline()
	ui.Step("Step 3: Setting up product and campaign...")

	productName := campaign.BuildProductName(completedMetadata)
	productID, err = findOrCreateProduct(ctx, db, tenantID, productName, completedMetadata)
	if err != nil {
		return fmt.Errorf("setup product: %w", err)
	}
	ui.Success("✓ Product ready: %s", productName)

	// Step 6: Create campaign
	// Prompt for optional trim and market
	var trim, market *string
	trimInput, _ := ui.Prompt("Enter trim variant (optional, press Enter to skip)")
	if strings.TrimSpace(trimInput) != "" {
		trim = &trimInput
	}

	marketInput, _ := ui.Prompt("Enter market (optional, press Enter to skip)")
	if strings.TrimSpace(marketInput) != "" {
		market = &marketInput
	}

	newCampaign, err = campaignMgr.CreateCampaign(ctx, tenantID, productID.String(), locale, trim, market)
	if err != nil {
		return fmt.Errorf("create campaign: %w", err)
	}
	ui.Success("✓ Campaign created: %s", newCampaign.ID.String())

	// Step 7: Ingest markdown
	ui.Newline()
	ui.Step("Step 4: Ingesting content into knowledge base...")

	if err := runIngestion(ctx, cfg, db, tenantID, productID, newCampaign.ID, extractResult.MarkdownPath); err != nil {
		return fmt.Errorf("ingestion failed: %w", err)
	}

	ui.Newline()
	ui.Success("Campaign setup complete!")

	// Offer to query
	queryNow, err := ui.Confirm("Would you like to query this campaign now?", false)
	if err == nil && queryNow {
		return runQueryMode(ctx, cfg, db, tenantID, newCampaign.ID)
	}

	return nil
}

// findOrCreateProduct finds an existing product by name or creates a new one.
func findOrCreateProduct(ctx context.Context, db *sql.DB, tenantID uuid.UUID, productName string, metadata *pdfextractor.DocumentMetadata) (uuid.UUID, error) {
	repo := kestorage.NewProductRepository(db)

	// Try to find existing product by name
	products, err := repo.ListByTenant(ctx, tenantID)
	if err == nil {
		for _, p := range products {
			if strings.EqualFold(p.Name, productName) {
				return p.ID, nil
			}
		}
	}

	// Create new product
	product := &kestorage.Product{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      productName,
		ModelYear: nil,
		Segment:   nil,
		BodyType:  nil,
	}

	// Set model year if available
	if metadata.ModelYear > 0 {
		year := int16(metadata.ModelYear)
		product.ModelYear = &year
	}

	// Set segment from domain/subdomain
	if metadata.Domain != "Unknown" && metadata.Domain != "" {
		segment := metadata.Domain
		if metadata.Subdomain != "Unknown" && metadata.Subdomain != "" {
			segment = fmt.Sprintf("%s - %s", segment, metadata.Subdomain)
		}
		product.Segment = &segment
	}

	// Store metadata as JSON
	metadataJSON, err := json.Marshal(map[string]interface{}{
		"domain":       metadata.Domain,
		"subdomain":    metadata.Subdomain,
		"make":         metadata.Make,
		"model":        metadata.Model,
		"condition":    metadata.Condition,
		"model_year":   metadata.ModelYear,
		"country_code": metadata.CountryCode,
	})
	if err == nil {
		product.Metadata = metadataJSON
	}

	if err := repo.Create(ctx, product); err != nil {
		return uuid.Nil, fmt.Errorf("create product: %w", err)
	}

	// Verify the product was created successfully
	createdProduct, err := repo.GetByID(ctx, tenantID, product.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("verify product creation: product was created but cannot be retrieved: %w", err)
	}
	
	// Return the verified product ID
	return createdProduct.ID, nil
}

// ensureDir ensures a directory exists, creating it if necessary.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

