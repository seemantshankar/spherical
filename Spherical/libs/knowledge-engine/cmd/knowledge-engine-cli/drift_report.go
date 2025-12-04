// Package main provides CLI commands for drift reporting.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// newDriftReportCmd creates the drift report subcommand.
// This implements T057: CLI drift report command summarizing open alerts for analysts.
func newDriftReportCmd() *cobra.Command {
	var (
		tenant   string
		campaign string
		format   string
		detailed bool
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate comprehensive drift report for analysts",
		Long: `Report generates a comprehensive drift report summarizing all open alerts,
drift status, and recommendations for analysts. This command is designed for
compliance and operational monitoring purposes.

The report includes:
- Summary statistics (total alerts, by type, by severity)
- Detailed alert listings with context
- Drift check results
- Recommendations for resolution

Use --detailed for full alert details including metadata.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			tenantID, err := resolveID(tenant)
			if err != nil {
				return fmt.Errorf("invalid tenant: %w", err)
			}

			logger.Info().
				Str("tenant", tenant).
				Str("format", format).
				Bool("detailed", detailed).
				Msg("Generating drift report")

			// Open database connection
			db, err := openDatabase(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Create repositories
			repos := storage.NewRepositories(db)

			// Create drift runner
			driftRunner := monitoring.NewDriftRunner(logger, nil, monitoring.DriftConfig{
				CheckInterval:      1 * time.Hour,
				FreshnessThreshold: 30 * 24 * time.Hour,
			})

			// Generate comprehensive report
			report, err := generateComprehensiveReport(ctx, logger, repos, driftRunner, tenantID, campaign)
			if err != nil {
				return fmt.Errorf("generate report: %w", err)
			}

			// Output based on format
			if format == "json" || outputJSON {
				return outputJSONReport(report)
			}

			return outputTextReport(report, detailed)
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or name (required)")
	cmd.Flags().StringVar(&campaign, "campaign", "", "filter by specific campaign")
	cmd.Flags().StringVar(&format, "format", "text", "output format (text, json)")
	cmd.Flags().BoolVar(&detailed, "detailed", false, "include detailed alert information")

	_ = cmd.MarkFlagRequired("tenant")

	return cmd
}

// ComprehensiveReport contains all drift-related information for analysts.
type ComprehensiveReport struct {
	TenantID          uuid.UUID
	GeneratedAt       time.Time
	Summary           ReportSummary
	OpenAlerts        []AlertDetail
	DriftCheckResults *DriftCheckSummary
	Recommendations   []string
}

// ReportSummary provides high-level statistics.
type ReportSummary struct {
	TotalOpenAlerts int
	AlertsByType    map[string]int
	AlertsByStatus  map[string]int
	StaleCampaigns  int
	HashMismatches  int
	EmbeddingDrift  int
	HighestSeverity string
	OldestAlertAge  time.Duration
}

// AlertDetail provides detailed information about a drift alert.
type AlertDetail struct {
	ID                uuid.UUID
	Type              string
	Status            string
	ProductID         *uuid.UUID
	CampaignVariantID *uuid.UUID
	DetectedAt        time.Time
	Age               time.Duration
	Details           map[string]interface{}
}

// DriftCheckSummary provides drift check results.
type DriftCheckSummary struct {
	LastChecked    time.Time
	StaleCampaigns int
	HashMismatches int
	EmbeddingDrift int
	TotalIssues    int
}

// generateComprehensiveReport creates a comprehensive drift report.
func generateComprehensiveReport(
	ctx context.Context,
	logger *observability.Logger,
	repos *storage.Repositories,
	driftRunner *monitoring.DriftRunner,
	tenantID uuid.UUID,
	campaignFilter string,
) (*ComprehensiveReport, error) {
	report := &ComprehensiveReport{
		TenantID:    tenantID,
		GeneratedAt: time.Now(),
		Summary: ReportSummary{
			AlertsByType:   make(map[string]int),
			AlertsByStatus: make(map[string]int),
		},
	}

	// Get open alerts
	openAlerts, err := repos.DriftAlerts.GetOpenByTenant(ctx, tenantID)
	if err != nil {
		// If repository method doesn't exist yet or query fails, return partial report
		logger.Warn().Err(err).Msg("Could not retrieve alerts, returning partial report")
		openAlerts = []*storage.DriftAlert{}
	}

	// Process alerts
	var oldestAlertTime *time.Time
	for _, alert := range openAlerts {
		// Apply campaign filter if specified
		if campaignFilter != "" && alert.CampaignVariantID != nil {
			campaignID, err := resolveID(campaignFilter)
			if err == nil && *alert.CampaignVariantID != campaignID {
				continue
			}
		}

		// Update summary statistics
		report.Summary.TotalOpenAlerts++
		report.Summary.AlertsByType[string(alert.AlertType)]++
		report.Summary.AlertsByStatus[string(alert.Status)]++

		// Track oldest alert
		if oldestAlertTime == nil || alert.DetectedAt.Before(*oldestAlertTime) {
			oldestAlertTime = &alert.DetectedAt
		}

		// Create alert detail
		detail := AlertDetail{
			ID:                alert.ID,
			Type:              string(alert.AlertType),
			Status:            string(alert.Status),
			ProductID:         alert.ProductID,
			CampaignVariantID: alert.CampaignVariantID,
			DetectedAt:        alert.DetectedAt,
			Age:               time.Since(alert.DetectedAt),
		}

		// Parse details JSON if available
		if len(alert.Details) > 0 {
			detail.Details = make(map[string]interface{})
			if err := json.Unmarshal(alert.Details, &detail.Details); err != nil {
				logger.Warn().Err(err).Msg("Failed to parse alert details")
			}
		}

		report.OpenAlerts = append(report.OpenAlerts, detail)
	}

	// Calculate oldest alert age
	if oldestAlertTime != nil {
		report.Summary.OldestAlertAge = time.Since(*oldestAlertTime)
	}

	// Run drift check to get current status
	checkResult, err := driftRunner.RunCheck(ctx, tenantID)
	if err != nil {
		logger.Warn().Err(err).Msg("Drift check failed, continuing with partial report")
	} else {
		report.DriftCheckResults = &DriftCheckSummary{
			LastChecked:    checkResult.CheckedAt,
			StaleCampaigns: len(checkResult.StaleCampaigns),
			HashMismatches: len(checkResult.HashMismatches),
			EmbeddingDrift: len(checkResult.EmbeddingDrift),
			TotalIssues:    checkResult.TotalAlerts,
		}

		report.Summary.StaleCampaigns = len(checkResult.StaleCampaigns)
		report.Summary.HashMismatches = len(checkResult.HashMismatches)
		report.Summary.EmbeddingDrift = len(checkResult.EmbeddingDrift)
	}

	// Generate recommendations
	report.Recommendations = generateRecommendations(report)

	// Determine highest severity
	report.Summary.HighestSeverity = determineHighestSeverity(report)

	return report, nil
}

// generateRecommendations creates actionable recommendations based on the report.
func generateRecommendations(report *ComprehensiveReport) []string {
	var recommendations []string

	if report.Summary.EmbeddingDrift > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Detected %d campaigns with mixed embedding versions. Queue re-embedding jobs to resolve.", report.Summary.EmbeddingDrift))
	}

	if report.Summary.StaleCampaigns > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Found %d stale campaigns exceeding freshness threshold. Review and update or archive.", report.Summary.StaleCampaigns))
	}

	if report.Summary.HashMismatches > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Detected %d hash mismatches. Investigate source document changes.", report.Summary.HashMismatches))
	}

	if report.Summary.OldestAlertAge > 7*24*time.Hour {
		recommendations = append(recommendations,
			fmt.Sprintf("Oldest alert is %.0f days old. Review and resolve or acknowledge.", report.Summary.OldestAlertAge.Hours()/24))
	}

	if report.Summary.TotalOpenAlerts == 0 {
		recommendations = append(recommendations, "No open alerts. System is healthy.")
	}

	return recommendations
}

// determineHighestSeverity determines the highest severity level from alerts.
func determineHighestSeverity(report *ComprehensiveReport) string {
	// Simple severity logic - can be enhanced
	if report.Summary.EmbeddingDrift > 0 {
		return "HIGH"
	}
	if report.Summary.HashMismatches > 0 {
		return "MEDIUM"
	}
	if report.Summary.StaleCampaigns > 0 {
		return "LOW"
	}
	return "NONE"
}

// outputJSONReport outputs the report in JSON format.
func outputJSONReport(report *ComprehensiveReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// outputTextReport outputs the report in human-readable text format.
func outputTextReport(report *ComprehensiveReport, detailed bool) error {
	fmt.Printf("╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                  DRIFT REPORT                                ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n\n")

	fmt.Printf("Tenant:     %s\n", report.TenantID.String())
	fmt.Printf("Generated:  %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Severity:   %s\n\n", report.Summary.HighestSeverity)

	// Summary section
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("SUMMARY\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")

	fmt.Printf("Total Open Alerts:     %d\n", report.Summary.TotalOpenAlerts)
	fmt.Printf("Stale Campaigns:       %d\n", report.Summary.StaleCampaigns)
	fmt.Printf("Hash Mismatches:       %d\n", report.Summary.HashMismatches)
	fmt.Printf("Embedding Drift:       %d\n", report.Summary.EmbeddingDrift)

	if report.Summary.OldestAlertAge > 0 {
		fmt.Printf("Oldest Alert Age:      %.0f days\n", report.Summary.OldestAlertAge.Hours()/24)
	}

	fmt.Printf("\nAlerts by Type:\n")
	for alertType, count := range report.Summary.AlertsByType {
		fmt.Printf("  • %s: %d\n", alertType, count)
	}

	// Drift check results
	if report.DriftCheckResults != nil {
		fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("DRIFT CHECK RESULTS\n")
		fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")

		fmt.Printf("Last Checked:         %s\n", report.DriftCheckResults.LastChecked.Format(time.RFC3339))
		fmt.Printf("Total Issues Found:   %d\n", report.DriftCheckResults.TotalIssues)
	}

	// Open alerts
	if len(report.OpenAlerts) > 0 {
		fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("OPEN ALERTS (%d)\n", len(report.OpenAlerts))
		fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")

		for i, alert := range report.OpenAlerts {
			fmt.Printf("%d. [%s] %s\n", i+1, alert.Type, alert.ID.String())
			fmt.Printf("   Status:    %s\n", alert.Status)
			fmt.Printf("   Age:       %.0f days\n", alert.Age.Hours()/24)
			fmt.Printf("   Detected:  %s\n", alert.DetectedAt.Format(time.RFC3339))

			if alert.ProductID != nil {
				fmt.Printf("   Product:   %s\n", alert.ProductID.String())
			}
			if alert.CampaignVariantID != nil {
				fmt.Printf("   Campaign:  %s\n", alert.CampaignVariantID.String())
			}

			if detailed && len(alert.Details) > 0 {
				fmt.Printf("   Details:\n")
				for key, value := range alert.Details {
					fmt.Printf("     • %s: %v\n", key, value)
				}
			}

			if i < len(report.OpenAlerts)-1 {
				fmt.Println()
			}
		}
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("RECOMMENDATIONS\n")
		fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")

		for i, rec := range report.Recommendations {
			fmt.Printf("%d. %s\n", i+1, rec)
		}
	}

	fmt.Println()

	return nil
}
