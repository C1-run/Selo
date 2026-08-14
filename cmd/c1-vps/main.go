// c1-vps is the C1 Forge V0 binary: the capability boundary around an agent.
//
// The V0 loop, in order:
//   Manifest → Validate/Freeze → Create Cell → Start Broker → Start Agent →
//   Capability-controlled execution → BREAK on violation → Verify →
//   GateChain verdict → Receipt.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anomalyco/c1-forge/internal/broker"
	"github.com/anomalyco/c1-forge/internal/cell"
	"github.com/anomalyco/c1-forge/internal/evidence"
	"github.com/anomalyco/c1-forge/internal/gatechain"
	"github.com/anomalyco/c1-forge/internal/manifest"
)

const version = "0.1.0"

type runOptions struct {
	manifestPath string
	runID        string
	runsDir      string
	mode         string
	agentBin     string
	timeoutMin   int
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: c1-vps run -manifest <path> [-run-id <id>] [-runs-dir <dir>] [-mode clean|escape] [-timeout-min <n>]")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var o runOptions
	fs.StringVar(&o.manifestPath, "manifest", "", "path to manifest JSON (required)")
	fs.StringVar(&o.runID, "run-id", "", "run id (default: derived from manifest)")
	fs.StringVar(&o.runsDir, "runs-dir", "runs", "evidence ledger base dir")
	fs.StringVar(&o.mode, "mode", "clean", "fake agent mode: clean | escape")
	fs.StringVar(&o.agentBin, "agent-bin", "", "path to agent binary (default: built v0-fake-agent)")
	fs.IntVar(&o.timeoutMin, "timeout-min", 10, "agent timeout in minutes")
	fs.Parse(os.Args[2:])

	if o.manifestPath == "" {
		fmt.Fprintln(os.Stderr, "error: -manifest is required")
		os.Exit(1)
	}

	if err := run(&o); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
}

