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

func (c *LocalCell) Run(cmdArgs []string, env []string, timeout time.Duration) (*Result, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = c.workdir
	cmd.Env = append(os.Environ(), env...)
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
	workdir  string // host path mounted RO as /work (workspace base)
	writable []string
	extra    []string // additional "host:guest:mode" mounts
	image    string
	container string
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
	for _, w := range c.writable {
		args = append(args, "-v", w+":/work/"+strings.TrimPrefix(w, c.workdir+"/")+":rw")
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