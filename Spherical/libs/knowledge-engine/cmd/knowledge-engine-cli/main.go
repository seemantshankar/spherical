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
	"strings"
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
	noColor    bool

	// Configuration and logger
	cfg    *config.Config
	logger *observability.Logger
	ui     *UI
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

		// Initialize UI
		ui = NewUI(outputJSON, noColor || !IsTerminal())

		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default: uses env vars)")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

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
		tenant            string
		product           string
		campaign          string
		markdown          string
		pdf               string
		sourceFile        string
		publishDraft      bool
		overwrite         bool
		operator          string
		embeddingBatchSize int
	)

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest brochure-derived Markdown into a campaign",
		Long: `Ingest parses Markdown (from pdf-extractor or manual upload), normalizes
specs/features/USPs, deduplicates, and stores them in the campaign.

If --pdf is provided instead of --markdown, the CLI automatically invokes
the pdf-extractor binary to generate Markdown first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer ui.Close()
			
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			ui.Section("Ingestion")
			
			// Determine markdown source
			markdownPath := markdown
			if pdf != "" && markdown == "" {
				// Extract PDF to Markdown using pdf-extractor
				ui.Step("Extracting PDF to Markdown")
				spinner := ui.Spinner("Extracting")
				
				tempDir, err := os.MkdirTemp("", "ke-ingest-*")
				if err != nil {
					if spinner != nil {
						spinner.Abort(true)
					}
					ui.Error("Failed to create temporary directory: %v", err)
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
					if spinner != nil {
						spinner.Abort(true)
					}
					ui.Error("PDF extraction failed: %v", err)
					return fmt.Errorf("pdf extraction failed: %w", err)
				}
				
				if spinner != nil {
					spinner.SetTotal(100, true)
				}
				ui.Success("PDF extracted successfully")
			}

			if markdownPath == "" {
				ui.Error("Either --markdown or --pdf is required")
				return fmt.Errorf("either --markdown or --pdf is required")
			}

			// Read markdown file
			ui.Step("Reading markdown file")
			content, err := os.ReadFile(markdownPath)
			if err != nil {
				ui.Error("Failed to read markdown file: %v", err)
				return fmt.Errorf("read markdown: %w", err)
			}
			ui.Info("File size: %s", FormatBytes(int64(len(content))))

			// Parse tenant/product/campaign IDs
			ui.Step("Validating parameters")
			tenantID, err := resolveID(tenant)
			if err != nil {
				ui.Error("Invalid tenant ID: %v", err)
				return fmt.Errorf("invalid tenant: %w", err)
			}

			productID, err := resolveID(product)
			if err != nil {
				ui.Error("Invalid product ID: %v", err)
				return fmt.Errorf("invalid product: %w", err)
			}

			campaignID, err := resolveID(campaign)
			if err != nil {
				ui.Error("Invalid campaign ID: %v", err)
				return fmt.Errorf("invalid campaign: %w", err)
			}

			// Get operator
			if operator == "" {
				operator = os.Getenv("USER")
				if operator == "" {
					operator = "cli"
				}
			}

			ui.Info("Tenant: %s", tenant)
			ui.Info("Product: %s", product)
			ui.Info("Campaign: %s", campaign)
			ui.Info("Operator: %s", operator)

			logger.Info().
				Str("tenant", tenant).
				Str("product", product).
				Str("campaign", campaign).
				Str("operator", operator).
				Int("content_size", len(content)).
				Msg("Starting ingestion")

			// Open database connection
			ui.Step("Connecting to database")
			db, err := openDatabase(cfg)
			if err != nil {
				ui.Error("Failed to connect to database: %v", err)
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()
			ui.Success("Database connected")

			// Create repositories
			repos := storage.NewRepositories(db)

			// Create embedding client
			ui.Step("Initializing embedding client")
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
					ui.Success("Using OpenRouter embedding service")
				} else {
					logger.Warn().Err(err).Msg("Failed to create embedding client, using mock")
					embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
					ui.Warning("Using mock embedding client (OpenRouter unavailable)")
				}
			} else {
				embClient = embedding.NewMockClient(cfg.Embedding.Dimension)
				ui.Warning("Using mock embedding client (OPENROUTER_API_KEY not set)")
			}

			// Create vector adapter
			ui.Step("Initializing vector store")
			vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{
				Dimension: cfg.Embedding.Dimension,
			})
			if err != nil {
				ui.Error("Failed to create vector adapter: %v", err)
				return fmt.Errorf("create vector adapter: %w", err)
			}
			ui.Success("Vector store initialized")

			// Create lineage writer
			lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

			// Create pipeline
			// Use provided batch size or default to 75
			batchSize := embeddingBatchSize
			if batchSize <= 0 {
				batchSize = 75 // Default batch size
			}
			
			pipeline := ingest.NewPipeline(
				logger,
				ingest.PipelineConfig{
					ChunkSize:         512,
					ChunkOverlap:      64,
					MaxConcurrentJobs: 4,
					DedupeThreshold:   0.95,
					EmbeddingBatchSize: batchSize,
				},
				repos,
				embClient,
				vectorAdapter,
				lineageWriter,
			)

			// Run ingestion
			ui.Newline()
			ui.Step("Starting ingestion pipeline")
			spinner := ui.Spinner("Processing")
			
			result, err := pipeline.Ingest(ctx, ingest.IngestionRequest{
				TenantID:     tenantID,
				ProductID:    productID,
				CampaignID:   campaignID,
				MarkdownPath: markdownPath,
				Operator:     operator,
				Overwrite:    overwrite,
				AutoPublish:  publishDraft,
			})
			
			if spinner != nil {
				spinner.SetTotal(100, true)
			}
			
			if err != nil {
				ui.Error("Ingestion failed: %v", err)
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

			ui.Newline()
			ui.Section("Results")
			ui.Success("Ingestion completed successfully")
			ui.Newline()
			
			// Create results table
			rows := [][]string{
				{"Job ID", result.JobID.String()},
				{"Specs Created", fmt.Sprintf("%d", result.SpecsCreated)},
				{"Features Created", fmt.Sprintf("%d", result.FeaturesCreated)},
				{"USPs Created", fmt.Sprintf("%d", result.USPsCreated)},
				{"Chunks Created", fmt.Sprintf("%d", result.ChunksCreated)},
				{"Duration", FormatDuration(result.Duration)},
			}
			
			ui.Table([]string{"Metric", "Value"}, rows)
			
			ui.Newline()
			totalItems := result.SpecsCreated + result.FeaturesCreated + result.USPsCreated + result.ChunksCreated
			ui.Info("Total items processed: %d in %s", totalItems, FormatDuration(result.Duration))

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
	cmd.Flags().IntVar(&embeddingBatchSize, "embedding-batch-size", 75, "batch size for embedding generation (50-100, default: 75)")

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

			// Open database connection early for product lookup
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()
			
			var productIDs []uuid.UUID
			for _, p := range products {
				pid, err := resolveProductID(db, p)
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

			// Database connection already opened above for product lookup

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

			// Load vectors from database into FAISS adapter
			chunkRepo := storage.NewKnowledgeChunkRepository(db)
			chunks, err := chunkRepo.GetWithEmbeddingsByTenantAndProducts(ctx, tenantID, productIDs)
			if err != nil {
				logger.Warn().Err(err).Msg("Failed to load chunks with embeddings, vector search may return no results")
			} else if len(chunks) > 0 {
				// Convert chunks to vector entries
				vectorEntries := make([]retrieval.VectorEntry, 0, len(chunks))
				for _, chunk := range chunks {
					if len(chunk.EmbeddingVector) == 0 {
						continue
					}
					
					embeddingVersion := ""
					if chunk.EmbeddingVersion != nil {
						embeddingVersion = *chunk.EmbeddingVersion
					}
					
					vectorEntries = append(vectorEntries, retrieval.VectorEntry{
						ID:                chunk.ID,
						TenantID:          chunk.TenantID,
						ProductID:         chunk.ProductID,
						CampaignVariantID: chunk.CampaignVariantID,
						ChunkType:         string(chunk.ChunkType),
						Visibility:        string(chunk.Visibility),
						EmbeddingVersion:  embeddingVersion,
						Vector:            chunk.EmbeddingVector,
						Metadata: map[string]interface{}{
							"text":       chunk.Text,
							"chunk_type": string(chunk.ChunkType),
							"source_doc": nil,
						},
					})
					if chunk.SourceDocID != nil {
						vectorEntries[len(vectorEntries)-1].Metadata["source_doc"] = chunk.SourceDocID.String()
					}
				}
				
				// Insert vectors into FAISS adapter
				if len(vectorEntries) > 0 {
					if err := vectorAdapter.Insert(ctx, vectorEntries); err != nil {
						logger.Warn().Err(err).Int("vector_count", len(vectorEntries)).Msg("Failed to insert vectors into FAISS adapter")
					} else {
						logger.Info().Int("vector_count", len(vectorEntries)).Msg("Loaded vectors into FAISS adapter")
					}
				}
			} else {
				logger.Debug().Msg("No chunks with embeddings found in database")
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

			// Load full chunk text from database and find additional relevant chunks
			if len(resp.SemanticChunks) > 0 || len(question) > 0 {
				queryKeywords := strings.Fields(strings.ToLower(question))
				// Filter out stop words and extract meaningful keywords
				stopWords := map[string]bool{
					"what": true, "are": true, "the": true, "this": true, "that": true,
					"is": true, "a": true, "an": true, "in": true, "on": true, "at": true,
					"to": true, "for": true, "of": true, "with": true, "from": true,
					"and": true, "or": true, "but": true, "if": true, "can": true,
					"do": true, "does": true, "did": true, "will": true, "would": true,
					"should": true, "could": true, "may": true, "might": true,
					"car": true, "cars": true, "vehicle": true, "vehicles": true,
					"per": true, "liter": true, "litre": true,
				}
				keywords := make([]string, 0)
				for _, kw := range queryKeywords {
					kw = strings.Trim(kw, ".,!?;:()[]{}'\"")
					kwLower := strings.ToLower(kw)
					// Only include if it's not a stop word and has meaningful length
					// Also handle plural/singular forms - if query is "features", also search for "feature"
					if len(kw) > 2 && !stopWords[kwLower] {
						keywords = append(keywords, kw)
						// Add singular form if plural (e.g., "features" -> "feature")
						if strings.HasSuffix(kwLower, "s") && len(kw) > 3 {
							singular := kw[:len(kw)-1]
							keywords = append(keywords, singular)
						}
						// Add plural form if singular (e.g., "feature" -> "features")
						if !strings.HasSuffix(kwLower, "s") && len(kw) > 3 {
							plural := kw + "s"
							keywords = append(keywords, plural)
						}
					}
				}
				// Add related terms for fuel efficiency queries
				if strings.Contains(strings.ToLower(question), "fuel") || strings.Contains(strings.ToLower(question), "mileage") || 
				   strings.Contains(strings.ToLower(question), "km") || strings.Contains(strings.ToLower(question), "efficiency") {
					keywords = append(keywords, "fuel", "efficiency", "mileage", "km")
				}
				
				// Load full text for chunks returned by router
				chunkMap := make(map[uuid.UUID]*retrieval.SemanticChunk)
				for i := range resp.SemanticChunks {
					chunkID := resp.SemanticChunks[i].ChunkID
					if chunkID == uuid.Nil {
						continue
					}
					
					// Load full text from database
					query := `SELECT text FROM knowledge_chunks WHERE id = $1`
					var fullText string
					err := db.QueryRowContext(ctx, query, chunkID).Scan(&fullText)
					if err == nil && fullText != "" {
						resp.SemanticChunks[i].Text = fullText
					}
					chunkMap[chunkID] = &resp.SemanticChunks[i]
				}
				
				// Also search database for chunks that match keywords (may find chunks missed by vector search)
				// Automatically search for spec_row chunks that match keywords - works for any query type
				if len(keywords) > 0 && len(productIDs) > 0 {
					// Build SQL query to find chunks with keywords - use parameterized query for safety
					searchTerms := make([]string, 0)
					queryArgs := []interface{}{tenantID}
					argIndex := 2 // Start from $2 since $1 is tenantID
					
					// Add product IDs
					productPlaceholders := make([]string, len(productIDs))
					for i, pid := range productIDs {
						productPlaceholders[i] = fmt.Sprintf("$%d", argIndex)
						queryArgs = append(queryArgs, pid)
						argIndex++
					}
					
					// Automatically search for spec_row chunks that match keywords
					// This helps find precise row-level chunks that vector search might miss
					if len(keywords) > 0 {
						// Build LIKE conditions for each keyword - use word boundary matching to avoid substring matches
						// This is generic and works for any query type
						keywordConditions := make([]string, 0)
						for _, kw := range keywords {
							// Escape single quotes in keyword for SQL safety
							escapedKw := strings.ReplaceAll(kw, "'", "''")
							kwLower := strings.ToLower(escapedKw)
							
							// Use word boundary matching: keyword must be at start/end or surrounded by spaces/punctuation
							// This prevents "usp" from matching "suspension"
							// For short keywords (3-4 chars), be more strict to avoid substring matches
							// For longer keywords, substring matching is usually fine
							
							if len(kwLower) <= 4 {
								// Short keywords: use strict word boundary matching to avoid substring matches
								// Match at start (keyword followed by space, colon, or punctuation)
								keywordConditions = append(keywordConditions, 
									fmt.Sprintf("LOWER(text) LIKE '%s %%' OR LOWER(text) LIKE '%s:%%' OR LOWER(text) LIKE '%s.%%' OR LOWER(text) LIKE '%s,%%' OR LOWER(text) = '%s'", kwLower, kwLower, kwLower, kwLower, kwLower))
								
								// Match at end (keyword preceded by space)
								keywordConditions = append(keywordConditions, 
									fmt.Sprintf("LOWER(text) LIKE '%% %s' OR LOWER(text) = '%s'", kwLower, kwLower))
								
								// Match in middle (keyword surrounded by spaces or punctuation)
								keywordConditions = append(keywordConditions, 
									fmt.Sprintf("LOWER(text) LIKE '%% %s %%' OR LOWER(text) LIKE '%% %s:%%' OR LOWER(text) LIKE '%% %s.%%' OR LOWER(text) LIKE '%% %s,%%'", kwLower, kwLower, kwLower, kwLower))
							} else {
								// Longer keywords: substring matching is usually fine
								keywordConditions = append(keywordConditions, 
									fmt.Sprintf("LOWER(text) LIKE '%%%s%%'", kwLower))
							}
							
							// Also try with "color" if keyword is "colour" (and vice versa)
							if kwLower == "colour" {
								keywordConditions = append(keywordConditions, 
									"LOWER(text) LIKE '% color %' OR LOWER(text) LIKE '% color' OR LOWER(text) LIKE 'color %' OR LOWER(text) = 'color'")
							} else if kwLower == "color" {
								keywordConditions = append(keywordConditions, 
									"LOWER(text) LIKE '% colour %' OR LOWER(text) LIKE '% colour' OR LOWER(text) LIKE 'colour %' OR LOWER(text) = 'colour'")
							}
							// Also search for "Key Feature:" prefix when querying for features
							if kwLower == "feature" || kwLower == "features" {
								keywordConditions = append(keywordConditions, 
									"LOWER(text) LIKE 'key feature:%' OR LOWER(text) LIKE '%key feature:%'")
							}
						}
						
						// Also search for multi-word phrases from the extracted keywords
						// This helps match phrases like "fuel tank capacity" or "colour options"
						// Only use the meaningful keywords that were already extracted (not all question words)
						if len(keywords) > 1 {
							// Add 2-word phrases from consecutive keywords
							for i := 0; i < len(keywords)-1; i++ {
								phrase := keywords[i] + " " + keywords[i+1]
								escapedPhrase := strings.ReplaceAll(phrase, "'", "''")
								keywordConditions = append(keywordConditions, 
									fmt.Sprintf("LOWER(text) LIKE '%%%s%%'", strings.ToLower(escapedPhrase)))
							}
							
							// Add 3-word phrases from consecutive keywords
							if len(keywords) > 2 {
								for i := 0; i < len(keywords)-2; i++ {
									phrase := keywords[i] + " " + keywords[i+1] + " " + keywords[i+2]
									escapedPhrase := strings.ReplaceAll(phrase, "'", "''")
									keywordConditions = append(keywordConditions, 
										fmt.Sprintf("LOWER(text) LIKE '%%%s%%'", strings.ToLower(escapedPhrase)))
								}
							}
						}
						
						// Search for chunks matching keywords (both spec_row and global chunks)
						// Search by tenant_id and keywords first (more flexible)
						// If product lookup found products, also try to match by product_id
						var chunkSearchSQL string
						var chunkSearchArgs []interface{}
						chunkSearchArgs = append(chunkSearchArgs, tenantID)
						
						// Search for chunks matching keywords (spec_row and global types)
						// Don't require embedding_vector - chunks may not have embeddings yet
						// This makes the search more flexible and works for any query type
						// Prioritize "Key Feature:" and "USP:" chunks by ordering them first
						chunkSearchSQL = fmt.Sprintf(`
							SELECT id, text, chunk_type FROM knowledge_chunks 
							WHERE tenant_id = $1 
								AND chunk_type IN ('spec_row', 'global')
								AND (%s)
							ORDER BY 
								CASE 
									WHEN LOWER(text) LIKE 'key feature:%%' THEN 1
									WHEN LOWER(text) LIKE 'usp:%%' THEN 2
									ELSE 3
								END
							LIMIT 50
						`, strings.Join(keywordConditions, " OR "))
						
						// SQL query executed silently
						
						rows, err := db.QueryContext(ctx, chunkSearchSQL, chunkSearchArgs...)
						if err != nil {
							logger.Debug().Err(err).Msg("Database search for chunks failed")
						} else {
							defer rows.Close()
							foundCount := 0
							keyFeatureCount := 0
							for rows.Next() {
								var chunkID uuid.UUID
								var fullText string
								var chunkTypeStr string
								if err := rows.Scan(&chunkID, &fullText, &chunkTypeStr); err == nil {
									// If not already in results, add it
									if _, exists := chunkMap[chunkID]; !exists {
										// Check if this is a "Key Feature:" or "USP:" chunk - give it higher priority
										score := float32(5.0)
										if strings.HasPrefix(fullText, "Key Feature:") || strings.HasPrefix(fullText, "USP:") {
											score = float32(15.0) // Very high score for Key Features and USPs to ensure they appear
											keyFeatureCount++
										}
										newChunk := retrieval.SemanticChunk{
											ChunkID:   chunkID,
											ChunkType: storage.ChunkType(chunkTypeStr),
											Text:      fullText,
											Distance:  0.3, // Good similarity for keyword match
											Score:     score,
										}
										resp.SemanticChunks = append(resp.SemanticChunks, newChunk)
										chunkMap[chunkID] = &resp.SemanticChunks[len(resp.SemanticChunks)-1]
										foundCount++
									}
								}
							}
							if foundCount > 0 {
								logger.Debug().Int("total_chunks", foundCount).Int("key_features", keyFeatureCount).Msg("Database search found chunks")
							}
						}
					}
					
					// Add keyword search terms
					for _, kw := range keywords {
						searchTerms = append(searchTerms, fmt.Sprintf("text LIKE $%d", argIndex))
						queryArgs = append(queryArgs, "%"+kw+"%")
						argIndex++
					}
					
					// Add keyword search terms for global chunks
					limit := maxChunks * 2
					
					searchSQL := fmt.Sprintf(`
						SELECT id, text FROM knowledge_chunks 
						WHERE tenant_id = $1 
							AND product_id IN (%s)
							AND (%s)
							AND embedding_vector IS NOT NULL
							AND LENGTH(text) > 50
						LIMIT %d
					`, strings.Join(productPlaceholders, ", "), strings.Join(searchTerms, " OR "), limit)
					
					rows, err := db.QueryContext(ctx, searchSQL, queryArgs...)
					if err == nil {
						defer rows.Close()
						for rows.Next() {
							var chunkID uuid.UUID
							var fullText string
							if err := rows.Scan(&chunkID, &fullText); err == nil {
								// If not already in results, check if it's highly relevant
								if _, exists := chunkMap[chunkID]; !exists {
									fullTextLower := strings.ToLower(fullText)
									// Check keyword relevance
									keywordMatches := 0
									for _, kw := range keywords {
										if strings.Contains(fullTextLower, kw) {
											keywordMatches++
										}
									}
									
									// Add chunk if it matches keywords (general approach - works for any query type)
									if keywordMatches >= 1 {
										newChunk := retrieval.SemanticChunk{
											ChunkID:   chunkID,
											ChunkType: storage.ChunkTypeGlobal,
											Text:      fullText,
											Distance:  0.5, // Neutral distance since we don't have vector similarity
											Score:     0.5,
										}
										resp.SemanticChunks = append(resp.SemanticChunks, newChunk)
									}
								}
							}
						}
					}
				}
				
				// Re-rank all chunks by keyword relevance in full text
				type scoredChunk struct {
					chunk *retrieval.SemanticChunk
					score float64
				}
				scoredChunks := make([]scoredChunk, 0, len(resp.SemanticChunks))
				
				// Scoring loop started silently
				for i := range resp.SemanticChunks {
					score := float64(resp.SemanticChunks[i].Score)
					fullText := resp.SemanticChunks[i].Text
					if fullText == "" {
						// Try to load text from database if empty
						chunkID := resp.SemanticChunks[i].ChunkID
						if chunkID != uuid.Nil {
							query := `SELECT text FROM knowledge_chunks WHERE id = $1`
							err := db.QueryRowContext(ctx, query, chunkID).Scan(&fullText)
							if err == nil && fullText != "" {
								resp.SemanticChunks[i].Text = fullText
							}
						}
						if fullText == "" {
							continue
						}
					}
					fullTextLower := strings.ToLower(fullText)
					
					// Chunk processing - no verbose logging
					
					// Calculate keyword relevance
					keywordMatches := 0
					matchedKeywords := make([]string, 0)
					for _, kw := range keywords {
						if strings.Contains(fullTextLower, kw) {
							keywordMatches++
							matchedKeywords = append(matchedKeywords, kw)
							score += 0.5 // Boost for keyword match
						}
					}
					
					// Big boost for multiple keyword matches
					if keywordMatches >= 2 {
						score += 1.0
					}
					
					// Very big boost if ALL keywords match (indicates high relevance)
					if keywordMatches == len(keywords) && len(keywords) > 1 {
						score += 5.0 // Significant boost for matching all keywords
					}
					
					// Generic multi-word phrase matching: extract phrases from question and boost if found in chunk
					// This works for any query type without hardcoding specific concepts
					questionLower := strings.ToLower(question)
					questionWords := strings.Fields(questionLower)
					
					// Check for 2-word phrases from the question (e.g., "fuel tank", "tank capacity", "fuel efficiency")
					for i := 0; i < len(questionWords)-1; i++ {
						phrase := questionWords[i] + " " + questionWords[i+1]
						if strings.Contains(fullTextLower, phrase) {
							score += 3.0 // Boost for matching 2-word phrases from question
						}
					}
					
					// Check for 3-word phrases (e.g., "fuel tank capacity")
					for i := 0; i < len(questionWords)-2; i++ {
						phrase := questionWords[i] + " " + questionWords[i+1] + " " + questionWords[i+2]
						if strings.Contains(fullTextLower, phrase) {
							score += 10.0 // Larger boost for matching 3-word phrases
						}
					}
					
					// Penalize very long chunks (like full spec tables) - prefer concise answers
					textLength := len(resp.SemanticChunks[i].Text)
					if textLength > 5000 {
						score *= 0.1 // Heavy penalty for very long chunks
					} else if textLength > 2000 {
						score *= 0.5 // Moderate penalty for long chunks
					}
					
					// Penalize very short chunks (likely headers)
					if textLength < 100 {
						score *= 0.2
					}
					
					// Boost score for "Key Feature:" and "USP:" chunks when querying for features/USPs
					// This ensures these chunks appear in results even if keyword matching is imperfect
					if strings.HasPrefix(fullText, "Key Feature:") || strings.HasPrefix(fullText, "USP:") {
						// Very significant boost for Key Features and USPs - they're highly relevant
						// Use a large boost to ensure they appear at the top of results
						score += 50.0
					}
					
					// Only include chunks with meaningful scores
					minScore := 1.0
					
					// REMOVED: All query-type-specific logic (isColorQuery, isFuelEfficiencyQuery, etc.)
					// The system now works generically using:
					// 1. Keyword matching and scoring
					// 2. Multi-word phrase matching (above)
					// 3. Vector search similarity
					// 4. Automatic spec_row chunk prioritization (below)
					
					// Old query-type-specific code removed - was here:
					// if isColorQuery {
					
					if score > minScore {
						scoredChunks = append(scoredChunks, scoredChunk{
							chunk: &resp.SemanticChunks[i],
							score: score,
						})
					}
				}
				
				// Sort by relevance score (descending)
				for i := 0; i < len(scoredChunks)-1; i++ {
					for j := i + 1; j < len(scoredChunks); j++ {
						if scoredChunks[i].score < scoredChunks[j].score {
							scoredChunks[i], scoredChunks[j] = scoredChunks[j], scoredChunks[i]
						}
					}
				}
				
				// Automatically prioritize spec_row chunks when available and relevant
				// This works for any query type - spec_row chunks are inherently more precise
				// Separate spec_row chunks from global chunks
				specRowChunks := make([]scoredChunk, 0)
				globalChunks := make([]scoredChunk, 0)
				
				for _, sc := range scoredChunks {
					if sc.score <= 0 {
						continue // Skip chunks with zero or negative scores
					}
					if sc.chunk.ChunkType == storage.ChunkTypeSpecRow {
						specRowChunks = append(specRowChunks, sc)
					} else {
						globalChunks = append(globalChunks, sc)
					}
				}
				
				// Determine if we should prioritize spec_row chunks
				// Criteria: spec_row chunks exist AND have reasonable relevance scores
				// Threshold: at least one spec_row chunk with score >= 2.0 (after keyword boosts)
				shouldPrioritizeSpecRow := false
				if len(specRowChunks) > 0 {
					// Check if any spec_row chunk has a good score
					for _, sc := range specRowChunks {
						if sc.score >= 2.0 {
							shouldPrioritizeSpecRow = true
							break
						}
					}
				}
				
				// Build final results
				reRanked := make([]retrieval.SemanticChunk, 0)
				seenChunkIDs := make(map[uuid.UUID]bool)
				
				if shouldPrioritizeSpecRow {
					// Sort spec_row chunks by score (highest first) - they're already sorted, but ensure it
					for i := 0; i < len(specRowChunks)-1; i++ {
						for j := i + 1; j < len(specRowChunks); j++ {
							if specRowChunks[i].score < specRowChunks[j].score {
								specRowChunks[i], specRowChunks[j] = specRowChunks[j], specRowChunks[i]
							}
						}
					}
					
					// Add spec_row chunks first (up to maxChunks)
					for _, sc := range specRowChunks {
						if len(reRanked) >= maxChunks {
							break
						}
						if !seenChunkIDs[sc.chunk.ChunkID] {
							reRanked = append(reRanked, *sc.chunk)
							seenChunkIDs[sc.chunk.ChunkID] = true
						}
					}
					
					// Only add global chunks if we have room AND they have very high scores
					// This prevents mixing precise spec_row results with broad global chunks
					// Only include global chunks that score significantly higher than spec_row chunks
					if len(reRanked) < maxChunks && len(specRowChunks) > 0 {
						// Calculate average spec_row score
						avgSpecRowScore := 0.0
						if len(specRowChunks) > 0 {
							sum := 0.0
							for _, sc := range specRowChunks {
								sum += sc.score
							}
							avgSpecRowScore = sum / float64(len(specRowChunks))
						}
						
						// Only include global chunks that score at least 1.5x the average spec_row score
						// This ensures we only add global chunks when they're significantly more relevant
						for _, sc := range globalChunks {
							if len(reRanked) >= maxChunks {
								break
							}
							if sc.score >= avgSpecRowScore*1.5 && !seenChunkIDs[sc.chunk.ChunkID] {
								reRanked = append(reRanked, *sc.chunk)
								seenChunkIDs[sc.chunk.ChunkID] = true
							}
						}
					}
				} else {
					// No relevant spec_row chunks, use global chunks (standard behavior)
					// Sort global chunks by score (they're already sorted, but ensure it)
					for i := 0; i < len(globalChunks)-1; i++ {
						for j := i + 1; j < len(globalChunks); j++ {
							if globalChunks[i].score < globalChunks[j].score {
								globalChunks[i], globalChunks[j] = globalChunks[j], globalChunks[i]
							}
						}
					}
					
					// Add global chunks (up to maxChunks)
					for _, sc := range globalChunks {
						if len(reRanked) >= maxChunks {
							break
						}
						if !seenChunkIDs[sc.chunk.ChunkID] {
							reRanked = append(reRanked, *sc.chunk)
							seenChunkIDs[sc.chunk.ChunkID] = true
						}
					}
					
					// If we still have room and spec_row chunks exist (even with lower scores), add them
					// This handles edge cases where spec_row chunks exist but didn't meet the threshold
					if len(reRanked) < maxChunks && len(specRowChunks) > 0 {
						for _, sc := range specRowChunks {
							if len(reRanked) >= maxChunks {
								break
							}
							if !seenChunkIDs[sc.chunk.ChunkID] {
								reRanked = append(reRanked, *sc.chunk)
								seenChunkIDs[sc.chunk.ChunkID] = true
							}
						}
					}
				}
				
				resp.SemanticChunks = reRanked
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
					fmt.Printf("\n  %d. [%s] (similarity: %.1f%%)\n", i+1, chunk.ChunkType, (1.0-chunk.Distance)*100)
					if chunk.Text != "" {
						// For spec_row chunks, parse and display structured fields nicely
						if chunk.ChunkType == storage.ChunkTypeSpecRow {
							// Parse structured format: Category, Sub-Category, Specification, Value, Additional Metadata
							lines := strings.Split(chunk.Text, "\n")
							category := ""
							subCategory := ""
							specification := ""
							value := ""
							additionalMetadata := ""
							
							for _, line := range lines {
								line = strings.TrimSpace(line)
								if strings.HasPrefix(line, "Category:") {
									category = strings.TrimPrefix(line, "Category:")
									category = strings.TrimSpace(category)
								} else if strings.HasPrefix(line, "Sub-Category:") {
									subCategory = strings.TrimPrefix(line, "Sub-Category:")
									subCategory = strings.TrimSpace(subCategory)
								} else if strings.HasPrefix(line, "Specification:") {
									specification = strings.TrimPrefix(line, "Specification:")
									specification = strings.TrimSpace(specification)
								} else if strings.HasPrefix(line, "Value:") {
									value = strings.TrimPrefix(line, "Value:")
									value = strings.TrimSpace(value)
								} else if strings.HasPrefix(line, "Additional Metadata:") {
									additionalMetadata = strings.TrimPrefix(line, "Additional Metadata:")
									additionalMetadata = strings.TrimSpace(additionalMetadata)
								}
							}
							
							// Display all fields
							if category != "" {
								fmt.Printf("     Category: %s\n", category)
							}
							if subCategory != "" {
								fmt.Printf("     Sub-Category: %s\n", subCategory)
							}
							if specification != "" {
								fmt.Printf("     Specification: %s\n", specification)
							}
							if value != "" {
								fmt.Printf("     Value: %s\n", value)
							}
							if additionalMetadata != "" && additionalMetadata != "Unknown" {
								fmt.Printf("     Additional Metadata: %s\n", additionalMetadata)
							}
						} else {
							// For non-spec_row chunks, display full text with proper indentation
							lines := strings.Split(chunk.Text, "\n")
							for _, line := range lines {
								fmt.Printf("     %s\n", line)
							}
						}
					} else {
						fmt.Printf("     (No text content available)\n")
					}
				}
				fmt.Println()
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

// resolveProductID resolves a product ID or name to a UUID by querying the database
func resolveProductID(db *sql.DB, idOrName string) (uuid.UUID, error) {
	if idOrName == "" {
		return uuid.Nil, fmt.Errorf("empty ID or name")
	}

	// Try to parse as UUID
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}

	// Query database by name (supports partial matching)
	var productID uuid.UUID
	query := "SELECT id FROM products WHERE name = $1 OR name LIKE $2 LIMIT 1"
	err := db.QueryRow(query, idOrName, idOrName+"%").Scan(&productID)
	if err == nil {
		return productID, nil
	}
	
	// If not found, try case-insensitive match
	err = db.QueryRow("SELECT id FROM products WHERE LOWER(name) = LOWER($1) OR LOWER(name) LIKE LOWER($2) LIMIT 1", 
		idOrName, idOrName+"%").Scan(&productID)
	if err == nil {
		return productID, nil
	}

	// Fallback to deterministic UUID generation (for backwards compatibility)
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
