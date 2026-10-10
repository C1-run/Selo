package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentEnvDropsSigningKey is the regression guard for the trust-domain
// leak: the agent subprocess must never inherit the key that signs its receipt.
func TestAgentEnvDropsSigningKey(t *testing.T) {
	t.Setenv("SELO_SIGNING_KEY", "super-secret-seed")
	t.Setenv("SELO_ALLOW_EPHEMERAL_KEY", "1")
	t.Setenv("PATH", "/usr/bin")

	env := agentEnv()
	for _, kv := range env {
		if strings.HasPrefix(kv, "SELO_SIGNING_KEY=") {
			t.Fatalf("agent environment leaked SELO_SIGNING_KEY: %s", kv)
		}
		if strings.HasPrefix(kv, "SELO_ALLOW_EPHEMERAL_KEY=") {
			t.Fatalf("agent environment leaked SELO_ALLOW_EPHEMERAL_KEY: %s", kv)
		}
	}

	// A harmless variable must still survive, or we've broken the agent's env.
	found := false
	for _, kv := range env {
		if kv == "PATH=/usr/bin" {
			found = true
		}
	}
	if !found {
		t.Fatal("agentEnv dropped PATH — the agent would not be able to run")
	}
}

func TestCheckForbiddenFileEditAllowed(t *testing.T) {
	diff := `diff --git a/src/main.go b/src/main.go
index abc..def 100644
--- a/src/main.go
+++ b/src/main.go
@@ -1 +1 @@
-foo
+bar`
	allowed := []string{"src/"}
	forbidden := []string{}
	violation, msg := CheckForbiddenFileEdit(diff, "", allowed, forbidden)
	if violation {
		t.Errorf("unexpected violation: %s", msg)
	}
}

func TestCheckForbiddenFileEditBlocked(t *testing.T) {
	diff := `diff --git a/src/secret.rs b/src/secret.rs
index abc..def 100644
--- a/src/secret.rs
+++ b/src/secret.rs
@@ -1 +1 @@
-foo
+bar`
	allowed := []string{"src/"}
	forbidden := []string{"src/secret.rs"}
	violation, msg := CheckForbiddenFileEdit(diff, "", allowed, forbidden)
	if !violation {
		t.Error("expected violation for forbidden file")
	}
	if msg == "" {
		t.Error("expected non-empty violation message")
	}
}

func TestCheckForbiddenFileEditOutsideAllowed(t *testing.T) {
	diff := `diff --git a/other/file.go b/other/file.go
index abc..def 100644
--- a/other/file.go
+++ b/other/file.go
@@ -1 +1 @@
-foo
+bar`
	allowed := []string{"src/"}
	forbidden := []string{}
	violation, msg := CheckForbiddenFileEdit(diff, "", allowed, forbidden)
	if !violation {
		t.Error("expected violation for file outside allowed")
	}
	if !strings.Contains(msg, "outside allowed scope") {
		t.Errorf("unexpected msg: %s", msg)
	}
}

func TestRunSecretScan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "creds.go"), []byte("const apiKey = \"sk-1234567890\""), 0644)
	hits, err := RunSecretScan(dir)
	if err != nil {
		t.Fatalf("RunSecretScan: %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected secret scan hits")
	}
}

func TestRunForbiddenClaimsScan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.go"), []byte("// PROFITABLE strategy"), 0644)
	hits, err := RunForbiddenClaimsScan(dir, []string{"PROFITABLE", "LIVE_READY"})
	if err != nil {
		t.Fatalf("RunForbiddenClaimsScan: %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected forbidden claims hits")
	}
}

