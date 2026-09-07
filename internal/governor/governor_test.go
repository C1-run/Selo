package governor

import (
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
