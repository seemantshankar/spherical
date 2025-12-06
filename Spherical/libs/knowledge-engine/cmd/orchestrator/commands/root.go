package commands

import (
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	verbose    bool
	noColor    bool
)

var rootCmd = &cobra.Command{
	Use:   "orchestrator",
	Short: "Product Knowledge Orchestrator - Unified CLI for campaign management",
	Long: `The Orchestrator provides a user-friendly interface to manage product campaigns,
extract specifications from PDF brochures, ingest content into knowledge databases,
and query campaigns with natural language questions.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Configuration and initialization will happen here
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

