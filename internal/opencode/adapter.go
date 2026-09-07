package opencode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Adapter wraps the OpenCode binary for C1 Forge task execution.
// It replaces scripts/opencode-adapter.sh with a native Go implementation.
type Adapter struct {
	opts AdapterOpts
}

// NewAdapter creates a new OpenCode adapter.
func NewAdapter(opts AdapterOpts) *Adapter {
	if opts.ServeTimeoutSec <= 0 {
		opts.ServeTimeoutSec = 30
	}
	if opts.RunTimeoutSec <= 0 {
		opts.RunTimeoutSec = opts.MaxMinutes * 60
	}
	if opts.RunTimeoutSec <= 0 {
		opts.RunTimeoutSec = 300
	}
	return &Adapter{opts: opts}
}

// Run executes a one-shot task through OpenCode.
// It starts `opencode serve`, sends the task via REST API, waits for completion,
// and returns the result.
func (a *Adapter) Run(ctx context.Context, taskGoal string) *RunResult {
	startTime := time.Now()
	result := &RunResult{}

	// Resolve binary
	binPath := a.opts.BinaryPath
	if binPath == "" {
		binPath = resolveOpenCodeBinary()
	}
	if binPath == "" {
		result.ExitCode = 99
		result.Stderr = "no opencode binary found"
		return result
	}

	// Start serve
	serveCtx, serveCancel := context.WithTimeout(ctx, time.Duration(a.opts.ServeTimeoutSec)*time.Second)
	defer serveCancel()

	server, err := Start(serveCtx, ServerOpts{
		BinaryPath:  binPath,
		Port:        0,
		WorkDir:     a.opts.WorkDir,
		Model:       a.opts.Model,
		Agent:       a.opts.Agent,
		Permissions: a.opts.Permissions,
		Allowlist:   a.opts.Allowlist,
	})
	if err != nil {
		result.ExitCode = 99
		result.Stderr = fmt.Sprintf("serve start failed: %v", err)
		return result
	}
	defer server.Stop()

	// Create client
	client := NewClient(server.URL())
	defer client.Close()

	// Create session
	session, err := client.CreateSession(ctx)
	if err != nil {
		result.ExitCode = 99
		result.Stderr = fmt.Sprintf("create session failed: %v", err)
		return result
	}
	result.SessionID = session.ID

	// Send prompt with timeout
	runCtx, runCancel := context.WithTimeout(ctx, time.Duration(a.opts.RunTimeoutSec)*time.Second)
	defer runCancel()

	prompt := taskGoal
	if a.opts.TaskFilePath != "" {
		data, err := os.ReadFile(a.opts.TaskFilePath)
		if err == nil {
			prompt = string(data)
		}
	}

	resp, err := client.Prompt(runCtx, session.ID, prompt)
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = 124
			_ = client.Abort(ctx, session.ID)
		} else {
			result.ExitCode = 99
			result.Stderr = fmt.Sprintf("prompt failed: %v", err)
		}
		return result
	}

	// Collect output
	var stdout strings.Builder
	for _, msg := range resp.Messages {
		if msg.Role == "assistant" {
			stdout.WriteString(msg.Content)
			stdout.WriteString("\n")
		}
	}
	result.Stdout = stdout.String()
	result.ExitCode = 0
	result.DurationMs = time.Since(startTime).Milliseconds()

	return result
}

// RunDirect executes OpenCode directly (without serve) as a subprocess.
// This is a fallback if the REST API approach fails.
func (a *Adapter) RunDirect(ctx context.Context, taskGoal string) *RunResult {
	startTime := time.Now()
	result := &RunResult{}

	binPath := a.opts.BinaryPath
	if binPath == "" {
		binPath = resolveOpenCodeBinary()
	}
	if binPath == "" {
		result.ExitCode = 99
		result.Stderr = "no opencode binary found"
		return result
	}

	args := []string{}
	if a.opts.Model != "" {
		args = append(args, "--model", a.opts.Model)
	}
	if a.opts.Agent != "" {
		args = append(args, "--agent", a.opts.Agent)
	}
	if a.opts.Permissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = a.opts.WorkDir
	cmd.Stdin = strings.NewReader(taskGoal)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 99
		}
	} else {
		result.ExitCode = 0
	}

	return result
}

// resolveOpenCodeBinary finds the opencode binary.
func resolveOpenCodeBinary() string {
	// Check env
	if env := os.Getenv("SELO_OPENCODE_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	// Check PATH
	if path, err := exec.LookPath("opencode"); err == nil {
		return path
	}
	return ""
}

// ValidateFilePath checks if a file path matches the allowlist patterns.
// Returns nil if allowlist is empty (permissive mode).
// Uses filepath.Match for glob pattern support (e.g., "*.go", "src/**").
func ValidateFilePath(path string, allowlist []string) error {
	if len(allowlist) == 0 {
		return nil // empty allowlist = permissive
	}

	// Normalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	for _, pattern := range allowlist {
		// Try exact match
		if absPath == pattern {
			return nil
		}
		// Try glob match
		matched, err := filepath.Match(pattern, absPath)
		if err != nil {
			continue // invalid pattern, skip
		}
		if matched {
			return nil
		}
		// Try prefix match for directory patterns
		if strings.HasSuffix(pattern, "/") || strings.HasSuffix(pattern, "/*") {
			dir := strings.TrimSuffix(strings.TrimSuffix(pattern, "*"), "/")
			if strings.HasPrefix(absPath, dir) {
				return nil
			}
		}
	}

	return fmt.Errorf("path %q not in permission allowlist", absPath)
}
