package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	
	"github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/orchestrator/ui"
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/extraction"
)

var (
	extractPDFPath    string
	extractOutputPath string
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract content from a PDF brochure",
	Long:  "Extract structured content from a PDF brochure and save it as markdown.",
	RunE:  runExtract,
}

func init() {
	extractCmd.Flags().StringVarP(&extractPDFPath, "pdf", "p", "", "Path to PDF file (required)")
	extractCmd.Flags().StringVarP(&extractOutputPath, "output", "o", "", "Output path for markdown file (optional)")
	extractCmd.MarkFlagRequired("pdf")
	rootCmd.AddCommand(extractCmd)
}

func runExtract(cmd *cobra.Command, args []string) error {
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
	
	ui.Section("PDF Extraction")
	
	// Validate PDF file
	if extractPDFPath == "" {
		return fmt.Errorf("PDF file path is required (use --pdf flag)")
	}
	
	// Get API key and model
	apiKey, err := cfg.GetOpenRouterAPIKey()
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}
	model := cfg.GetLLMModel()
	
	// Determine output path
	if extractOutputPath == "" {
		baseName := filepath.Base(extractPDFPath)
		extractOutputPath = filepath.Join(filepath.Dir(extractPDFPath), 
			fmt.Sprintf("%s-specs.md", 
				filepath.Base(baseName[:len(baseName)-len(filepath.Ext(baseName))])))
	}
	
	ui.Info("PDF file: %s", extractPDFPath)
	ui.Info("Output file: %s", extractOutputPath)
	ui.Newline()
	
	// Create extractor
	extractor := extraction.NewOrchestrator(apiKey, model)
	
	// Show progress
	spinner := ui.NewSpinner("Extracting content from PDF...")
	spinner.Start()
	
	// Extract
	result, err := extractor.Extract(ctx, extractPDFPath, extractOutputPath)
	spinner.Stop()
	
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}
	
	ui.Success("✓ Extraction completed successfully!")
	ui.Newline()
	ui.Section("Extraction Summary")
	ui.Table([]string{"Metric", "Value"}, [][]string{
		{"Output File", result.MarkdownPath},
		{"Duration", ui.FormatDuration(result.Duration)},
		{"Make", result.Metadata.Make},
		{"Model", result.Metadata.Model},
		{"Year", fmt.Sprintf("%d", result.Metadata.ModelYear)},
		{"Domain", result.Metadata.Domain},
	})
	
	ui.Newline()
	ui.Success("Markdown saved to: %s", result.MarkdownPath)
	
	return nil
}

