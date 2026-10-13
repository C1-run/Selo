package governor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPatchLimitsOk(t *testing.T) {
	g := NewGovernor(3, 5, 100, "")
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-foo
+bar`
	violation, msg := g.CheckPatchLimits(diff)
	if violation {
		t.Errorf("unexpected violation: %s", msg)
	}
}

func TestCheckPatchLimitsFilesExceeded(t *testing.T) {
	g := NewGovernor(3, 1, 100, "")
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a/a.go
@@ -1 +1 @@
-foo
+bar
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-foo
+bar`
	violation, _ := g.CheckPatchLimits(diff)
	if !violation {
		t.Error("expected violation for too many files")
	}
}

func TestCheckPatchLimitsLinesExceeded(t *testing.T) {
	g := NewGovernor(3, 10, 2, "")
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-foo
+bar
+extra
+more`
	violation, msg := g.CheckPatchLimits(diff)
	if !violation {
		t.Error("expected violation for too many patch lines")
	}
	if msg == "" {
		t.Error("expected non-empty msg")
	}
}

func TestCheckRoundsExceeded(t *testing.T) {
	g := NewGovernor(3, 10, 100, "")
	if g.CheckRoundsExceeded(2) {
		t.Error("round 2 should not exceed max 3")
	}
	if !g.CheckRoundsExceeded(3) {
		t.Error("round 3 should exceed max 3")
	}
}

func TestIsStoppedNoFile(t *testing.T) {
	g := NewGovernor(3, 10, 100, "/tmp/nonexistent-stop-file-xyz")
	if g.IsStopped() {
		t.Error("should not be stopped for non-existent file")
	}
}

// writeTask writes a task.md and returns its path.
func writeTask(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "task.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	return p
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Regression: a section header must close every other section. The documented
// task layout puts commands: before forbidden_files:; with the old parser the
// forbidden paths were also appended to commands and then executed as test
// commands.
func TestParseTaskConfigSectionsDoNotMerge(t *testing.T) {
	body := `id: "run-1"
goal: "add a helper"
repo: "/tmp/repo"
commands:
  - "python3 -m pytest -q"
max_minutes: 30
forbidden_files:
  - "README.md"
`
	_, _, _, _, commands, allowed, forbidden, claims, _, err := ParseTaskConfig(writeTask(t, body))
	if err != nil {
		t.Fatalf("ParseTaskConfig: %v", err)
	}
	if !equalStrings(commands, []string{"python3 -m pytest -q"}) {
		t.Errorf("commands = %q, want [python3 -m pytest -q]", commands)
	}
	if !equalStrings(forbidden, []string{"README.md"}) {
		t.Errorf("forbidden_files = %q, want [README.md]", forbidden)
	}
	if len(allowed) != 0 {
		t.Errorf("allowed_files = %q, want empty", allowed)
	}
	if len(claims) != 0 {
		t.Errorf("forbidden_claims = %q, want empty", claims)
	}
}

// Same separation must hold when the sections appear in the opposite order.
func TestParseTaskConfigSectionsDoNotMergeReversed(t *testing.T) {
	body := `id: "run-2"
goal: "add a helper"
forbidden_files:
  - "secrets/**"
commands:
  - "go test ./..."
`
	_, _, _, _, commands, _, forbidden, _, _, err := ParseTaskConfig(writeTask(t, body))
	if err != nil {
		t.Fatalf("ParseTaskConfig: %v", err)
	}
	if !equalStrings(commands, []string{"go test ./..."}) {
		t.Errorf("commands = %q, want [go test ./...]", commands)
	}
	if !equalStrings(forbidden, []string{"secrets/**"}) {
		t.Errorf("forbidden_files = %q, want [secrets/**]", forbidden)
	}
}

// allowed_files: and forbidden_claims: must also close the commands section.
func TestParseTaskConfigLaterSectionsCloseCommands(t *testing.T) {
	body := `commands:
  - "npm test"
allowed_files:
  - "src/**"
forbidden_claims:
  - "PRODUCTION_READY"
`
	_, _, _, _, commands, allowed, forbidden, claims, _, err := ParseTaskConfig(writeTask(t, body))
	if err != nil {
		t.Fatalf("ParseTaskConfig: %v", err)
	}
	if !equalStrings(commands, []string{"npm test"}) {
		t.Errorf("commands = %q, want [npm test]", commands)
	}
	if !equalStrings(allowed, []string{"src/**"}) {
		t.Errorf("allowed_files = %q, want [src/**]", allowed)
	}
	if !equalStrings(claims, []string{"PRODUCTION_READY"}) {
		t.Errorf("forbidden_claims = %q, want [PRODUCTION_READY]", claims)
	}
	if len(forbidden) != 0 {
		t.Errorf("forbidden_files = %q, want empty", forbidden)
	}
}
