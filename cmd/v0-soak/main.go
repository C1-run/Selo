// v0-soak is the C1 Forge V0 soak harness: a chaotic orchestrator that runs
// the c1-vps pipeline repeatedly, hunting for state corruption, orphan
// processes, broker socket deadlocks and evidence ledger corruption.
//
// THIS IS A STABILITY/LIFECYCLE TEST, NOT A SECURITY TEST. The local backend
// has no kernel-level containment; see SOAK_REPORT.md for the full warning.
//
// Usage:
//
//	v0-soak -runs 200            # fixed number of runs (default 200)
//	v0-soak -minutes 480         # run until duration elapses
//	v0-soak -out-dir soak-out    # report + artifacts dir (default soak-out)
//
// Exit code 0 = PASS, 1 = FAIL (report written either way).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anomalyco/c1-forge/internal/evidence"
	"github.com/anomalyco/c1-forge/internal/manifest"
)

// persona drives the fake agent mode and the expected verdict.
type persona struct {
	name    string
	mode    string
	weight  int
	want    string // verdict
	timeout int    // manifest max minutes
}

var personas = []persona{
	{name: "GoodCitizen", mode: "clean", weight: 60, want: "pass", timeout: 1},
	{name: "ScopeEscape", mode: "escape", weight: 15, want: "stop", timeout: 1},
	{name: "Panicker", mode: "panic", weight: 10, want: "stop", timeout: 1},
	{name: "Sleeper", mode: "sleep", weight: 15, want: "stop", timeout: 1},
	// Bypasser is intentionally absent: the local backend has no containment,
	// so bypass probing belongs to the VPS-only Gauntlet, not this soak.
}

type runResult struct {
	seq      int
	persona  string
	killed   bool
	got      string
	want     string
	ok       bool
	chainOK  bool
	note     string
	duration time.Duration
}

