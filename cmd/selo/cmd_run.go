package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/C1-run/selo/internal/queue"
	"github.com/C1-run/selo/internal/receipt"
	"github.com/C1-run/selo/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runRepo           string
	runCommands       []string
	runMaxMinutes     int
	runForbiddenFiles []string
	runDev            bool
	runSigner         string
	runTSA            string
	runTSASoft        bool
	runRekor          string
	runRekorSoft      bool
)

var runCmd = &cobra.Command{
	Use:   "run [goal]",
	Short: "Run a single task through the safety pipeline",
	Long: `Creates a temporary task in the queue, processes it with the full safety
pipeline (governor, scans, Pinocchio, GateChain), produces a receipt, and exits.

Always produces a receipt — consistent with daemon behavior.`,
	Args: cobra.ExactArgs(1),
	RunE: runRunCmd,
}

func init() {
	runCmd.Flags().StringVar(&runRepo, "repo", "", "Git repository path (default: cwd)")
	runCmd.Flags().StringArrayVar(&runCommands, "commands", nil, "Test commands to run (repeatable)")
	runCmd.Flags().IntVar(&runMaxMinutes, "max-minutes", 30, "Max minutes for the agent")
	runCmd.Flags().StringArrayVar(&runForbiddenFiles, "forbidden-files", nil, "Files the agent must not modify")
	runCmd.Flags().BoolVar(&runDev, "dev", false, "Allow a per-process ephemeral signing key when SELO_SIGNING_KEY is unset (dev only — receipts are not attributable across runs)")
	runCmd.Flags().StringVar(&runSigner, "signer", "", "Signing backend: file (default), keychain, or command (see ADR-008). Overrides SELO_SIGNER.")
	runCmd.Flags().StringVar(&runTSA, "tsa", "", "RFC3161 timestamp authority URL (see ADR-005). The receipt gains an externally verifiable signing time.")
	runCmd.Flags().BoolVar(&runTSASoft, "tsa-soft", false, "If the TSA is unreachable, record the timestamp as absent instead of failing the run (ADR-005)")
	runCmd.Flags().StringVar(&runRekor, "rekor", "", "Sigstore Rekor transparency-log URL (see ADR-006). Publishes the receipt hash so the run is publicly witnessed.")
	runCmd.Flags().BoolVar(&runRekorSoft, "rekor-soft", false, "If the transparency log is unreachable, record the entry as absent instead of failing the run (ADR-006)")
}

func runRunCmd(cmd *cobra.Command, args []string) error {
	goal := args[0]

	if runRepo == "" {
		var err error
		runRepo, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine cwd: %w", err)
		}
	}

	// Resolve base dir from config
	baseDir, err := findBaseDir(globalCfgFile)
	if err != nil {
		return fmt.Errorf("cannot find base dir: %w", err)
	}

	cfg := loadConfig(globalCfgFile, baseDir)
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	qm := queue.NewQueueManager(baseDir)
	receiptWriter := receipt.NewReceiptWriter(qm.ReceiptsDir(), qm.RunsDir())
	worktreeMgr := workspace.NewWorktreeManager(filepath.Join(baseDir, "worktrees"))

	// Generate a task ID
	taskID := fmt.Sprintf("run-%s", generateShortID())

	// Write task.md to pending queue
	taskDir := qm.PendingDir()
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return fmt.Errorf("creating pending dir: %w", err)
	}

	taskPath := filepath.Join(taskDir, taskID+".md")
	taskContent := fmt.Sprintf(`id: "%s"
goal: "%s"
repo: "%s"
commands:
`, taskID, goal, runRepo)
	for _, c := range runCommands {
		taskContent += fmt.Sprintf(`  - "%s"
`, c)
	}
	if runMaxMinutes > 0 {
		taskContent += fmt.Sprintf("max_minutes: %d\n", runMaxMinutes)
	}
	if len(runForbiddenFiles) > 0 {
		taskContent += "forbidden_files:\n"
		for _, f := range runForbiddenFiles {
			taskContent += fmt.Sprintf(`  - "%s"
`, f)
		}
	}

	if err := os.WriteFile(taskPath, []byte(taskContent), 0644); err != nil {
		return fmt.Errorf("writing task file: %w", err)
	}

	fmt.Printf("[selo] Task %s queued: %s\n", taskID, goal)

	// Process the task (reuses daemon logic)
	if runDev {
		os.Setenv("SELO_ALLOW_EPHEMERAL_KEY", "1")
	}
	if runSigner != "" {
		os.Setenv("SELO_SIGNER", runSigner)
	}
	if runTSA != "" {
		os.Setenv("SELO_TSA_URL", runTSA)
	}
	if runTSASoft {
		os.Setenv("SELO_TSA_SOFT", "1")
	}
	if runRekor != "" {
		os.Setenv("SELO_REKOR_URL", runRekor)
	}
	if runRekorSoft {
		os.Setenv("SELO_REKOR_SOFT", "1")
	}
	processed := processOneTask(qm, receiptWriter, worktreeMgr, cfg)
	if !processed {
		return fmt.Errorf("task %s was not processed", taskID)
	}

	// Find and display the receipt
	receiptPath := filepath.Join(qm.RunsDir(), "run-"+taskID, "receipt.json")
	if _, err := os.Stat(receiptPath); err == nil {
		fmt.Printf("[selo] Receipt: %s\n", receiptPath)
	}

	// Fail closed: a production run (no --dev) must not silently emit an
	// unsigned receipt. Signing fails closed when SELO_SIGNING_KEY is unset,
	// so an unsigned file here means the run is misconfigured and the receipt
	// cannot serve as evidence of what happened.
	if !runDev {
		if receiptData, rerr := os.ReadFile(receiptPath); rerr == nil {
			var signed struct {
				Signature string `json:"signature"`
			}
			if json.Unmarshal(receiptData, &signed) == nil && signed.Signature == "" {
				return fmt.Errorf("receipt was not signed: run `selo keys generate` (Selo loads %s automatically) or set SELO_SIGNING_KEY, so receipts are attributable across runs; or pass --dev to accept an ephemeral per-process key", receipt.DefaultSigningKeyPath())
			}
		}
	}

	// Spec §2.2 exit codes: 0 success/noop, 1 failure, 2 timeout. A rejected
	// task must fail the command so shell pipelines and CI stop on it.
	if receiptData, err := os.ReadFile(receiptPath); err == nil {
		var r struct {
			Verdict      string `json:"verdict"`
			FinalVerdict string `json:"final_verdict"`
		}
		if json.Unmarshal(receiptData, &r) == nil {
			verdict := r.FinalVerdict
			if verdict == "" {
				verdict = r.Verdict
			}
			switch {
			case verdict == "FAILED_TIMEOUT":
				os.Exit(2)
			case strings.HasPrefix(verdict, "FAILED"):
				fmt.Printf("[selo] Task rejected: %s\n", verdict)
				os.Exit(1)
			}
		}
	}

	return nil
}

// generateShortID creates a short random ID for run commands.
func generateShortID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(1) // ensure unique nanosecond
	}
	return string(b)
}
