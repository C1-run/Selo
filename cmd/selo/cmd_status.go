package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/C1-run/selo/internal/queue"
	"github.com/spf13/cobra"
)

var (
	statusDir  string
	statusJSON bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon/queue status and recent receipts",
	RunE:  runStatusCmd,
}

func init() {
	statusCmd.Flags().StringVar(&statusDir, "dir", "", "Base directory (default: auto-detect)")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	baseDir := statusDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}

	qm := queue.NewQueueManager(baseDir)

	type statusInfo struct {
		BaseDir      string   `json:"base_dir"`
		LockExists   bool     `json:"lock_exists"`
		PendingCount int      `json:"pending_count"`
		RunningCount int      `json:"running_count"`
		DoneCount    int      `json:"done_count"`
		FailedCount  int      `json:"failed_count"`
		RecentTasks  []string `json:"recent_tasks"`
	}

	info := statusInfo{
		BaseDir: baseDir,
	}

	// Check daemon lock
	lockPath := qm.DaemonLockPath()
	if _, err := os.Stat(lockPath); err == nil {
		info.LockExists = true
	}

	// Count pending tasks
	if pending, err := qm.ScanPending(); err == nil {
		info.PendingCount = len(pending)
	}

	// Count running tasks
	runningDir := filepath.Join(baseDir, "queue", "running")
	if entries, err := os.ReadDir(runningDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				info.RunningCount++
			}
		}
	}

	// Count done tasks
	doneDir := filepath.Join(baseDir, "queue", "done")
	if entries, err := os.ReadDir(doneDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				info.DoneCount++
			}
		}
	}

	// Count failed tasks
	failedDir := filepath.Join(baseDir, "queue", "failed")
	if entries, err := os.ReadDir(failedDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				info.FailedCount++
			}
		}
	}

	// Recent receipts
	runsDir := filepath.Join(baseDir, "runs")
	if entries, err := os.ReadDir(runsDir); err == nil {
		count := 0
		for i := len(entries) - 1; i >= 0 && count < 5; i-- {
			if entries[i].IsDir() && strings.HasPrefix(entries[i].Name(), "run-") {
				info.RecentTasks = append(info.RecentTasks, entries[i].Name())
				count++
			}
		}
	}

	if statusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Printf("Selo Status\n")
	fmt.Printf("  Base dir:    %s\n", info.BaseDir)
	fmt.Printf("  Daemon:      %s\n", boolStr(info.LockExists, "running", "not running"))
	fmt.Printf("  Pending:     %d tasks\n", info.PendingCount)
	fmt.Printf("  Running:     %d tasks\n", info.RunningCount)
	fmt.Printf("  Done:        %d tasks\n", info.DoneCount)
	fmt.Printf("  Failed:      %d tasks\n", info.FailedCount)
	if len(info.RecentTasks) > 0 {
		fmt.Printf("  Recent:      %s\n", strings.Join(info.RecentTasks, ", "))
	}
	return nil
}

func boolStr(b bool, trueVal, falseVal string) string {
	if b {
		return trueVal
	}
	return falseVal
}
