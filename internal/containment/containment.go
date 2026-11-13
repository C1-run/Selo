// Package containment provides isolated task execution via git worktree.
//
// WorktreeContainment gives each task its own git worktree — the agent
// never touches the main checkout. Docker and local strategies are
// intentionally refused at config validation.
package containment

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Containment is the unified interface for isolated task execution.
type Containment interface {
	Setup(repoPath, taskID string) (string, error)
	Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
	Teardown(repoPath, taskID string) error
	Workdir() string
}

// Result captures the outcome of a command execution.
type Result struct {
	ExitCode int
	TimedOut bool
	Output   string
}

// Strategy identifies the containment strategy.
type Strategy string

const (
	StrategyWorktree Strategy = "worktree"
)

// Config configures the containment.
type Config struct {
	Strategy Strategy
	WorkDir  string
}

// New creates a Containment based on the strategy.
// Only StrategyWorktree is accepted; all others are refused.
func New(cfg Config) (Containment, error) {
	switch cfg.Strategy {
	case StrategyWorktree:
		return &WorktreeContainment{workDir: cfg.WorkDir}, nil
	default:
		return nil, fmt.Errorf("unknown or unsupported containment strategy: %s", cfg.Strategy)
	}
}

// --- WorktreeContainment ---

type WorktreeContainment struct {
	workDir string
	workdir string
}

func (c *WorktreeContainment) Setup(repoPath, taskID string) (string, error) {
	wtPath := filepath.Join(c.workDir, fmt.Sprintf("wt-%s", taskID))
	os.MkdirAll(c.workDir, 0755)

	if absPath, err := filepath.Abs(wtPath); err == nil {
		wtPath = absPath
	}

	if _, err := os.Stat(wtPath); err == nil {
		rmCmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rmCmd.Dir = repoPath
		rmCmd.Run()
		os.RemoveAll(wtPath)
	}

	branchName := fmt.Sprintf("selo/%s", taskID)

	// A branch checked out in a stale worktree cannot be deleted, which would
	// make `worktree add -b` fail on a rerun. --expire now is required because
	// the default prune expiry is mtime-based over 3 months.
	pruneCmd := exec.Command("git", "worktree", "prune", "--expire", "now")
	pruneCmd.Dir = repoPath
	pruneCmd.Run()

	delCmd := exec.Command("git", "branch", "-D", branchName)
	delCmd.Dir = repoPath
	delCmd.Run()

	cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtPath, "HEAD")
	cmd.Dir = repoPath
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		return "", fmt.Errorf("create worktree: %w\n%s", err, string(out))
	}

	c.workdir = wtPath
	return wtPath, nil
}

func (c *WorktreeContainment) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = c.workdir
	cmd.Env = append(sanitizedEnv(), env...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: -1}, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var r Result
	select {
	case err := <-done:
		if err == nil {
			r.ExitCode = 0
		} else if ee, ok := err.(*exec.ExitError); ok {
			r.ExitCode = ee.ExitCode()
		} else {
			return nil, err
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		r.TimedOut = true
		r.ExitCode = -1
	}
	r.Output = out.String()
	return &r, nil
}

func (c *WorktreeContainment) Teardown(repoPath, taskID string) error {
	wtPath := filepath.Join(c.workDir, fmt.Sprintf("wt-%s", taskID))
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoPath
	cmd.Stderr = os.Stderr
	cmd.Output()

	os.RemoveAll(wtPath)
	return nil
}

func (c *WorktreeContainment) Workdir() string { return c.workdir }

// --- LocalContainment ---

type LocalContainment struct {
	workDir string
	workdir string
}

var envAllowlist = map[string]bool{
	"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true,
	"LANG": true, "LC_ALL": true, "SHELL": true, "TERM": true,
}

func sanitizedEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && envAllowlist[k] {
			out = append(out, kv)
		}
	}
	return out
}

func (c *LocalContainment) Setup(repoPath, taskID string) (string, error) {
	c.workdir = c.workDir
	os.MkdirAll(c.workDir, 0755)
	return c.workDir, nil
}

func (c *LocalContainment) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = c.workdir
	cmd.Env = append(sanitizedEnv(), env...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: -1}, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var r Result
	select {
	case err := <-done:
		if err == nil {
			r.ExitCode = 0
		} else if ee, ok := err.(*exec.ExitError); ok {
			r.ExitCode = ee.ExitCode()
		} else {
			return nil, err
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		r.TimedOut = true
		r.ExitCode = -1
	}
	r.Output = out.String()
	return &r, nil
}

func (c *LocalContainment) Teardown(repoPath, taskID string) error {
	return nil
}

func (c *LocalContainment) Workdir() string { return c.workdir }
