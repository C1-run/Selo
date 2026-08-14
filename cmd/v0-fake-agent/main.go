// v0-fake-agent is a deliberately simple agent that asks the C1 capability
// broker before every action. It cannot act without a granted capability —
// exactly what a real agent must be reduced to.
//
// Mode "clean":   writes an allowed file, exits 0.
// Mode "escape":  tries to write a forbidden file -> DENY -> exits 2.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type req struct {
	RunID  string   `json:"run_id"`
	Kind   string   `json:"kind"`
	Target string   `json:"target"`
	Args   []string `json:"args"`
}

type decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Policy  string `json:"policy"`
}

func ask(sock, runID, kind, target string, args ...string) (decision, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return decision{}, fmt.Errorf("dial broker: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := req{RunID: runID, Kind: kind, Target: target, Args: args}
	if err := json.NewEncoder(conn).Encode(r); err != nil {
		return decision{}, err
	}
	var d decision
	if err := json.NewDecoder(conn).Decode(&d); err != nil {
		return decision{}, err
	}
	return d, nil
}

func main() {
	sock := flag.String("sock", "", "broker unix socket path")
	runID := flag.String("run-id", "", "run id")
	workdir := flag.String("workdir", "", "workspace root")
	mode := flag.String("mode", "clean", "clean | escape | bypass | panic | sleep")
	sleepSec := flag.Int("sleep-sec", 120, "how long sleep mode hangs (must exceed the run timeout)")
	flag.Parse()

	if *sock == "" || *runID == "" || *workdir == "" {
		fmt.Fprintln(os.Stderr, "usage: v0-fake-agent --sock <path> --run-id <id> --workdir <dir> --mode clean|escape")
		os.Exit(1)
	}

	switch *mode {
	case "clean":
		// brief liveness window so the soak harness can SIGKILL us mid-run;
		// a real agent lives much longer than 200ms anyway.
		time.Sleep(200 * time.Millisecond)
		target := filepath.Join(*workdir, "src", "payments", "fixed.ts")
		d, err := ask(*sock, *runID, "filesystem.write", "src/payments/fixed.ts")
		if err != nil {
			fmt.Fprintf(os.Stderr, "broker error: %v\n", err)
			os.Exit(3)
		}
		if !d.Allowed {
			fmt.Fprintf(os.Stderr, "DENIED: %s (%s)\n", d.Reason, d.Policy)
			os.Exit(2)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
			os.Exit(3)
		}
		if err := os.WriteFile(target, []byte("// fixed by agent\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			os.Exit(3)
		}
		fmt.Printf("ALLOWED: wrote %s\n", target)
	case "escape":
		time.Sleep(200 * time.Millisecond)
		d, err := ask(*sock, *runID, "filesystem.write", "src/auth/middleware.ts")
		if err != nil {
			fmt.Fprintf(os.Stderr, "broker error: %v\n", err)
			os.Exit(3)
		}
		if !d.Allowed {
			fmt.Fprintf(os.Stderr, "DENIED: %s (%s) — agent stops\n", d.Reason, d.Policy)
			os.Exit(2)
		}
		// In a real agent this write would never happen; broker granted it
		// even though the manifest forbids it — C1 failed.
		fmt.Fprintln(os.Stderr, "UNEXPECTED: broker allowed out-of-scope write — C1 is broken")
		os.Exit(4)
	case "bypass":
		// Adversarial mode: try to reach forbidden capabilities WITHOUT
		// asking the broker. Any attempt that succeeds means the cell
		// boundary leaked and C1 is not an execution boundary.
		failures := bypassAttempts(*workdir)
		if len(failures) > 0 {
			fmt.Fprintf(os.Stderr, "BYPASS LEAKED: %v — C1 boundary broken\n", failures)
			os.Exit(5)
		}
		fmt.Println("BYPASS IMPOSSIBLE: all direct attempts blocked by cell boundary")
	case "panic":
		// Simulates a crashed agent: killed by the OS with SIGKILL (137).
		time.Sleep(200 * time.Millisecond)
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
	case "sleep":
		// Simulates a hung agent: must be stopped by the cell timeout.
		time.Sleep(time.Duration(*sleepSec) * time.Second)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(1)
	}
}

// bypassAttempts tries each capability directly, without the broker.
// Returns the list of attempts that SUCCEEDED — a non-empty result is a
// boundary leak. Every attempt must fail for C1 to hold.
func bypassAttempts(workdir string) []string {
	var leaked []string
	try := func(label string, f func() error) {
		if err := f(); err != nil {
			fmt.Printf("BLOCKED  %s: %v\n", label, err)
		} else {
			fmt.Printf("LEAKED   %s\n", label)
			leaked = append(leaked, label)
		}
	}

	outside := filepath.Join(workdir, "src", "auth", "middleware.ts")
	inside := filepath.Join(workdir, "src", "payments", "fixed.ts")

	// 1. filesystem: direct write outside the granted scope.
	try("direct write outside scope", func() error {
		return os.WriteFile(outside, []byte("x"), 0644)
	})
	// sanity: the granted scope must be writable — if even that fails, the
	// boundary is misconfigured and C1 cannot run a real agent.
	if err := os.WriteFile(inside, []byte("x"), 0644); err != nil {
		leaked = append(leaked, "sanity: granted scope not writable: "+err.Error())
		fmt.Printf("MISCONFIG granted scope unwritable: %v\n", err)
	} else {
		fmt.Println("OK      direct write inside scope")
	}
	try("write to /etc (rootfs must be RO)", func() error {
		return os.WriteFile("/etc/c1-bypass", []byte("x"), 0644)
	})
	try("write to /var (rootfs must be RO)", func() error {
		return os.WriteFile("/var/c1-bypass", []byte("x"), 0644)
	})

	// 2. exec: a subprocess shell must inherit the same boundary.
	try("shell subprocess write outside scope", func() error {
		return exec.Command("sh", "-c", "echo x > "+outside).Run()
	})

	// 3. git: config / hooks write outside work; push needs network.
	try("git config write outside scope", func() error {
		return exec.Command("git", "-C", workdir, "config", "core.hooksPath", "/etc").Run()
	})
	try("git push (network must be off)", func() error {
		return exec.Command("git", "-C", workdir, "push", "origin", "HEAD").Run()
	})

	// 4. network: any external dial must fail.
	try("network dial 8.8.8.8:53", func() error {
		conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	})
	try("network dial example.com:443", func() error {
		conn, err := net.DialTimeout("tcp", "example.com:443", 3*time.Second)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	})

	// 5. capabilities: setuid and privilege escalation must fail.
	try("setuid escalate", func() error {
		return exec.Command("sh", "-c", "chmod 4755 /bin/sh 2>/dev/null; [ -u /bin/sh ]").Run()
	})
	try("privilege escalate via rootfs", func() error {
		return exec.Command("sh", "-c", "echo 0 > /proc/sys/kernel/hostname 2>/dev/null").Run()
	})

	return leaked
}