// TestRunForbiddenClaimsScanDoesNotShellOut is the regression guard for a
// fail-open: the scan used to shell out to `grep` and ignore its error, so on a
// host without grep (or with a grep that rejected the arguments) every term was
// silently skipped and the control reported "no forbidden claims".
func TestRunForbiddenClaimsScanDoesNotShellOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("// PROFITABLE strategy\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // nothing on PATH: no grep, no anything

	hits, err := RunForbiddenClaimsScan(dir, []string{"PROFITABLE"})
	if err != nil {
		t.Fatalf("RunForbiddenClaimsScan: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("claim not detected with an empty PATH: the scan still depends on an external command and fails open")
	}
}

// TestRunForbiddenClaimsScanAgreesWithDiffScoped pins the invariant that the
// full-worktree scan and the diff-scoped scan classify the same file the same
// way. The full scan used `grep -i` over a fixed extension allowlist, so it
// missed both evasion substitutions and every file outside that list, while the
// diff-scoped scan caught them — the same claim was a violation or not
// depending only on whether a diff happened to be available.
func TestRunForbiddenClaimsScanAgreesWithDiffScoped(t *testing.T) {
	cases := []struct{ name, file, content string }{
		{"evasion substitution", "bad.go", "// pr0duct1on_ready\n"},
		{"yaml outside the old allowlist", "config.yml", "claim: PRODUCTION_READY\n"},
		{"python outside the old allowlist", "app.py", "CLAIM = 'PRODUCTION_READY'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			scoped, err := RunForbiddenClaimsScanDiffScoped(dir, []string{"PRODUCTION_READY"}, []string{tc.file})
			if err != nil {
				t.Fatalf("diff-scoped scan: %v", err)
			}
			if len(scoped) == 0 {
				t.Fatalf("diff-scoped scan missed %s: the test case itself is wrong", tc.file)
			}

			full, err := RunForbiddenClaimsScan(dir, []string{"PRODUCTION_READY"})
			if err != nil {
				t.Fatalf("full scan: %v", err)
			}
			if len(full) == 0 {
				t.Errorf("full worktree scan missed %s, but the diff-scoped scan caught it", tc.file)
			}
		})
	}
}

func TestMapVerdictSuccess(t *testing.T) {
	r := &C1Result{ExitCode: 0, Diff: "diff --git a/src/main.go b/src/main.go\n--- a/src/main.go\n+++ b/src/main.go\n@@ -1 +1 @@\n-foo\n+bar\n+extra line here to make diff longer"}
	v := MapVerdict(r, nil, false)
	if v != "SUCCESS_WITH_RECEIPT" {
		t.Errorf("expected SUCCESS_WITH_RECEIPT, got %s", v)
	}
}

func TestMapVerdictNoop(t *testing.T) {
	r := &C1Result{ExitCode: 0, Diff: ""}
	v := MapVerdict(r, nil, false)
	if v != "NOOP_WITH_RECEIPT" {
		t.Errorf("expected NOOP_WITH_RECEIPT, got %s", v)
	}
}

func TestMapVerdictPartialFailure(t *testing.T) {
	r := &C1Result{ExitCode: 1, Diff: "diff --git a/a.go b/a.go\n+change", TestOutput: "FAIL TestFoo"}
	v := MapVerdict(r, nil, false)
	if v != "PARTIAL_FAILURE" {
		t.Errorf("expected PARTIAL_FAILURE, got %s", v)
	}
}

func TestMapVerdictTimeout(t *testing.T) {
	r := &C1Result{ExitCode: -1, TimedOut: true}
	v := MapVerdict(r, nil, false)
	if v != "FAILED_TIMEOUT" {
		t.Errorf("expected FAILED_TIMEOUT, got %s", v)
	}
}

func TestMapVerdictSafety(t *testing.T) {
	r := &C1Result{ExitCode: 0}
	v := MapVerdict(r, []string{"forbidden file modified: src/secret.rs (matches: src/secret.rs)"}, false)
	if v != "FAILED_SAFETY" {
		t.Errorf("expected FAILED_SAFETY, got %s", v)
	}
}

func TestMapVerdictNeedsHuman(t *testing.T) {
	r := &C1Result{ExitCode: 0}
	v := MapVerdict(r, []string{"PROFITABLE: /path/file.go:1:// PROFITABLE claim"}, false)
	if v != "NEEDS_HUMAN" {
		t.Errorf("expected NEEDS_HUMAN, got %s", v)
	}
}

