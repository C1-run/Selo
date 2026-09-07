package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var smokeCmd = &cobra.Command{
	Use:   "smoke [command]",
	Short: "Run smoke tests against the configured runner",
	Long: `Available smoke tests:
  c1-loop        Smoke test the C1 Loop runner
  actual-c1      Smoke test the actual C1 runner
  c1-runtimes    Smoke test C1 runtime availability
  opencode       Smoke test OpenCode integration`,
	Args: cobra.ExactArgs(1),
	RunE: runSmokeCmd,
}

func runSmokeCmd(cmd *cobra.Command, args []string) error {
	sub := args[0]
	allowShim := len(args) > 1 && args[1] == "--allow-shim"

	switch sub {
	case "c1-loop":
		smokeC1Loop()
	case "actual-c1":
		smokeActualC1(allowShim)
	case "c1-runtimes":
		smokeC1Runtimes()
	case "opencode":
		smokeOpenCode()
	default:
		fmt.Fprintf(os.Stderr, "unknown smoke command: %q\n", sub)
		fmt.Fprintln(os.Stderr, "Available: c1-loop, actual-c1, c1-runtimes, opencode")
		os.Exit(1)
	}
	return nil
}
