package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server manages an OpenCode serve process lifecycle.
type Server struct {
	cmd     *exec.Cmd
	url     string
	port    int
	process *os.Process
	mu      sync.Mutex
}

var portRe = regexp.MustCompile(`(?i)listening on.*?:(\d+)|addr.*?:(\d+)|:(\d+)/|port\s*[:=]\s*(\d+)`)

// Start launches `opencode serve` and waits for it to bind.
func Start(ctx context.Context, opts ServerOpts) (*Server, error) {
	args := []string{"serve"}
	if opts.Port > 0 {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Agent != "" {
		args = append(args, "--agent", opts.Agent)
	}
	if opts.Permissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	cmd := exec.CommandContext(ctx, opts.BinaryPath, args...)
	cmd.Dir = opts.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start serve: %w", err)
	}

	s := &Server{
		cmd:     cmd,
		port:    opts.Port,
		process: cmd.Process,
	}

	// Wait for the port to appear in output
	if err := s.waitForReady(stdout, 30*time.Second); err != nil {
		s.Stop()
		return nil, fmt.Errorf("wait for ready: %w", err)
	}

	return s, nil
}

// URL returns the server's base URL.
func (s *Server) URL() string {
	return s.url
}

// Port returns the server's port.
func (s *Server) Port() int {
	return s.port
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.process == nil {
		return nil
	}

	// Send SIGTERM
	s.process.Signal(os.Interrupt)

	// Wait up to 5s
	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Force kill
		if s.process != nil {
			s.process.Kill()
		}
		<-done
	}

	s.process = nil
	return nil
}

// Wait waits for the server process to exit.
func (s *Server) Wait() error {
	return s.cmd.Wait()
}

// waitForReady reads stdout until the port URL appears or timeout.
func (s *Server) waitForReady(r io.Reader, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()

		// Look for port in output
		if matches := portRe.FindStringSubmatch(line); matches != nil {
			for _, m := range matches[1:] {
				if m != "" {
					port, err := strconv.Atoi(m)
					if err == nil && port > 0 {
						s.port = port
						s.url = fmt.Sprintf("http://localhost:%d", port)
						return nil
					}
				}
			}
		}

		// Also check for common ready indicators
		if strings.Contains(line, "ready") || strings.Contains(line, "started") {
			if s.port > 0 {
				s.url = fmt.Sprintf("http://localhost:%d", s.port)
				return nil
			}
		}
	}

	return fmt.Errorf("timeout waiting for serve to start")
}

// WriteRunInfo writes the opencode-run-info.json file.
func WriteRunInfo(workDir string, info RunInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, "opencode-run-info.json"), data, 0644)
}

// keep unused import happy
var _ = time.Second