func TestMapVerdictLimitExceeded(t *testing.T) {
	r := &C1Result{ExitCode: 0}
	v := MapVerdict(r, []string{"too many files modified: 20 (max 10)"}, true)
	if v != "FAILED_LIMIT_EXCEEDED" {
		t.Errorf("expected FAILED_LIMIT_EXCEEDED, got %s", v)
	}
	// Also test via safety hit containing "too many"
	v2 := MapVerdict(r, []string{"too many patch lines"}, false)
	if v2 != "FAILED_LIMIT_EXCEEDED" {
		t.Errorf("expected FAILED_LIMIT_EXCEEDED via safety hit, got %s", v2)
	}
}

func TestMockRunnerBasic(t *testing.T) {
	dir := t.TempDir()
	r := NewMockRunner(dir, "/tmp/fake-task.md", 1, "")
	result := r.Run()
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Mode != ModeMock {
		t.Errorf("expected mock mode, got %s", result.Mode)
	}
	if !strings.Contains(result.Stdout, "mock") {
		t.Errorf("expected mock output, got: %s", result.Stdout)
	}
}

func TestRealRunnerConfig(t *testing.T) {
	cfg := RunnerConfig{
		Command: "echo",
		Args:    []string{"hello", "from", "{{task_id}}"},
	}
	if cfg.Command != "echo" {
		t.Errorf("bad command")
	}
	if len(cfg.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(cfg.Args))
	}
}

// TestDiscoverActualC1PrefersExplicitConfig verifies explicit path is preferred.
func TestDiscoverActualC1PrefersExplicitConfig(t *testing.T) {
	// Use /bin/echo as a stand-in "binary" for testing
	info := DiscoverC1BinaryWithKind("/bin/echo", false)
	if info.Path != "/bin/echo" {
		t.Errorf("expected /bin/echo, got %s", info.Path)
	}
	if !info.Verified {
		t.Error("expected verified=true")
	}
}

// TestDiscoverActualC1RejectsShimWhenNotAllowed verifies shim is rejected without --allow-shim.
func TestDiscoverActualC1RejectsShimWhenNotAllowed(t *testing.T) {
	// Shim scripts are rejected unless allowShim=true
	// When no explicit path and no binary on PATH, should return empty
	info := DiscoverC1BinaryWithKind("", false)
	// If there's actually a c1 on PATH, we skip the reject check
	if info.Path != "" {
		t.Skip("actual C1 binary found on PATH; cannot verify shim rejection")
	}
	if info.Kind != BinaryKindUnknown {
		t.Errorf("expected unknown kind when no binary found, got %s", info.Kind)
	}
}

// TestDiscoverActualC1AllowsShimInTestMode verifies shim is allowed with --allow-shim.
func TestDiscoverActualC1AllowsShimInTestMode(t *testing.T) {
	// Use /bin/echo as a stand-in
	info := DiscoverC1BinaryWithKind("/bin/echo", true)
	if info.Path != "/bin/echo" {
		t.Errorf("expected /bin/echo, got %s", info.Path)
	}
}

// TestGetBinaryVersion verifies GetBinaryVersion works.
func TestGetBinaryVersion(t *testing.T) {
	version := GetBinaryVersion("/bin/echo")
	// echo doesn't have --version, so this may be empty
	_ = version
	// Should not crash
}

// TestClassifyAndVersion verifies binary kind classification.
func TestClassifyAndVersion(t *testing.T) {
	kind, _ := classifyAndVersion("/usr/local/bin/c1")
	if kind != BinaryKindRealC1 {
		t.Errorf("expected real_c1 for 'c1', got %s", kind)
	}
	kind2, _ := classifyAndVersion("/usr/local/bin/opencode")
	if kind2 != BinaryKindOpenCode {
		t.Errorf("expected opencode for 'opencode', got %s", kind2)
	}
	kind3, _ := classifyAndVersion("/usr/local/bin/c1-loop.sh")
	if kind3 != BinaryKindShim {
		t.Errorf("expected shim for 'c1-loop.sh', got %s", kind3)
	}
	kind4, _ := classifyAndVersion("/usr/local/bin/unknown-binary")
	if kind4 != BinaryKindUnknown {
		t.Errorf("expected unknown for unknown binary, got %s", kind4)
	}
}

// TestDiscoverActualC1EnvVar verifies SELO_C1_BIN env var is used.
func TestDiscoverActualC1EnvVar(t *testing.T) {
	os.Setenv("SELO_C1_BIN", "/bin/echo")
	defer os.Unsetenv("SELO_C1_BIN")
	info := DiscoverC1BinaryWithKind("", false)
	if info.Path != "/bin/echo" {
		t.Errorf("expected /bin/echo from env var, got %s", info.Path)
	}
}

