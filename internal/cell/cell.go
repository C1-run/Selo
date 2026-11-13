// Package cell creates an isolated execution environment for the agent.
//
// LocalCell provides process-level isolation on the host (working dir +
// capability interface), used for local development and tests.
package cell

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	ExitCode int
	TimedOut bool
	Output   string
	Stderr   string
}

// Cell is the interface the executor needs: run the agent command inside an
// isolated environment, then tear it down.
type Cell interface {
	Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
	Kill() error
}

// LocalCell runs the command in a sandbox directory on the host.
type LocalCell struct {
	workdir string
	cmd     *exec.Cmd
}

func NewLocal(workdir string) *LocalCell {
	os.MkdirAll(workdir, 0755)
	return &LocalCell{workdir: workdir}
}

func (c *LocalCell) Workdir() string { return c.workdir }

// envAllowlist is the only host environment inherited by a LocalCell agent.
// Everything else on the host (API keys, tokens, database URLs) stays out.
// The caller's explicit env (C1_* etc.) is appended after.
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

func (c *LocalCell) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = c.workdir
	cmd.Env = append(sanitizedEnv(), env...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	c.cmd = cmd

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
	r.Stderr = errb.String()
	return &r, nil
}

func (c *LocalCell) Kill() error {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return nil
}
