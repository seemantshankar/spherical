package commands

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/campaign"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/orchestrator_factories"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/vector"
)

var (
	queryCampaignID string
	queryQuestion   string
	queryTenantID   string
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query a campaign with natural language questions",
	Long:  "Query a campaign's knowledge base using natural language questions.",
	RunE:  runQuery,
}

func init() {
	queryCmd.Flags().StringVarP(&queryCampaignID, "campaign", "c", "", "Campaign ID (required)")
	queryCmd.Flags().StringVarP(&queryQuestion, "question", "q", "", "Question to ask (optional, will enter interactive mode if not provided)")
	queryCmd.Flags().StringVar(&queryTenantID, "tenant", "", "Tenant ID (optional, defaults to config)")
	queryCmd.MarkFlagRequired("campaign")
	rootCmd.AddCommand(queryCmd)
}

func runQuery(cmd *cobra.Command, args []string) error {
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
	
	// Parse campaign ID
	campaignUUID, err := uuid.Parse(queryCampaignID)
	if err != nil {
		return fmt.Errorf("invalid campaign ID: %w", err)
	}
	
	// Get tenant ID
	var tenantUUID uuid.UUID
	if queryTenantID != "" {
		tenantUUID, err = uuid.Parse(queryTenantID)
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
	
	// If question is provided, execute single query; otherwise enter interactive mode
	if queryQuestion != "" {
		return runSingleQuery(ctx, cfg, db, tenantUUID, campaignUUID, queryQuestion)
	}
	
	// Enter interactive query mode
	return runQueryMode(ctx, cfg, db, tenantUUID, campaignUUID)
}

// runSingleQuery executes a single query and displays results.
func runSingleQuery(ctx context.Context, cfg *orcconfig.Config, db *sql.DB, tenantID, campaignID uuid.UUID, question string) error {
	ui.Section("Query Campaign")
	ui.Info("Campaign: %s", campaignID.String())
	ui.Info("Question: %s", question)
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
	
	// Execute query
	ui.Info("Searching for answers...")
	spinner := ui.NewSpinner("Thinking...")
	spinner.Start()
	
	result, err := queryOrch.Query(ctx, tenantID, campaignInfo.ProductID, campaignID, vectorStore, question)
	spinner.Stop()
	
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
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
			// Normalize tabs to spaces to keep layout clean
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
	
	return nil
}