func run(o *runOptions) error {
	frozen, err := manifest.Load(o.manifestPath, o.runID)
	if err != nil {
		return err
	}
	m := frozen.Manifest

	// 1. Freeze the manifest into the run dir.
	ledger, err := evidence.NewLedger(o.runsDir, m.RunID)
	if err != nil {
		return err
	}
	defer ledger.Close()
	manifestJSON, _ := json.MarshalIndent(frozen, "", "  ")
	ledger.WriteArtifact("manifest.json", manifestJSON)
	ledger.AppendFlow("MANIFEST_FROZEN", "hash="+frozen.Hash)
	fmt.Printf("manifest frozen: %s hash=%s\n", m.RunID, frozen.Hash)

	// 2. Start the capability broker over a unix socket.
	sock := filepath.Join(os.TempDir(), "c1-"+m.RunID+".sock")
	os.Remove(sock)
	brk := broker.New(m)
	brk.OnDecide = func(req broker.CapabilityRequest, d broker.Decision) {
		ledger.AppendDecision(d.Allowed, req.Kind, req.Target, d.Policy, d.Reason)
	}
	brokerDone := make(chan error, 1)
	go func() { brokerDone <- brk.Serve(sock) }()
	defer os.Remove(sock)
	// wait for the broker socket to accept connections
	served := false
	for i := 0; i < 100; i++ {
		select {
		case err := <-brokerDone:
			return fmt.Errorf("broker failed to start: %w", err)
		default:
		}
		if _, err := os.Stat(sock); err == nil {
			served = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !served {
		return fmt.Errorf("broker did not start")
	}

	// 3. Prepare the cell.
	timeout := time.Duration(o.timeoutMin) * time.Minute
	agentBin := o.agentBin
	if agentBin == "" {
		agentBin = fakeAgentDefault()
	}
	var c cell.Cell
	var workdir string // host-visible path
	var sockInCell = sock
	var agentBinInCell = agentBin
	agentWorkdir := ""
	if cell.DockerAvailable() {
		workdir = m.Repo.Path
		sockDir := filepath.Dir(sock)
		agentDir := filepath.Dir(agentBin)
		c = cell.NewDocker(workdir, writeMounts(workdir, m.Capability.Filesystem.Write), m.Agent.Image,
			sockDir+":/c1run:rw", agentDir+":/agent:ro")
		sockInCell = "/c1run/" + filepath.Base(sock)
		agentBinInCell = "/agent/" + filepath.Base(agentBin)
		fmt.Println("cell backend: docker")
	} else {
		workdir = filepath.Join(ledger.Dir(), "workspace")
		os.RemoveAll(workdir)
		os.MkdirAll(workdir, 0755)
		agentWorkdir = workdir
		c = cell.NewLocal(workdir)
		fmt.Println("cell backend: local (no docker)")
	}
	// inside the container the repo is mounted at /work; the agent is told
	// that path, not the host path.
	if _, isDocker := c.(*cell.DockerCell); isDocker {
		agentWorkdir = "/work"
	}

	// 4. Run the agent under the broker.
	env := []string{
		"C1_BROKER_SOCK=" + sockInCell,
		"C1_RUN_ID=" + m.RunID,
		"C1_WORKDIR=" + agentWorkdir,
	}
	ledger.AppendFlow("AGENT_START", stringsJoin(m.Agent.Command, " "))
	result, runErr := c.Run(
		[]string{agentBinInCell, "-sock", sockInCell, "-run-id", m.RunID, "-workdir", agentWorkdir, "-mode", o.mode},
		env, timeout,
	)
	if runErr != nil {
		ledger.AppendFlow("CELL_ERROR", runErr.Error())
		c.Kill()
		return runErr
	}
	ledger.WriteArtifact("stdout.log", []byte(result.Output))
	ledger.AppendFlow("AGENT_EXIT", fmt.Sprintf("exit=%d", result.ExitCode))

	// 5. BREAK: a denied capability or non-zero exit stops the run.
	stopped := result.ExitCode != 0
	if stopped {
		ledger.AppendFlow("CELL_KILLED", fmt.Sprintf("exit=%d", result.ExitCode))
		c.Kill()
	}

	// 6. Verify evidence chain and build the verdict.
	chainErr := evidence.VerifyChain(filepath.Join(ledger.Dir(), "events.jsonl"))
	steps, stepErr := verdictSteps(chainErr, stopped)
	verdict := "REVIEW"
	if stepErr == nil {
		summary, _ := gatechain.Summarize(steps)
		v := gatechain.ConsumeDecision(summary)
		verdict = string(v.Action)
		ledger.AppendVerdict(verdict)
	}

	// 7. Write the receipt.
	writeReceipt(ledger.Dir(), m, frozen.Hash, verdict, result, chainErr, stopped)

	fmt.Printf("VERDICT: %s\n", verdict)
	return nil
}

// verdictSteps maps run outcome into GateChain steps. Missing evidence or a
// broken chain can never produce PASS.
func verdictSteps(chainErr error, stopped bool) ([]gatechain.ChainStep, error) {
	var evidenceStep gatechain.ChainStep
	if chainErr != nil {
		evidenceStep = gatechain.ChainStep{
			ID: "evidence", Label: "evidence chain invalid",
			Status: gatechain.StatusBlock, Coverage: gatechain.CovBlocked, Risk: gatechain.RiskBlocked,
			ReasonCode: "chain_invalid", EvidenceRef: "events.jsonl",
		}
	} else {
		evidenceStep = gatechain.ChainStep{
			ID: "evidence", Label: "evidence chain valid",
			Status: gatechain.StatusPass, Coverage: gatechain.CovCovered, Risk: gatechain.RiskLow,
			ReasonCode: "chain_valid", EvidenceRef: "events.jsonl",
		}
	}
	if stopped {
		evidenceStep = gatechain.ChainStep{
			ID: "agent", Label: "agent stopped on violation",
			Status: gatechain.StatusBlock, Coverage: gatechain.CovBlocked, Risk: gatechain.RiskBlocked,
			ReasonCode: "capability_violation", EvidenceRef: "events.jsonl",
		}
	}
	return []gatechain.ChainStep{evidenceStep}, nil
}

func writeReceipt(dir string, m *manifest.RunManifest, hash string, verdict string, r *cell.Result, chainErr error, stopped bool) {
	rec := map[string]any{
		"run_id":        m.RunID,
		"task":          m.Task,
		"agent":         m.Agent.Command,
		"manifest_hash": hash,
		"verdict":       verdict,
		"exit_code":     r.ExitCode,
		"timed_out":     r.TimedOut,
		"stopped":       stopped,
		"evidence":      "COMPLETE",
		"finished_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if chainErr != nil {
		rec["evidence"] = "BROKEN"
		rec["evidence_error"] = chainErr.Error()
	}
	data, _ := json.MarshalIndent(rec, "", "  ")
	os.WriteFile(filepath.Join(dir, "receipt.json"), data, 0644)

	md := fmt.Sprintf(`# C1 FORGE RECEIPT

Run        %s
Task       %s
Agent      %s
Manifest   %s

RESULT     %s
EVIDENCE   %s
EXIT       %d

Started    %s
`, m.RunID, m.Task, stringsJoin(m.Agent.Command, " "), hash[:16], verdict,
		rec["evidence"], r.ExitCode, m.RunID)
	os.WriteFile(filepath.Join(dir, "receipt.md"), []byte(md), 0644)
}

func fakeAgentDefault() string {
	// The fake agent binary lives alongside c1-vps when built with -o.
	exe, err := os.Executable()
	if err != nil {
		return "v0-fake-agent"
	}
	return filepath.Join(filepath.Dir(exe), "v0-fake-agent")
}

// writeMounts converts manifest write globs into host paths that the cell
// mounts read-write. Everything else in the workspace stays read-only.
func writeMounts(workdir string, write []string) []string {
	var out []string
	for _, g := range write {
		p := strings.TrimSuffix(strings.TrimSuffix(g, "/**"), "/")
		if p == "" || p == "." || p == "**" {
			continue
		}
		if strings.Contains(p, "**") {
			continue // nested glob not expressible as a mount; dir stays RO
		}
		out = append(out, filepath.Join(workdir, filepath.FromSlash(p)))
	}
	return out
}

func stringsJoin(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}