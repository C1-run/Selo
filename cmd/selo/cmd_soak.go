package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/C1-run/selo/internal/soak"
	"github.com/spf13/cobra"
)

var (
	soakDuration     string
	soakTaskCount    int
	soakInterval     string
	soakFixtureMode  bool
	soakStopOnSafety bool
	soakOutDir       string
	soakRunner       string
)

var soakCmd = &cobra.Command{
	Use:   "soak",
	Short: "Run a soak test to stress the Selo pipeline",
	RunE:  runSoakCmd,
}

func init() {
	soakCmd.Flags().StringVar(&soakDuration, "duration", "24h", "Soak duration")
	soakCmd.Flags().IntVar(&soakTaskCount, "task-count", 0, "Max tasks to process (0=unlimited)")
	soakCmd.Flags().StringVar(&soakInterval, "interval", "1m", "Poll interval")
	soakCmd.Flags().BoolVar(&soakFixtureMode, "fixture-mode", true, "Use generated fixture tasks")
	soakCmd.Flags().BoolVar(&soakStopOnSafety, "stop-on-safety", true, "Stop on safety failure")
	soakCmd.Flags().StringVar(&soakOutDir, "out", "runs/soak", "Output directory")
	soakCmd.Flags().StringVar(&soakRunner, "runner", "", "Runner type: opencode (requires SELO_OPENCODE_BIN)")
}

func runSoakCmd(cmd *cobra.Command, args []string) error {
	cfg := soak.SoakConfig{
		Duration:     24 * time.Hour,
		TaskCount:    0,
		Interval:     1 * time.Minute,
		FixtureMode:  true,
		StopOnSafety: true,
		OutDir:       "runs/soak",
	}

	if d, err := time.ParseDuration(soakDuration); err == nil {
		cfg.Duration = d
	}
	cfg.TaskCount = soakTaskCount
	if i, err := time.ParseDuration(soakInterval); err == nil {
		cfg.Interval = i
	}
	cfg.FixtureMode = soakFixtureMode
	cfg.StopOnSafety = soakStopOnSafety
	cfg.OutDir = soakOutDir

	// OpenCode soak mode
	if soakRunner == "opencode" {
		ocBin := os.Getenv("SELO_OPENCODE_BIN")
		if ocBin == "" {
			return fmt.Errorf("--runner opencode requires SELO_OPENCODE_BIN env var")
		}
		ocModel := os.Getenv("SELO_OPENCODE_MODEL")
		if ocModel == "" {
			ocModel = "opencode/deepseek-v4-flash-free"
		}
		runOpenCodeSoak(cfg, ocBin, ocModel)
		return nil
	}

	fmt.Printf("[selo] Starting soak: duration=%v, interval=%v, fixture=%v, out=%s\n",
		cfg.Duration, cfg.Interval, cfg.FixtureMode, cfg.OutDir)
	fmt.Printf("[selo] PID %d\n", os.Getpid())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n[selo] Soak interrupted by signal.")
		cancel()
	}()

	runner := soak.NewSoakRunner(cfg)
	summary, err := runner.Run(ctx)
	if err != nil {
		return fmt.Errorf("soak error: %w", err)
	}

	fmt.Printf("\n=== Soak Complete ===\n")
	fmt.Printf("Summary: %s/SOAK_SUMMARY.json\n", cfg.OutDir)
	fmt.Printf("Final Verdict: %s\n", summary.FinalVerdict)

	if summary.FinalVerdict == "FAIL_UNSAFE" || summary.FinalVerdict == "FAIL_DAEMON_UNSTABLE" {
		os.Exit(1)
	}
	return nil
}
