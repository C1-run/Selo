package manifest

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFreezeRequiresFields(t *testing.T) {
	cases := []struct {
		name   string
		m      RunManifest
		errSub string
	}{
		{"nil", RunManifest{}, "version"},
		{"no run id", RunManifest{Version: "0.1"}, "run_id"},
		{"no repo", RunManifest{Version: "0.1", RunID: "r1"}, "repo"},
		{"no agent", RunManifest{Version: "0.1", RunID: "r1", Repo: RepoSpec{Path: "/x"}}, "agent"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Freeze(&c.m)
			if err == nil {
				t.Fatalf("expected error containing %q", c.errSub)
			}
		})
	}
}

func TestFreezeHashIsDeterministic(t *testing.T) {
	m := RunManifest{
		Version: "0.1",
		RunID:   "run_001",
		Task:    "fix webhook",
		Repo:    RepoSpec{Path: "/repos/c1", Base: "HEAD"},
		Agent:   AgentSpec{Command: []string{"claude"}},
		Capability: CapabilitySet{
			Filesystem: FilesystemCapability{
				Read:  []string{"src/**", "tests/**"},
				Write: []string{"tests/payments/**", "src/payments/**"},
			},
		},
		Network: NetworkPolicy{Connection: NetworkCapability{Mode: "none"}},
		Verify:  VerificationSpec{Test: "npm test"},
	}
	f1, err := Freeze(&m)
	if err != nil {
		t.Fatal(err)
	}
	// Same manifest, different slice ordering -> same hash
	m2 := m
	m2.Capability.Filesystem.Write = []string{"src/payments/**", "tests/payments/**"}
	f2, err := Freeze(&m2)
	if err != nil {
		t.Fatal(err)
	}
	if f1.Hash != f2.Hash {
		t.Errorf("hash differs with reordered globs: %s vs %s", f1.Hash, f2.Hash)
	}
	if f1.Hash != HashOf(&m) {
		t.Errorf("HashOf mismatch")
	}
}

func TestLoadRoundTrip(t *testing.T) {
	m := RunManifest{
		Version: "0.1", RunID: "run_002", Task: "t",
		Repo: RepoSpec{Path: "/repos/c1"}, Agent: AgentSpec{Command: []string{"sh"}},
		Capability: CapabilitySet{Exec: ExecCapability{Allow: []string{"npm test"}}},
		Network:    NetworkPolicy{Connection: NetworkCapability{Mode: "none"}},
	}
	data, _ := json.Marshal(m)
	path := t.TempDir() + "/manifest.json"
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Manifest.Task != "t" {
		t.Errorf("task = %q", f.Manifest.Task)
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}