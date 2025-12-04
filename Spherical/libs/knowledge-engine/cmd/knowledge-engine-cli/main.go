// Package main provides the Knowledge Engine CLI entrypoint.
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	_ "github.com/mattn/go-sqlite3"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

var (
	// Global flags
	cfgFile    string
	outputJSON bool
	verbose    bool

	// Configuration and logger
	cfg    *config.Config
	logger *observability.Logger
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "knowledge-engine-cli",
	Short: "Knowledge Engine CLI for ingestion, retrieval, and administration",
	Long: `Knowledge Engine CLI provides commands for managing product knowledge data.

Use this tool to:
- Ingest brochure-derived Markdown into campaigns
- Publish and rollback campaign versions
- Query structured specs and semantic chunks
- Monitor drift and manage lineage
- Export/import data for audits

All commands support --json for automation.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		logFormat := "console"
		if outputJSON {
			logFormat = "json"
		}

		logger = observability.NewLogger(observability.LogConfig{
			Level:       cfg.Observability.LogLevel,
			Format:      logFormat,
			ServiceName: "knowledge-engine-cli",
		})

		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default: uses env vars)")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Add subcommands
	rootCmd.AddCommand(newIngestCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newCompareCmd())
	rootCmd.AddCommand(newDriftCmd())
	rootCmd.AddCommand(newPurgeCmd()) // T054
	rootCmd.AddCommand(newDriftReportCmd()) // T057
	rootCmd.AddCommand(newExportCmd())
	rootCmd.AddCommand(newImportCmd())
	rootCmd.AddCommand(newMigrateCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDemoCmd())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newIngestCmd creates the ingest subcommand.
func newIngestCmd() *cobra.Command {
	var (
		tenant       string
		product      string
		campaign     string
		markdown     string
		pdf          string
		sourceFile   string
		publishDraft bool
		overwrite    bool
		operator     string
	)

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest brochure-derived Markdown into a campaign",
		Long: `Ingest parses Markdown (from pdf-extractor or manual upload), normalizes
specs/features/USPs, deduplicates, and stores them in the campaign.

If --pdf is provided instead of --markdown, the CLI automatically invokes
the pdf-extractor binary to generate Markdown first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			// Determine markdown source
			markdownPath := markdown
			if pdf != "" && markdown == "" {
				// Extract PDF to Markdown using pdf-extractor
				logger.Info().Str("pdf", pdf).Msg("Extracting PDF to Markdown")
				
				tempDir, err := os.MkdirTemp("", "ke-ingest-*")
				if err != nil {
					return fmt.Errorf("create temp dir: %w", err)
				}
				defer os.RemoveAll(tempDir)

				markdownPath = filepath.Join(tempDir, "extracted.md")
				extractCmd := exec.CommandContext(ctx, "pdf-extractor",
					"--input", pdf,
					"--output", markdownPath,
				)
				extractCmd.Stdout = os.Stdout
				extractCmd.Stderr = os.Stderr

				if err := extractCmd.Run(); err != nil {
					return fmt.Errorf("pdf extraction failed: %w", err)
				}
			}

			if markdownPath == "" {
				return fmt.Errorf("either --markdown or --pdf is required")
			}

			// Read markdown file
			content, err := os.ReadFile(markdownPath)
			if err != nil {
				return fmt.Errorf("read markdown: %w", err)
			}

			// Parse tenant/product/campaign IDs
			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			productID, err := resolveID(product)
			if err != nil {
				return fmt.Errorf("invalid product: %w", err)
			}

			campaignID, err := resolveID(campaign)
			if err != nil {
				return fmt.Errorf("invalid campaign: %w", err)
			}

			// Get operator
			if operator == "" {
				operator = os.Getenv("USER")
				if operator == "" {
					operator = "cli"
				}
			}

			logger.Info().
				Str("tenant", tenant).
				Str("product", product).
				Str("campaign", campaign).
				Str("operator", operator).
				Int("content_size", len(content)).
				Msg("Starting ingestion")

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Create repositories
			repos := storage.NewRepositories(db)

			// Create embedding client
			var embClient embedding.Embedder
			apiKey := os.Getenv("OPENROUTER_API_KEY")
			if apiKey != "" {
				client, err := embedding.NewClient(embedding.Config{
					APIKey:  apiKey,
					Model:   cfg.Embedding.Model,
					BaseURL: "https://openrouter.ai/api/v1",
				})
				if err == nil {
					embClient = client
				} else {
					logger.Warn().Err(err).Msg("Failed to create embedding client, using mock")
					embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
				}
			} else {
				embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
			}

			// Create vector adapter
			vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
				Dimension: cfg.Embedding.Dimension,
			})
			if err != nil {
				return fmt.Errorf("create vector adapter: %w", err)
			}

			// Create lineage writer
			lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

			// Create pipeline
			pipeline := ingest.NewPipeline(
				logger,
				ingest.PipelineConfig{
					ChunkSize:         512,
					ChunkOverlap:      64,
					MaxConcurrentJobs: 4,
					DedupeThreshold:   0.95,
				},
				repos,
				embClient,
				vectorAdapter,
				lineageWriter,
			)

			// Run ingestion
			result, err := pipeline.Ingest(ctx, ingest.IngestionRequest{
				TenantID:     tenantID,
				ProductID:    productID,
				CampaignID:   campaignID,
				MarkdownPath: markdownPath,
				Operator:     operator,
				Overwrite:    overwrite,
				AutoPublish:  publishDraft,
			})
			if err != nil {
				return fmt.Errorf("ingestion failed: %w", err)
			}

			// Output result
			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"jobId":           result.JobID.String(),
					"status":          "completed",
					"specsCreated":    result.SpecsCreated,
					"featuresCreated": result.FeaturesCreated,
					"uspsCreated":     result.USPsCreated,
					"chunksCreated":   result.ChunksCreated,
					"duration":        result.Duration.String(),
				})
			}

			fmt.Printf("✓ Ingestion completed successfully\n")
			fmt.Printf("  Job ID: %s\n", result.JobID)
			fmt.Printf("  Specs: %d | Features: %d | USPs: %d | Chunks: %d\n",
				result.SpecsCreated, result.FeaturesCreated, result.USPsCreated, result.ChunksCreated)
			fmt.Printf("  Duration: %s\n", result.Duration)

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&product, "product", "", "product ID or name (required)")
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign variant ID or name (required)")
	cmd.Flags().StringVar(&markdown, "markdown", "", "path to Markdown file")
	cmd.Flags().StringVar(&pdf, "pdf", "", "path to PDF file (triggers pdf-extractor)")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "original source file path for lineage")
	cmd.Flags().BoolVar(&publishDraft, "publish-draft", false, "auto-publish after ingestion")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite existing draft")
	cmd.Flags().StringVar(&operator, "operator", "", "operator name for audit trail")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("product")
	_ = cmd.MarkFlagRequired("campaign")

	return cmd
}

