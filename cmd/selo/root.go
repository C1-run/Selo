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

// Version is the Selo build version. Override at build time with
// -ldflags "-X main.Version=x.y.z" (the release workflow passes the git tag).
// It is recorded in every receipt as selo_version so a verifier knows which
// rule set produced the receipt.
var Version = "dev"

func init() {
	rootCmd.Version = Version
	rootCmd.PersistentFlags().StringVar(&globalCfgFile, "config", "config/selo.yaml", "Path to config file")
	rootCmd.AddCommand(
		daemonCmd,
		runCmd,
		checkCmd,
		verifyCmd,
		receiptCmd,
		keysCmd,
		mcpCmd,
		statusCmd,
		initCmd,
		smokeCmd,
		selftestCmd,
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
