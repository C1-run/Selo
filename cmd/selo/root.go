package main

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd is the top-level command.
var rootCmd = &cobra.Command{
	Use:          "selo",
	Short:        "Selo — the audit brain for AI coding agents",
	Long:         `Selo is a safety-first task runner that wraps any coding agent and produces a cryptographic audit trail.`,
	SilenceUsage: true,
}

var globalCfgFile string

func init() {
	rootCmd.PersistentFlags().StringVar(&globalCfgFile, "config", "config/selo.yaml", "Path to config file")
	rootCmd.AddCommand(
		daemonCmd,
		runCmd,
		checkCmd,
		verifyCmd,
		receiptCmd,
		keysCmd,
		statusCmd,
		initCmd,
		smokeCmd,
		soakCmd,
		completionCmd,
	)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
