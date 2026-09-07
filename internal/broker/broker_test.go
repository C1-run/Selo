package broker

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/selo-dev/selo/internal/manifest"
)

func testManifest() *manifest.RunManifest {
	return &manifest.RunManifest{
		Version: "0.1",
		RunID:   "run_001",
		Repo:    manifest.RepoSpec{Path: "/repos/c1"},
		Agent:   manifest.AgentSpec{Command: []string{"sh"}},
		Capability: manifest.CapabilitySet{
			Filesystem: manifest.FilesystemCapability{
				Read:  []string{"src/**", "tests/**"},
				Write: []string{"src/payments/**", "tests/payments/**"},
			},
			Exec: manifest.ExecCapability{Allow: []string{"npm test"}},
		},
		Network: manifest.NetworkPolicy{Connection: manifest.NetworkCapability{Mode: "model", Model: "anthropic"}},
		Verify:  manifest.VerificationSpec{Test: "npm test"},
	}
}

func TestAuthorizeDenyByDefault(t *testing.T) {
	b := New(testManifest())
	// unknown kind
	d := b.Authorize(CapabilityRequest{RunID: "run_001", Kind: "network.connect", Target: "x"})
	if d.Allowed {
		t.Error("unknown kind must deny")
	}
	// empty manifest capabilities -> deny
	empty := &manifest.RunManifest{Version: "0.1", RunID: "r", Repo: manifest.RepoSpec{Path: "/x"}}
	empty.Capability = manifest.CapabilitySet{}
	b2 := New(empty)
	d = b2.Authorize(CapabilityRequest{RunID: "r", Kind: "filesystem.write", Target: "src/anything.ts"})
	if d.Allowed {
		t.Error("no write capability must deny")
	}
}

func TestFSWriteScope(t *testing.T) {
	b := New(testManifest())
	tests := []struct {
		target string
		want   bool
	}{
		{"src/payments/webhook.ts", true},
		{"src/payments/deep/webhook.ts", true},
		{"./src/payments/webhook.ts", true},
		{"src/auth/middleware.ts", false},
		{"production/auth.ts", false},
		{"src/paymentsx/evil.ts", false},
		{"README.md", false},
	}
	for _, tc := range tests {
		d := b.Authorize(CapabilityRequest{RunID: "run_001", Kind: "filesystem.write", Target: tc.target})
		if d.Allowed != tc.want {
			t.Errorf("write %s = %v (reason %s), want %v", tc.target, d.Allowed, d.Reason, tc.want)
		}
	}
}

func TestExecScope(t *testing.T) {
	b := New(testManifest())
	tests := []struct {
		target string
		args   []string
		want   bool
	}{
		{"npm", []string{"test"}, true},
		{"npm", []string{"run", "lint"}, false},
		{"sh", []string{"-c", "rm -rf /"}, false},
	}
	for _, tc := range tests {
		d := b.Authorize(CapabilityRequest{RunID: "run_001", Kind: "exec", Target: tc.target, Args: tc.args})
		if d.Allowed != tc.want {
			t.Errorf("exec %s %v = %v, want %v", tc.target, tc.args, d.Allowed, tc.want)
		}
	}
}

func TestGitCapabilities(t *testing.T) {
	b := New(testManifest())
	if d := b.Authorize(CapabilityRequest{RunID: "run_001", Kind: "git.push"}); d.Allowed {
		t.Error("git.push must deny by default")
	}
	if d := b.Authorize(CapabilityRequest{RunID: "run_001", Kind: "git.commit"}); d.Allowed {
		t.Error("git.commit must deny by default")
	}
}

func TestRunIDMismatchDenies(t *testing.T) {
	b := New(testManifest())
	d := b.Authorize(CapabilityRequest{RunID: "other", Kind: "filesystem.write", Target: "src/payments/a.ts"})
	if d.Allowed {
		t.Error("different run_id must deny")
	}
}

func TestServeOverSocket(t *testing.T) {
	b := New(testManifest())
	sock := filepath.Join(t.TempDir(), "broker.sock")
	go b.Serve(sock)
	defer os.Remove(sock)

	// wait for the socket to appear
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("broker socket never appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	req := CapabilityRequest{RunID: "run_001", Kind: "filesystem.write", Target: "src/auth/x.ts"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatal(err)
	}
	var d Decision
	if err := json.NewDecoder(conn).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if d.Allowed {
		t.Errorf("forbidden path allowed via socket: %+v", d)
	}
}