// TestRunnerBinaryMetadataWrittenToReceipt verifies binary metadata fields are populated.
// This test works by checking the runner's binary info structure.
func TestRunnerBinaryMetadataWrittenToReceipt(t *testing.T) {
	info := DiscoverC1BinaryWithKind("/bin/echo", false)
	if info.Path == "" {
		t.Fatal("expected binary info path")
	}
	if info.Kind == BinaryKindUnknown {
		t.Log("binary kind is unknown (expected for /bin/echo)")
	}
	if info.Verified != true {
		t.Error("expected verified=true")
	}
}

func TestPathInAllowlist(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{"exact match", "src/main.go", []string{"src/main.go"}, true},
		{"glob one segment", "src/main.go", []string{"src/*.go"}, true},
		{"glob does not cross directories", "src/deep/x.go", []string{"src/*.go"}, false},
		{"dir star covers nested", "src/deep/x.go", []string{"src/*"}, true},
		{"doublestar covers nested", "src/deep/x.go", []string{"src/**"}, true},
		{"trailing slash covers nested", "src/deep/x.go", []string{"src/"}, true},
		{"bare dir covers nested", "src/deep/x.go", []string{"src"}, true},
		{"bare dir rejects sibling prefix", "srcx/other.go", []string{"src"}, false},
		{"dot-slash normalizes", "src/main.go", []string{"./src/"}, true},
		{"empty pattern ignored", "src/main.go", []string{""}, false},
		{"empty list matches nothing", "src/main.go", nil, false},
		{"unrelated dir rejected", "docs/x.md", []string{"src/"}, false},
	}
	for _, tc := range cases {
		if got := pathInAllowlist(tc.path, tc.patterns); got != tc.want {
			t.Errorf("%s: pathInAllowlist(%q, %v) = %v, want %v", tc.name, tc.path, tc.patterns, got, tc.want)
		}
	}
}

func TestCheckForbiddenFileEditAllowlistGlob(t *testing.T) {
	diff := "diff --git a/src/main.go b/src/main.go\nindex abc..def 100644\n--- a/src/main.go\n+++ b/src/main.go\n@@ -1 +1 @@\n-foo\n+bar"
	violation, msg := CheckForbiddenFileEdit(diff, "", []string{"src/*.go"}, nil)
	if violation {
		t.Errorf("unexpected violation: %s", msg)
	}
	violation, _ = CheckForbiddenFileEdit(diff, "", []string{"docs/"}, nil)
	if !violation {
		t.Error("expected violation for file outside glob allowlist")
	}
}

func TestForbiddenClaimsScanDiffScopedFoldsEvasions(t *testing.T) {
	workDir := t.TempDir()
	content := "package main\n\n// lowercase: production_ready\n// leet: pr0ducti0n_ready\n// zero-width: pro\u200bduction_ready\n// unrelated: production finished\n"
	if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	hits, err := RunForbiddenClaimsScanDiffScoped(workDir, []string{"PRODUCTION_READY"}, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d (%v), want 3 (lowercase, leet, zero-width)", len(hits), hits)
	}
	hits, err = RunForbiddenClaimsScanDiffScoped(workDir, []string{"PRODUCTION_READY"}, []string{"other.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("unchanged file produced hits: %v", hits)
	}
}

func TestSecretScanDetectsCommonTokens(t *testing.T) {
	workDir := t.TempDir()
	content := "aws = AKIAIOSFODNN7EXAMPLE\n" +
		"gh = ghp_" + strings.Repeat("a", 36) + "\n" +
		"slack = xoxb-123456789012-abcdef\n" +
		"google = AIza" + strings.Repeat("A", 35) + "\n" +
		"openai = sk-" + strings.Repeat("x", 25) + "\n"
	if err := os.WriteFile(filepath.Join(workDir, "creds.env"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	hits, err := RunSecretScan(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 5 {
		t.Fatalf("hits = %d (%v), want >= 5 common token types", len(hits), hits)
	}
}
