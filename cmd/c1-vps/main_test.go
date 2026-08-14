package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anomalyco/c1-forge/internal/cell"
	"github.com/anomalyco/c1-forge/internal/manifest"
)

// helper: create a fixture repo + manifest, run the full pipeline in-process.
func runV0(t *testing.T, mode string) (verdict string, rec map[string]any, ledgerDir string) {
	t.Helper()
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	os.MkdirAll(filepath.Join(repoDir, "src", "payments"), 0755)
	os.MkdirAll(filepath.Join(repoDir, "src", "auth"), 0755)

	m := manifestForTest(t, filepath.Join(base, "manifest.json"), repoDir)
	runsDir := filepath.Join(base, "runs")
	// keep the run dir short: unix socket paths are capped at ~104 bytes

	// build fake agent once per test run
	agentBin := filepath.Join(base, "v0-fake-agent")
	buildFakeAgent(t, agentBin)

	o := runOptions{
		manifestPath: filepath.Join(base, "manifest.json"),
		runID:        m.RunID,
		runsDir:      runsDir,
		mode:         mode,
		agentBin:     agentBin,
		timeoutMin:   2,
	}
	if err := run(&o); err != nil {
		t.Fatalf("run: %v", err)
	}

	recData, err := os.ReadFile(filepath.Join(runsDir, m.RunID, "receipt.json"))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	json.Unmarshal(recData, &rec)
	return rec["verdict"].(string), rec, filepath.Join(runsDir, m.RunID)
}

func TestV0AllowedRunPasses(t *testing.T) {
	verdict, rec, dir := runV0(t, "clean")
	if verdict != "pass" {
		t.Errorf("verdict = %s, want pass (%v)", verdict, rec)
	}
	// evidence present and complete
	for _, name := range []string{"manifest.json", "events.jsonl", "receipt.json", "receipt.md", "stdout.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}
	if rec["evidence"] != "COMPLETE" {
		t.Errorf("evidence = %v, want COMPLETE", rec["evidence"])
	}
}

func TestV0ScopeEscapeStops(t *testing.T) {
	verdict, rec, dir := runV0(t, "escape")
	if verdict != "stop" {
		t.Errorf("verdict = %s, want stop (%v)", verdict, rec)
	}
	if rec["stopped"] != true {
		t.Errorf("stopped = %v, want true", rec["stopped"])
	}
	// a denied capability must be in the ledger
	data, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if !contains(data, "CAPABILITY_DENIED") {
		t.Error("no CAPABILITY_DENIED event in ledger")
	}
}

// TestV0BrokerCannotBeBypassed is the adversarial proof: an agent that does
// NOT ask the broker — it just tries to reach capabilities directly — must
// be blocked by the cell boundary itself. That is what separates an
// execution boundary from command interception.
//
// This only holds behind a real sandbox (docker). The local backend has no
// containment and must not be presented as one, so the test skips there.
func TestV0BrokerCannotBeBypassed(t *testing.T) {
	if !cell.DockerAvailable() {
		t.Skip("no docker daemon; local backend is not a security boundary — run this in CI with docker")
	}
	verdict, rec, dir := runV0(t, "bypass")
	if verdict != "pass" {
		t.Fatalf("verdict = %s, want pass: cell boundary leaked (%v)", verdict, rec)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "stdout.log"))
	if !contains(out, "BYPASS IMPOSSIBLE") {
		t.Errorf("agent did not confirm boundary: %s", out)
	}
	if contains(out, "LEAKED") {
		t.Errorf("capability leaked through cell boundary:\n%s", out)
	}
}

func manifestForTest(t *testing.T, path, repoDir string) *manifest.RunManifest {
	t.Helper()
	m := &manifest.RunManifest{
		Version: "0.1",
		RunID:   "run_test_1",
		Task:    "fix payment webhook",
		Repo:    manifest.RepoSpec{Path: repoDir, Base: "HEAD"},
		Agent:   manifest.AgentSpec{Command: []string{"v0-fake-agent"}, Image: "alpine:3.19"},
		Capability: manifest.CapabilitySet{
			Filesystem: manifest.FilesystemCapability{
				Read:  []string{"src/**", "tests/**"},
				Write: []string{"src/payments/**"},
			},
			Exec: manifest.ExecCapability{Allow: []string{"npm test"}},
		},
		Network: manifest.NetworkPolicy{Connection: manifest.NetworkCapability{Mode: "none"}},
		Verify:  manifest.VerificationSpec{Test: "npm test"},
		Limits:  manifest.ResourceLimits{MaxMinutes: 2},
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return m
}

func buildFakeAgent(t *testing.T, out string) {
	cmd := exec.Command("go", "build", "-o", out, "github.com/anomalyco/c1-forge/cmd/v0-fake-agent")
	cmd.Dir = repoRoot(t)
	if out_, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake agent: %v\n%s", err, out_)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return filepath.Dir(filepath.Dir(wd)) // cmd/c1-vps -> repo root
}

func contains(data []byte, s string) bool {
	for i := 0; i+len(s) <= len(data); i++ {
		if string(data[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}