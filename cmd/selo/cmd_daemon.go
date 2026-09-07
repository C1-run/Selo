package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/selo-dev/selo/internal/queue"
	"github.com/selo-dev/selo/internal/receipt"
	"github.com/selo-dev/selo/internal/workspace"
	"github.com/spf13/cobra"
)

var daemonOneShot bool

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the Selo daemon to process queued tasks",
	RunE:  runDaemon,
}

func init() {
	daemonCmd.Flags().BoolVar(&daemonOneShot, "one-shot", false, "Process one pending task and exit")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	baseDir, err := findBaseDir(globalCfgFile)
	if err != nil {
		return fmt.Errorf("cannot find base dir: %w", err)
	}

	cfg := loadConfig(globalCfgFile, baseDir)

	qm := queue.NewQueueManager(baseDir)
	receiptWriter := receipt.NewReceiptWriter(qm.ReceiptsDir(), qm.RunsDir())
	worktreeMgr := workspace.NewWorktreeManager(filepath.Join(baseDir, "worktrees"))

	lockPath := qm.DaemonLockPath()
	lock, err := queue.WriteLock(lockPath, "daemon")
	if err != nil {
		return fmt.Errorf("cannot acquire daemon lock: %w", err)
	}
	defer lock.Remove()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("[selo] Daemon started (PID %d, base: %s)\n", os.Getpid(), baseDir)

	if daemonOneShot {
		processOneTask(qm, receiptWriter, worktreeMgr, cfg)
		return nil
	}

	pollInterval := time.Duration(cfg.Forge.PollIntervalSec) * time.Second
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	for {
		if cfg.Forge.StopFile != "" {
			if _, err := os.Stat(filepath.Join(baseDir, cfg.Forge.StopFile)); err == nil {
				fmt.Println("[selo] Stop file detected. Shutting down.")
				os.Remove(filepath.Join(baseDir, cfg.Forge.StopFile))
				break
			}
		}

		select {
		case <-sigCh:
			fmt.Println("[selo] Signal received. Shutting down.")
			return nil
		default:
		}

		processed := processOneTask(qm, receiptWriter, worktreeMgr, cfg)
		if !processed {
			time.Sleep(pollInterval)
		}
	}

	return nil
}
