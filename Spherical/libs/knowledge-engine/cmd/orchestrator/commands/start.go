package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/startup"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the interactive orchestrator",
	Long:  "Start the interactive orchestrator with menu-driven interface for campaign management.",
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Short-lived context for startup checks
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer startupCancel()

	// Load configuration - cfgFile is for orchestrator config, pass empty string to use defaults
	cfg, err := orcconfig.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	
	// Initialize UI with global flags
	ui.InitUI(noColor, verbose)
	defer ui.Close()
	
	// Display welcome banner
	displayWelcomeBanner()
	
	// Run startup checks
	if err := runStartupChecks(startupCtx, cfg); err != nil {
		return fmt.Errorf("startup checks failed: %w", err)
	}
	
	// Main interactive loop
	for {
		choice := displayMainMenu()

		// Use a fresh context per action to avoid inheriting an expired parent (menu can stay open a long time).
		actionCtx, actionCancel := context.WithTimeout(context.Background(), 60*time.Minute)
		
		switch choice {
		case 1:
			if err := handleQueryCampaign(actionCtx, cfg); err != nil {
				ui.Error("Query failed: %v", err)
				ui.Prompt("\nPress Enter to continue...")
			}
		case 2:
			if err := handleCreateCampaign(actionCtx, cfg); err != nil {
				ui.Error("Failed to create campaign: %v", err)
				ui.Prompt("\nPress Enter to continue...")
			}
		case 3:
			if err := handleDeleteCampaign(actionCtx, cfg); err != nil {
				ui.Error("Failed to delete campaign: %v", err)
				ui.Prompt("\nPress Enter to continue...")
			}
		case 4:
			if err := handleDeleteDatabase(actionCtx, cfg); err != nil {
				ui.Error("Failed to delete database: %v", err)
				ui.Prompt("\nPress Enter to continue...")
			}
		case 5:
			actionCancel()
			displayGoodbye()
			return nil
		default:
			ui.Error("Invalid choice. Please try again.")
		}

		// Cancel action context to clean up timers/handles
		actionCancel()
	}
}

func displayWelcomeBanner() {
	fmt.Println()
	ui.Box("Product Knowledge Orchestrator", `Welcome! Managing product campaigns made easy.`)
	fmt.Println()
}

func runStartupChecks(ctx context.Context, cfg *orcconfig.Config) error {
	ui.Section("Initializing...")
	
	// Check environment
	ui.Info("Checking environment...")
	apiKey, err := cfg.GetOpenRouterAPIKey()
	if err != nil {
		return fmt.Errorf("OPENROUTER_API_KEY not set: %w", err)
	}
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	ui.Success("Environment check passed")
	
	// Check database connectivity
	ui.Info("Verifying database connection...")
	db, err := openDatabase(cfg)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer db.Close()
	ui.Success("Database connected")
	
	// Run migrations
	ui.Info("Checking database migrations...")
	migrationDir := findMigrationDir(cfg)
	driver := cfg.KnowledgeEngine.Database.Driver
	migrationMgr := startup.NewMigrationManager(db, migrationDir, driver)
	status, err := migrationMgr.CheckMigrations(ctx)
	if err != nil {
		return fmt.Errorf("check migrations: %w", err)
	}
	
	if len(status.Pending) > 0 {
		ui.Info("Running %d pending migrations...", len(status.Pending))
		if err := migrationMgr.RunMigrations(ctx, status); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		ui.Success("Migrations completed")
	} else {
		ui.Success("Database is up to date")
	}
	
	// Build CLIs
	ui.Info("Checking CLI tools...")
	cliBuilder := startup.NewCLIBuilder(cfg.Orchestrator.RepoRoot, cfg.Orchestrator.BinDir)
	binaries, err := cliBuilder.CheckBinaries(ctx)
	if err != nil {
		return fmt.Errorf("check binaries: %w", err)
	}
	
	needsBuild := false
	for _, bin := range binaries {
		if bin.NeedsRebuild {
			needsBuild = true
			break
		}
	}
	
	if needsBuild {
		ui.Info("Building CLI tools...")
		if err := cliBuilder.BuildAllNeeded(ctx, binaries); err != nil {
			return fmt.Errorf("build binaries: %w", err)
		}
		ui.Success("CLI tools ready")
	} else {
		ui.Success("CLI tools are up to date")
	}
	
	ui.Newline()
	ui.Success("Ready! Press Enter to continue...")
	_, _ = ui.Prompt("")
	
	return nil
}

func displayMainMenu() int {
	fmt.Println()
	ui.Box("Product Knowledge Orchestrator", `Welcome! What would you like to do?

 1. [Q] Query Existing Campaign
 2. [+] Create New Campaign
 3. [-] Delete Campaign
 4. [X] Delete Entire Database
 5. [E] Exit`)
	fmt.Println()
	
	choice, err := ui.PromptInt("Enter your choice (1-5)")
	if err != nil {
		return 0
	}
	return choice
}

// Handlers are implemented in create_campaign.go and handlers.go

func displayGoodbye() {
	fmt.Println()
	ui.Success("Thank you for using the Product Knowledge Orchestrator!")
	fmt.Println()
}