func main() {
	runs := flag.Int("runs", 200, "number of runs (0 = use -minutes)")
	minutes := flag.Int("minutes", 0, "duration limit in minutes (overrides run count when >0)")
	outDir := flag.String("out-dir", "soak-out", "report/artifact output dir")
	keep := flag.Bool("keep", false, "keep per-run ledger dirs (default: delete)")
	verbose := flag.Bool("verbose", false, "verbose logging")
	flag.Parse()

	os.MkdirAll(*outDir, 0755)

	vpsBin := filepath.Join(*outDir, "c1-vps")
	agentBin := filepath.Join(*outDir, "v0-fake-agent")
	buildBinary("github.com/anomalyco/c1-forge/cmd/c1-vps", vpsBin)
	buildBinary("github.com/anomalyco/c1-forge/cmd/v0-fake-agent", agentBin)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var results []runResult
	startAll := time.Now()
	deadline := time.Now().Add(time.Duration(*minutes) * time.Minute)
	chaosKills := 0
	recovered := 0

	seq := 0
	for {
		if *minutes > 0 && time.Now().After(deadline) {
			break
		}
		if *minutes == 0 && seq >= *runs {
			break
		}
		seq++

		p := pickPersona(rng)
		chaos := rng.Intn(100) < 10
		rr := runResult{seq: seq, persona: p.name, killed: chaos, want: p.want}

		base := filepath.Join(*outDir, fmt.Sprintf("run-%05d", seq))
		repoDir := filepath.Join(base, "repo")
		os.MkdirAll(filepath.Join(repoDir, "src", "payments"), 0755)
		os.MkdirAll(filepath.Join(repoDir, "src", "auth"), 0755)
		manifestPath := filepath.Join(base, "manifest.json")
		runID := fmt.Sprintf("run_%05d", seq)
		writeManifest(manifestPath, manifestFor(repoDir, runID, p))
		runsDir := filepath.Join(base, "runs")

		t0 := time.Now()
		cmd := exec.Command(vpsBin, "run",
			"-manifest", manifestPath,
			"-run-id", runID,
			"-runs-dir", runsDir,
			"-mode", p.mode,
			"-agent-bin", agentBin,
			"-timeout-min", fmt.Sprint(p.timeout),
		)
		// give the run dirs a conforming layout: c1-vps derives them from
		// runs-dir + run-id, so nothing else to prepare.

		var err error
		if err = cmd.Start(); err != nil {
			fmt.Printf("RUN %d FAIL (%s): start: %v\n", seq, p.name, err)
			os.Exit(1)
		}

				// chaos: SIGKILL this run's agent (matched by run-id in its argv) as
		// soon as it starts. The agent typically lives only ~15ms, so fixed
		// sleeps miss it; poll at 1ms until the process appears.
		rr.killed = false
		if chaos {
			if *verbose {
				fmt.Printf("chaos: run %d (%s) arming SIGKILL\n", seq, p.name)
			}
			chaosKillDone := make(chan bool, 1)
			chaosCancel := make(chan struct{})
			chaosTarget := "v0-fake-agent.*" + runID
			go func() {
				defer func() { chaosKillDone <- false }()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					select {
					case <-chaosCancel:
						return
					default:
					}
				if pid := agentPID(chaosTarget); pid > 0 {
					syscall.Kill(pid, syscall.SIGKILL)
					if *verbose {
						fmt.Printf("chaos: SIGKILL %d (%s)\n", pid, chaosTarget)
					}
					chaosKillDone <- true
					return
				}
					time.Sleep(time.Millisecond)
				}
			}()
			err = cmd.Wait()
			close(chaosCancel)
			rr.killed = <-chaosKillDone
			if rr.killed {
				chaosKills++
			}
		} else {
			err = cmd.Wait()
		}
		rr.duration = time.Since(t0)

		if err != nil {
			rr.note = "pipeline error: " + err.Error()
			rr.ok = false
			results = append(results, rr)
			writeReport(*outDir, results, chaosKills, recovered, startAll)
			os.Exit(1)
		}

		ledgerDir := filepath.Join(runsDir, runID)
		rec, rerr := readReceipt(ledgerDir)
		if rerr != nil {
			rr.note = "receipt missing: " + rerr.Error()
			rr.ok = false
			results = append(results, rr)
			writeReport(*outDir, results, chaosKills, recovered, startAll)
			os.Exit(1)
		}
		rr.got = rec["verdict"].(string)

		// 1) hash chain revalidation
		chainErr := evidence.VerifyChain(filepath.Join(ledgerDir, "events.jsonl"))
		rr.chainOK = chainErr == nil
		if !rr.chainOK {
			rr.note = "hash chain broken: " + chainErr.Error()
		}

		// 2) verdict matrix assertion. A chaos-killed agent must STOP
		// (fail-closed), regardless of persona.
		if rr.killed {
			rr.want = "stop"
		}
		rr.ok = rr.got == rr.want
		if !rr.ok && rr.note == "" {
			rr.note = fmt.Sprintf("want=%s got=%s", rr.want, rr.got)
		}

		// 3) STOP runs of ScopeEscape must carry CAPABILITY_DENIED — unless the
		// agent was chaos-killed before it ever asked the broker.
		if p.mode == "escape" && rr.got == "stop" && !rr.killed {
			data, _ := os.ReadFile(filepath.Join(ledgerDir, "events.jsonl"))
			if !strings.Contains(string(data), "CAPABILITY_DENIED") {
				rr.ok = false
				rr.note = "STOP without CAPABILITY_DENIED in ledger"
			}
		}

		// 4) Sleeper must be timed out — unless chaos-killed first (then it
		// stops via SIGKILL, not timeout).
		if p.mode == "sleep" && !rr.killed && rec["timed_out"] != true {
			rr.ok = false
			if rr.note == "" {
				rr.note = "sleep run not marked timed_out"
			}
		}

		// 5) chaos-killed runs must never PASS (fail-closed)
		if rr.killed && rr.got == "pass" {
			rr.ok = false
			rr.note = "FALSE PASS after SIGKILL — fail-closed violated"
		}
		if rr.killed && rr.got == "stop" {
			recovered++
		}

		// 6) orphan check: no fake agent may survive a completed run. The check
		// runs twice with a settle delay: macOS can briefly show a reaped
		// child in the process table before the snapshot catches up.
		time.Sleep(250 * time.Millisecond)
		orphan1 := agentPID("v0-fake-agent")
		time.Sleep(250 * time.Millisecond)
		orphan2 := agentPID("v0-fake-agent")
		if orphan1 > 0 || orphan2 > 0 {
			rr.ok = false
			if rr.note == "" {
				rr.note = fmt.Sprintf("orphan agent process detected (pid %d/%d)", orphan1, orphan2)
			}
		}

		results = append(results, rr)

		if !rr.ok {
			fmt.Printf("RUN %d FAIL (%s): %s\n", seq, p.name, rr.note)
			writeReport(*outDir, results, chaosKills, recovered, startAll)
			os.Exit(1)
		}

		if !*keep {
			os.RemoveAll(base)
		}

		if seq%25 == 0 {
			fmt.Printf("progress: %d runs\n", seq)
		}
	}

	ok := summarize(results)
	writeReport(*outDir, results, chaosKills, recovered, startAll)
	if !ok {
		os.Exit(1)
	}
	fmt.Printf("SOAK PASS: %d runs in %s\n", len(results), time.Since(startAll).Round(time.Second))
}

