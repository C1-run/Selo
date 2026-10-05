// Package containment provides a unified interface for isolated task execution.
//
// Two strategies:
//   - WorktreeContainment: git worktree-based isolation (lightweight, host-based)
//   - DockerContainment: container-based isolation (strong, network-isolated)
//
// Both implement the Containment interface, allowing callers to swap strategies
// without changing the task execution logic.
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
	// Setup creates the isolated environment (worktree or container).
	Setup(repoPath, taskID string) (string, error)
	// Run executes a command inside the isolated environment.
	Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
	// Teardown removes the isolated environment.
	Teardown(repoPath, taskID string) error
	// Workdir returns the path to the isolated workspace.
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
	StrategyDocker   Strategy = "docker"
	StrategyLocal    Strategy = "local"
)

// Config configures the containment.
type Config struct {
	Strategy Strategy
	WorkDir  string   // base directory for worktrees or container mounts
	Image    string   // Docker image (only for StrategyDocker)
	Writable []string // writable paths (only for StrategyDocker)
	Extra    []string // extra mounts (only for StrategyDocker)
}

// New creates a Containment based on the strategy.
func New(cfg Config) (Containment, error) {
	switch cfg.Strategy {
	case StrategyWorktree:
		return &WorktreeContainment{workDir: cfg.WorkDir}, nil
	case StrategyDocker:
		if !DockerAvailable() {
			return nil, fmt.Errorf("docker not available")
		}
		return &DockerContainment{
			workDir:  cfg.WorkDir,
			image:    cfg.Image,
			writable: cfg.Writable,
			extra:    cfg.Extra,
		}, nil
	case StrategyLocal:
		return &LocalContainment{workDir: cfg.WorkDir}, nil
	default:
		return nil, fmt.Errorf("unknown containment strategy: %s", cfg.Strategy)
	}
}

// DockerAvailable reports whether a docker daemon is reachable.
func DockerAvailable() bool {
	err := exec.Command("docker", "info").Run()
	return err == nil
}

// --- WorktreeContainment ---

type WorktreeContainment struct {
	workDir string
	workdir string // set after Setup
}

func (c *WorktreeContainment) Setup(repoPath, taskID string) (string, error) {
	wtPath := filepath.Join(c.workDir, fmt.Sprintf("wt-%s", taskID))
	os.MkdirAll(c.workDir, 0755)

	if absPath, err := filepath.Abs(wtPath); err == nil {
		wtPath = absPath
	}

	// Remove existing worktree if present
	if _, err := os.Stat(wtPath); err == nil {
		exec.Command("git", "worktree", "remove", "--force", wtPath).Run()
		os.RemoveAll(wtPath)
	}

	branchName := fmt.Sprintf("selo/%s", taskID)
	exec.Command("git", "branch", "-D", branchName).Run()

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

// --- DockerContainment ---

type DockerContainment struct {
	workDir   string
	image     string
	writable  []string
	extra     []string
	workdir   string
	container string
	volumes   []string
}

func (c *DockerContainment) Setup(repoPath, taskID string) (string, error) {
	c.workdir = c.workDir
	return c.workDir, nil
}

func (c *DockerContainment) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	args := []string{"run", "--rm", "--name", "c1_cell_" + fmt.Sprint(os.Getpid()),
		"--network", "none",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=64m",
		"-v", c.workdir + ":/work:ro",
	}
	for i, w := range c.writable {
		vol := fmt.Sprintf("c1_w_%d_%d", os.Getpid(), i)
		args = append(args, "-v", vol+":/work/"+strings.TrimPrefix(w, c.workdir+"/")+":rw")
		c.volumes = append(c.volumes, vol)
	}
	for _, m := range c.extra {
		args = append(args, "-v", m)
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, c.image)
	args = append(args, cmdArgs...)

	cmd := exec.Command("docker", args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: -1}, fmt.Errorf("docker start: %w", err)
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
	for _, vol := range c.volumes {
		exec.Command("docker", "volume", "rm", "-f", vol).Run()
	}
	return &r, nil
}

func (c *DockerContainment) Teardown(repoPath, taskID string) error {
	return nil // docker run --rm cleans up
}

func (c *DockerContainment) Workdir() string { return c.workdir }

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
