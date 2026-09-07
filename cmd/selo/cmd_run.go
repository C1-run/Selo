package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/selo-dev/selo/internal/queue"
	"github.com/selo-dev/selo/internal/receipt"
	"github.com/selo-dev/selo/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runRepo           string
	runCommands       []string
	runMaxMinutes     int
	runForbiddenFiles []string
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
	processed := processOneTask(qm, receiptWriter, worktreeMgr, cfg)
	if !processed {
		return fmt.Errorf("task %s was not processed", taskID)
	}

	// Find and display the receipt
	receiptPath := filepath.Join(qm.RunsDir(), "run-"+taskID, "receipt.json")
	if _, err := os.Stat(receiptPath); err == nil {
		fmt.Printf("[selo] Receipt: %s\n", receiptPath)
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