func pickPersona(rng *rand.Rand) persona {
	total := 0
	for _, p := range personas {
		total += p.weight
	}
	n := rng.Intn(total)
	for _, p := range personas {
		if n < p.weight {
			return p
		}
		n -= p.weight
	}
	return personas[0]
}

func manifestFor(repoDir, runID string, p persona) *manifest.RunManifest {
	return &manifest.RunManifest{
		Version: "0.1",
		RunID:   runID,
		Task:    "soak task " + p.name,
		Repo:    manifest.RepoSpec{Path: repoDir, Base: "HEAD"},
		Agent:   manifest.AgentSpec{Command: []string{"v0-fake-agent"}, Image: "alpine:3.19"},
		Capability: manifest.CapabilitySet{
			Filesystem: manifest.FilesystemCapability{
				Read:  []string{"src/**", "tests/**"},
				Write: []string{"src/payments/**"},
			},
		},
		Network: manifest.NetworkPolicy{Connection: manifest.NetworkCapability{Mode: "none"}},
		Limits:  manifest.ResourceLimits{MaxMinutes: p.timeout},
	}
}

func writeManifest(path string, m *manifest.RunManifest) {
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(path, data, 0644)
}

func readReceipt(dir string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func buildBinary(pkg, out string) {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = repoRoot()
	if o, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build %s: %v\n%s", pkg, err, o)
		os.Exit(1)
	}
}

// repoRoot walks up from the working directory until it finds go.mod.
func repoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// agentPID returns the PID of a live (non-zombie) v0-fake-agent matching the
// pgrep pattern (runs are serial, so at most one matches), or 0.
func agentPID(pattern string) int {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return 0 // pgrep exit 1 = no match
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid := 0
		fmt.Sscanf(line, "%d", &pid)
		if pid <= 0 {
			continue
		}
		// skip zombies: SIGKILL on a zombie is a no-op and would
		// falsely inflate the chaos-kill counter.
		stat, err := exec.Command("ps", "-o", "stat=", "-p", fmt.Sprint(pid)).Output()
		if err != nil {
			continue // race: process already reaped
		}
		if !strings.Contains(string(stat), "Z") {
			return pid
		}
	}
	return 0
}

func summarize(results []runResult) bool {
	ok := true
	fmt.Println("\nverdict matrix:")
	byPersona := map[string][2]int{}
	for _, r := range results {
		bp := byPersona[r.persona]
		bp[0]++
		if r.ok {
			bp[1]++
		} else {
			ok = false
		}
		byPersona[r.persona] = bp
	}
	for _, p := range personas {
		bp := byPersona[p.name]
		fmt.Printf("  %-14s ran=%d matched=%d\n", p.name, bp[0], bp[1])
	}
	for _, r := range results {
		if !r.ok {
			fmt.Printf("  FAIL run %d (%s): %s\n", r.seq, r.persona, r.note)
		}
	}
	return ok
}