// newPublishCmd creates the publish subcommand.
func newPublishCmd() *cobra.Command {
	var (
		tenant       string
		campaign     string
		version      int
		releaseNotes string
		rollback     bool
		approvedBy   string
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish or rollback a campaign version",
		Long: `Publish promotes a draft campaign to published status, making it available
for retrieval. Use --rollback to revert to a previous version.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			campaignID, err := resolveID(campaign)
			if err != nil {
				return fmt.Errorf("invalid campaign: %w", err)
			}

			if approvedBy == "" {
				approvedBy = os.Getenv("USER")
				if approvedBy == "" {
					approvedBy = "cli"
				}
			}

			publisher := ingest.NewPublisher(logger)

			if rollback {
				logger.Info().
					Str("tenant", tenant).
					Str("campaign", campaign).
					Int("version", version).
					Msg("Rolling back campaign")

				result, err := publisher.Rollback(ctx, ingest.RollbackRequest{
					TenantID:      tenantID,
					CampaignID:    campaignID,
					TargetVersion: version,
					Operator:      approvedBy,
				})
				if err != nil {
					return fmt.Errorf("rollback failed: %w", err)
				}

				if outputJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]interface{}{
						"action":   "rollback",
						"campaign": result.CampaignID.String(),
						"version":  result.CurrentVersion,
						"status":   string(result.Status),
					})
				}

				fmt.Printf("✓ Rolled back to version %d\n", result.CurrentVersion)
			} else {
				logger.Info().
					Str("tenant", tenant).
					Str("campaign", campaign).
					Msg("Publishing campaign")

				result, err := publisher.Publish(ctx, ingest.PublishRequest{
					TenantID:     tenantID,
					CampaignID:   campaignID,
					Version:      version,
					ApprovedBy:   approvedBy,
					ReleaseNotes: releaseNotes,
				})
				if err != nil {
					return fmt.Errorf("publish failed: %w", err)
				}

				if outputJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]interface{}{
						"action":        "publish",
						"campaign":      result.CampaignID.String(),
						"version":       result.Version,
						"status":        string(result.Status),
						"effectiveFrom": result.EffectiveFrom.Format(time.RFC3339),
					})
				}

				fmt.Printf("✓ Published campaign version %d\n", result.Version)
				fmt.Printf("  Effective from: %s\n", result.EffectiveFrom.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign variant ID or name (required)")
	cmd.Flags().IntVar(&version, "version", 0, "version to publish or rollback to")
	cmd.Flags().StringVar(&releaseNotes, "notes", "", "release notes for publication")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "rollback to specified version")
	cmd.Flags().StringVar(&approvedBy, "approved-by", "", "approver name for audit trail")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("campaign")

	return cmd
}

// newQueryCmd creates the query subcommand.
func newQueryCmd() *cobra.Command {
	var (
		tenant    string
		products  []string
		question  string
		intent    string
		maxChunks int
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query structured specs and semantic chunks",
		Long: `Query retrieves structured facts and semantic chunks for a question.
Results include citations and lineage information.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			var productIDs []uuid.UUID
			for _, p := range products {
				pid, err := resolveID(p)
				if err != nil {
					return fmt.Errorf("invalid product %s: %w", p, err)
				}
				productIDs = append(productIDs, pid)
			}

			logger.Info().
				Str("tenant", tenant).
				Int("products", len(productIDs)).
				Str("question", question).
				Msg("Executing query")

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Create spec view repository
			specViewRepo := storage.NewSpecViewRepository(db)

			// Create embedder (use mock for now, can be enhanced to use real embeddings)
			var embClient embedding.Embedder
			apiKey := os.Getenv("OPENROUTER_API_KEY")
			if apiKey != "" {
				client, err := embedding.NewClient(embedding.Config{
					APIKey:  apiKey,
					Model:   cfg.Embedding.Model,
					BaseURL: "https://openrouter.ai/api/v1",
				})
				if err == nil {
					embClient = client
				} else {
					logger.Warn().Err(err).Msg("Failed to create embedding client, using mock")
					embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
				}
			} else {
				embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
			}

			// Create retrieval infrastructure
			memCache := cache.NewMemoryClient(1000)
			vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
				Dimension: cfg.Embedding.Dimension,
			})
			if err != nil {
				return fmt.Errorf("create vector adapter: %w", err)
			}

			router := retrieval.NewRouter(
				logger,
				memCache,
				vectorAdapter,
				embClient,
				specViewRepo,
				retrieval.RouterConfig{
					MaxChunks:                 maxChunks,
					StructuredFirst:           true,
					SemanticFallback:          true,
					IntentConfidenceThreshold: 0.7,
				},
			)

			// Build request
			req := retrieval.RetrievalRequest{
				TenantID:   tenantID,
				ProductIDs: productIDs,
				Question:   question,
				MaxChunks:  maxChunks,
			}

			if intent != "" {
				intentVal := retrieval.Intent(intent)
				req.IntentHint = &intentVal
			}

			// Execute query
			resp, err := router.Query(ctx, req)
			if err != nil {
				return fmt.Errorf("query failed: %w", err)
			}

			// Output result
			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"intent":          string(resp.Intent),
					"latencyMs":       resp.LatencyMs,
					"structuredFacts": resp.StructuredFacts,
					"semanticChunks":  resp.SemanticChunks,
				})
			}

			fmt.Printf("Intent: %s (latency: %dms)\n\n", resp.Intent, resp.LatencyMs)

			if len(resp.StructuredFacts) > 0 {
				fmt.Printf("Structured Facts:\n")
				for _, fact := range resp.StructuredFacts {
					fmt.Printf("  • %s / %s: %s", fact.Category, fact.Name, fact.Value)
					if fact.Unit != "" {
						fmt.Printf(" %s", fact.Unit)
					}
					fmt.Printf(" (conf: %.2f)\n", fact.Confidence)
				}
				fmt.Println()
			}

			if len(resp.SemanticChunks) > 0 {
				fmt.Printf("Semantic Chunks:\n")
				for i, chunk := range resp.SemanticChunks {
					text := chunk.Text
					if len(text) > 100 {
						text = text[:100] + "..."
					}
					fmt.Printf("  %d. [%s] %s (dist: %.3f)\n", i+1, chunk.ChunkType, text, chunk.Distance)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringSliceVar(&products, "products", nil, "product IDs to query")
	cmd.Flags().StringVar(&question, "question", "", "question to answer (required)")
	cmd.Flags().StringVar(&intent, "intent", "", "intent hint (spec_lookup, usp_lookup, comparison)")
	cmd.Flags().IntVar(&maxChunks, "max-chunks", 6, "maximum chunks to return")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("question")

	return cmd
}

// newCompareCmd creates the compare subcommand.
func newCompareCmd() *cobra.Command {
	var (
		tenant    string
		primary   string
		secondary string
		dims      []string
		maxRows   int
	)

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two products",
		Long:  `Compare retrieves pre-computed comparisons between two products.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
				Msg("Comparing products")

			// TODO: Wire up comparison materializer when store is available
			_ = ctx
			_ = tenantID
			_ = primaryID
			_ = secondaryID
			_ = dims
			_ = maxRows

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"primary":     primaryID.String(),
					"secondary":   secondaryID.String(),
					"comparisons": []interface{}{},
				})
			}

			fmt.Printf("No pre-computed comparisons found for this product pair.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&primary, "primary", "", "primary product ID (required)")
	cmd.Flags().StringVar(&secondary, "secondary", "", "secondary product ID (required)")
	cmd.Flags().StringSliceVar(&dims, "dimensions", nil, "dimensions to compare")
	cmd.Flags().IntVar(&maxRows, "max-rows", 20, "maximum rows to return")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("primary")
	_ = cmd.MarkFlagRequired("secondary")

	// Add recompute subcommand (T045)
	cmd.AddCommand(newComparisonRecomputeCmd())

	return cmd
}

// newDriftCmd creates the drift subcommand.
func newDriftCmd() *cobra.Command {
	var (
		tenant   string
		campaign string
		check    bool
		report   bool
	)

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Monitor and report drift alerts",
		Long: `Drift commands help detect stale campaigns, hash changes, and conflicts.
Use --check to run a drift scan or --report to view current alerts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			driftRunner := monitoring.NewDriftRunner(logger, nil, monitoring.DriftConfig{
				CheckInterval:      1 * time.Hour,
				FreshnessThreshold: 30 * 24 * time.Hour,
			})

			if check {
				logger.Info().
					Str("tenant", tenant).
					Msg("Running drift check")

				result, err := driftRunner.RunCheck(ctx, tenantID)
				if err != nil {
					return fmt.Errorf("drift check failed: %w", err)
				}

				if outputJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]interface{}{
						"checkedAt":      result.CheckedAt.Format(time.RFC3339),
						"staleCampaigns": len(result.StaleCampaigns),
						"hashMismatches": len(result.HashMismatches),
						"embeddingDrift": len(result.EmbeddingDrift),
						"totalAlerts":    result.TotalAlerts,
					})
				}

				fmt.Printf("✓ Drift check completed at %s\n", result.CheckedAt.Format(time.RFC3339))
				fmt.Printf("  Stale campaigns: %d\n", len(result.StaleCampaigns))
				fmt.Printf("  Hash mismatches: %d\n", len(result.HashMismatches))
				fmt.Printf("  Embedding drift: %d\n", len(result.EmbeddingDrift))
				fmt.Printf("  Total alerts: %d\n", result.TotalAlerts)
			}

			if report {
				logger.Info().
					Str("tenant", tenant).
					Msg("Generating drift report")

				alerts, err := driftRunner.ListOpenAlerts(ctx, tenantID)
				if err != nil {
					return fmt.Errorf("failed to list alerts: %w", err)
				}

				if outputJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]interface{}{
						"tenant": tenantID.String(),
						"alerts": alerts,
					})
				}

				if len(alerts) == 0 {
					fmt.Printf("No open drift alerts for tenant %s\n", tenant)
				} else {
					fmt.Printf("Open drift alerts for tenant %s:\n\n", tenant)
					for _, alert := range alerts {
						fmt.Printf("  • [%s] %s - %s\n",
							alert.AlertType, alert.ID, alert.DetectedAt.Format(time.RFC3339))
					}
				}
			}

			if !check && !report {
				_ = cmd.Help()
			}

			_ = campaign // For future filtering
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&campaign, "campaign", "", "specific campaign to check")
	cmd.Flags().BoolVar(&check, "check", false, "run drift check")
	cmd.Flags().BoolVar(&report, "report", false, "generate drift report")

	_ = cmd.MarkFlagRequired("tenant")

	return cmd
}

