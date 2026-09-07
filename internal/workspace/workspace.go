package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WorktreeManager creates and removes git worktrees for isolated task execution.
type WorktreeManager struct {
	WorkDir string // base directory where worktrees are created
}

// NewWorktreeManager creates a new worktree manager.
func NewWorktreeManager(workDir string) *WorktreeManager {
	return &WorktreeManager{WorkDir: workDir}
}

// CreateWorktree creates a git worktree from the given repo at a named branch.
// Returns the absolute path to the worktree.
func (wm *WorktreeManager) CreateWorktree(repoPath, taskID string) (string, error) {
	wtPath := filepath.Join(wm.WorkDir, fmt.Sprintf("wt-%s", taskID))
	os.MkdirAll(wm.WorkDir, 0755)
	// Resolve to absolute path
	if absPath, err := filepath.Abs(wtPath); err == nil {
		wtPath = absPath
	}

	// Remove existing worktree directory if present
	if _, err := os.Stat(wtPath); err == nil {
		// Try to remove through git first, then force delete
		exec.Command("git", "worktree", "remove", "--force", wtPath).Run()
		os.RemoveAll(wtPath)
	}

	branchName := fmt.Sprintf("selo/%s", taskID)

	// Remove existing branch if present
	exec.Command("git", "branch", "-D", branchName, "2>/dev/null").Run()

	// Create a new branch from HEAD
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtPath, "HEAD")
	cmd.Dir = repoPath
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		return "", fmt.Errorf("create worktree: %w\n%s", err, string(out))
	}

	return wtPath, nil
}

// RemoveWorktree removes a git worktree.
func (wm *WorktreeManager) RemoveWorktree(repoPath, taskID string) error {
	wtPath := filepath.Join(wm.WorkDir, fmt.Sprintf("wt-%s", taskID))
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoPath
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("remove worktree: %w\n%s", err, string(out))
	}

	// Clean up any remaining directory
	os.RemoveAll(wtPath)
	return nil
}

// CleanupAll removes all selo worktrees.
func (wm *WorktreeManager) CleanupAll(repoPath string) error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	cmd.Stderr = os.Stderr
	cmd.Run()

	entries, err := os.ReadDir(wm.WorkDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			os.RemoveAll(filepath.Join(wm.WorkDir, e.Name()))
		}
	}
	return nil
}
