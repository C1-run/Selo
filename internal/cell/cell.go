// Package cell creates an isolated execution environment for the agent.
//
// Two backends:
//   - LocalCell: process-level isolation on the host (working dir + capability
//     interface), used for local development and tests.
//   - DockerCell: full container isolation for the VPS (network=none,
//     no privileged, no docker.sock, read-only workspace, writable only the
//     granted subpaths).
package cell

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	ExitCode int
	TimedOut bool
	Output   string
}

// Cell is the interface the executor needs: run the agent command inside an
// isolated environment, then tear it down.
type Cell interface {
	// Run executes the agent command and returns its result.
	Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
	// Kill forces termination and cleanup.
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
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
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
	return &r, nil
}

func (c *LocalCell) Kill() error {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return nil
}

// DockerCell runs the agent in an ephemeral container. Requires docker.
type DockerCell struct {
	workdir   string // host path mounted RO as /work (workspace base)
	writable  []string
	extra     []string // additional "host:guest:mode" mounts
	image     string
	container string
	volumes   []string // named volumes to clean up after the run
}

func NewDocker(workdir string, writable []string, image string, extra ...string) *DockerCell {
	return &DockerCell{workdir: workdir, writable: writable, extra: extra, image: image}
}

func (c *DockerCell) Workdir() string { return c.workdir }

func (c *DockerCell) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	args := []string{"run", "--rm", "--name", "c1_cell_" + fmt.Sprint(os.Getpid()),
		"--network", "none",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=64m",
		"-v", c.workdir + ":/work:ro",
	}
	// Writable scope uses named volumes, never bind mounts: a bind mount
	// shares the host filesystem with the RO workspace, so an agent could
	// hardlink a file out of the RO tree into a writable dir and then modify
	// it (hardlink == same inode). Named volumes live on a different
	// filesystem; hardlinking across them fails with EXDEV.
	// ponytail: agent output inside writable volumes is not mirrored back to
	// the host tree; the pipeline reads results from the runs dir, not the
	// workspace. Add a docker cp back if that ever changes.
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
	c.container = fmt.Sprint(os.Getpid()) // placeholder; not used for kill via name

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

func (c *DockerCell) Kill() error {
	// docker run --rm already cleans up on process kill; nothing else needed.
	return nil
}

// DockerAvailable reports whether a docker daemon is reachable.
func DockerAvailable() bool {
	err := exec.Command("docker", "info").Run()
	return err == nil
}