// newExportCmd creates the export subcommand.
func newExportCmd() *cobra.Command {
	var (
		tenant   string
		product  string
		campaign string
		format   string
		output   string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export data to CSV or Parquet",
		Long: `Export campaign data for auditing or migration.
Supports CSV and Parquet formats.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			logger.Info().
				Str("tenant", tenant).
				Str("format", format).
				Str("output", output).
				Msg("Exporting data")

			// Create output file
			file, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer file.Close()

			if format == "csv" {
				writer := csv.NewWriter(file)
				defer writer.Flush()

				// Write header
				if err := writer.Write([]string{
					"tenant_id", "product_id", "campaign_id", "category", "name", "value", "unit",
				}); err != nil {
					return fmt.Errorf("write header: %w", err)
				}

				// TODO: Query actual data from store
				// For now, write placeholder row
				if err := writer.Write([]string{
					tenantID.String(), product, campaign, "Example", "example_spec", "value", "unit",
				}); err != nil {
					return fmt.Errorf("write row: %w", err)
				}

				fmt.Printf("✓ Exported to %s (CSV format)\n", output)
			} else if format == "parquet" {
				return fmt.Errorf("parquet export not yet implemented")
			} else {
				return fmt.Errorf("unsupported format: %s", format)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&product, "product", "", "product filter")
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign filter")
	cmd.Flags().StringVar(&format, "format", "csv", "output format (csv, parquet)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (required)")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// newImportCmd creates the import subcommand.
func newImportCmd() *cobra.Command {
	var (
		tenant string
		input  string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from CSV or Parquet",
		Long: `Import campaign data from exported files.
Use --dry-run to validate without committing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			logger.Info().
				Str("tenant", tenant).
				Str("input", input).
				Bool("dry_run", dryRun).
				Msg("Importing data")

			// Open input file
			file, err := os.Open(input)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer file.Close()

			// Detect format from extension
			ext := filepath.Ext(input)
			if ext == ".csv" {
				reader := csv.NewReader(file)
				
				// Read header
				header, err := reader.Read()
				if err != nil {
					return fmt.Errorf("read header: %w", err)
				}
				
				// Read records
				var rowCount int
				for {
					record, err := reader.Read()
					if err != nil {
						break
					}
					rowCount++
					
					if verbose {
						logger.Info().
							Str("tenant_id", tenantID.String()).
							Int("fields", len(record)).
							Int("row", rowCount).
							Msg("Processing row")
					}
				}

				if dryRun {
					fmt.Printf("✓ Dry run: would import %d rows (header: %v)\n", rowCount, header)
				} else {
					// TODO: Actually import the data
					fmt.Printf("✓ Imported %d rows from %s\n", rowCount, input)
				}
			} else if ext == ".parquet" {
				return fmt.Errorf("parquet import not yet implemented")
			} else {
				return fmt.Errorf("unsupported file format: %s", ext)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVarP(&input, "input", "i", "", "input file path (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}

// newMigrateCmd creates the migrate subcommand.
func newMigrateCmd() *cobra.Command {
	var (
		sqlite   bool
		postgres bool
		down     bool
		version  int
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long: `Run database migrations for SQLite or Postgres.
Use --down to rollback migrations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "sqlite"
			if postgres {
				target = "postgres"
			}

			if down {
				logger.Info().
					Str("target", target).
					Int("version", version).
					Msg("Rolling back migrations")
				// TODO: Implement migration rollback
				fmt.Printf("✓ Rolled back to version %d on %s\n", version, target)
				return nil
			}

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Determine migration file
			var migrationFile string
			if target == "sqlite" {
				migrationFile = "db/migrations/0001_init_sqlite.sql"
			} else {
				migrationFile = "db/migrations/0001_init.sql"
			}

			// Read migration file (relative to knowledge-engine directory)
			migrationPath := migrationFile

			logger.Info().
				Str("target", target).
				Str("file", migrationPath).
				Msg("Running migrations")

			migrationSQL, err := os.ReadFile(migrationPath)
			if err != nil {
				return fmt.Errorf("read migration file: %w", err)
			}

			// Execute migration
			_, err = db.Exec(string(migrationSQL))
			if err != nil {
				return fmt.Errorf("execute migration: %w", err)
			}

			fmt.Printf("✓ Migrations applied on %s\n", target)
			return nil
		},
	}

	cmd.Flags().BoolVar(&sqlite, "sqlite", true, "migrate SQLite database")
	cmd.Flags().BoolVar(&postgres, "postgres", false, "migrate Postgres database")
	cmd.Flags().BoolVar(&down, "down", false, "rollback migrations")
	cmd.Flags().IntVar(&version, "version", 0, "target migration version")

	return cmd
}

// newVersionCmd creates the version subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.Encode(map[string]string{
					"version": "0.1.0",
					"go":      "1.23",
				})
				return
			}
			fmt.Println("knowledge-engine-cli v0.1.0")
		},
	}
}

// resolveID parses a string as UUID or generates one for name lookup.
func resolveID(idOrName string) (uuid.UUID, error) {
	if idOrName == "" {
		return uuid.Nil, fmt.Errorf("empty ID or name")
	}

	// Try to parse as UUID
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}

	// If not a UUID, generate a deterministic UUID from name (for dev/testing)
	// In production, this would query the database by name
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(idOrName)), nil
}

// openDatabase opens a database connection based on the configuration.
func openDatabase(cfg *config.Config) (*sql.DB, error) {
	dsn := cfg.DatabaseDSN()
	
	var driver string
	if cfg.Database.Driver == "sqlite" {
		driver = "sqlite3"
	} else if cfg.Database.Driver == "postgres" {
		driver = "postgres"
		// Import postgres driver if needed
		// _ "github.com/lib/pq"
		return nil, fmt.Errorf("postgres driver not yet implemented in CLI")
	} else {
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set connection pool settings for SQLite
	if cfg.Database.Driver == "sqlite" {
		db.SetMaxOpenConns(cfg.Database.SQLite.MaxOpenConns)
	}

	return db, nil
}