func writeReport(outDir string, results []runResult, chaosKills, recovered int, start time.Time) {
	var b strings.Builder
	b.WriteString("# SOAK REPORT — C1 Forge V0 Stability/Lifecycle\n\n")
	b.WriteString("> **LOCAL MAC SOAK TEST ≠ REAL LINUX SECURITY VALIDATION.**\n")
	b.WriteString("> This test exercises the c1-vps pipeline on the local backend (no kernel\n")
	b.WriteString("> containment, no docker). It validates stability and evidence integrity\n")
	b.WriteString("> ONLY. Kernel-level security claims require The Gauntlet on a VPS.\n\n")

	ok := true
	for _, r := range results {
		if !r.ok {
			ok = false
			break
		}
	}
	verdict := "PASS"
	if !ok {
		verdict = "FAIL"
	}
	b.WriteString(fmt.Sprintf("**EXECUTIVE SUMMARY: %s**\n\n", verdict))
	b.WriteString(fmt.Sprintf("Runs: %d | Duration: %s | Chaos SIGKILLs: %d | Fail-closed recoveries: %d\n\n",
		len(results), time.Since(start).Round(time.Second), chaosKills, recovered))

	b.WriteString("## Verdict Matrix\n\n| Persona | Runs | Matched |\n|---|---|---|\n")
	byPersona := map[string][2]int{}
	for _, r := range results {
		bp := byPersona[r.persona]
		bp[0]++
		if r.ok {
			bp[1]++
		}
		byPersona[r.persona] = bp
	}
	for _, p := range personas {
		bp := byPersona[p.name]
		b.WriteString(fmt.Sprintf("| %s | %d | %d |\n", p.name, bp[0], bp[1]))
	}

	b.WriteString("\n## Failures\n\n")
	fails := 0
	for _, r := range results {
		if !r.ok {
			fails++
			b.WriteString(fmt.Sprintf("- run %d (%s): %s\n", r.seq, r.persona, r.note))
		}
	}
	if fails == 0 {
		b.WriteString("- none\n")
	}

	b.WriteString("\n## Orphan Audit\n\n- Agent processes after runs: ")
	if agentPID("v0-fake-agent") > 0 {
		b.WriteString("**ORPHANS FOUND**\n")
	} else {
		b.WriteString("none\n")
	}

	chainFails := 0
	for _, r := range results {
		if !r.chainOK && r.got != "" {
			chainFails++
		}
	}
	b.WriteString("\n## Evidence Integrity\n\n- Hash chain revalidated per run: ")
	if chainFails == 0 {
		b.WriteString("100% valid\n")
	} else {
		b.WriteString(fmt.Sprintf("%d BROKEN\n", chainFails))
	}

	b.WriteString("\n## Not Tested (requires architecture that does not exist yet)\n\n")
	b.WriteString("- Daemon kill -9 + recovery state machine (c1-vps is one-shot, no daemon)\n")
	b.WriteString("- Orphan container audit (no docker on host)\n")
	b.WriteString("- User namespaces, seccomp, cgroup v2 kill (VPS-only)\n")
	b.WriteString("- Goroutine/RSS leak sampling in-process (child-process model)\n")
	b.WriteString("- Host-side git verification RCE surface (Verify phase unimplemented)\n\n")

	b.WriteString("## How to Run\n\n```\ncd cmd/v0-soak && go run . -runs 200\ncd cmd/v0-soak && go run . -minutes 480\n```\n")

	os.WriteFile(filepath.Join(outDir, "SOAK_REPORT.md"), []byte(b.String()), 0644)
	fmt.Printf("\nreport: %s\n", filepath.Join(outDir, "SOAK_REPORT.md"))
}