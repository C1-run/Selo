package testintegrity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDeletedTestFile(t *testing.T) {
	before := &TestInventory{
		Files: []string{"foo_test.go", "bar_test.go", "main.go"},
		Count: 3,
	}
	after := &TestInventory{
		Files: []string{"foo_test.go", "main.go"},
		Count: 2,
	}

	result := Analyze(before, after, "", false)
	if result.Passed {
		t.Error("expected test integrity to fail when test file deleted")
	}
	if len(result.TestsRemoved) != 1 || result.TestsRemoved[0] != "bar_test.go" {
		t.Errorf("expected TestsRemoved=[bar_test.go], got %v", result.TestsRemoved)
	}
	if result.RecommendedVerdict != "NEEDS_HUMAN" {
		t.Errorf("expected NEEDS_HUMAN verdict, got %s", result.RecommendedVerdict)
	}
}

func TestDetectModifiedTestFile(t *testing.T) {
	before := &TestInventory{
		Files: []string{"foo_test.go", "main_test.go"},
		Count: 2,
	}
	after := &TestInventory{
		Files: []string{"foo_test.go", "main_test.go"},
		Count: 2,
	}
	diff := `diff --git a/main_test.go b/main_test.go
index abc..def 100644
--- a/main_test.go
+++ b/main_test.go
@@ -1,3 +1,4 @@
 package main
 
+// new line
 import "testing"
`

	result := Analyze(before, after, diff, false)
	if result.Passed {
		t.Error("expected test integrity to fail when test file modified")
	}
	if len(result.TestsModified) != 1 || result.TestsModified[0] != "main_test.go" {
		t.Errorf("expected TestsModified=[main_test.go], got %v", result.TestsModified)
	}
}

func TestDetectPackageJsonTestScriptChange(t *testing.T) {
	before := &TestInventory{
		TestCommands: []string{"npm test"},
		Count:        0,
	}
	after := &TestInventory{
		TestCommands: []string{"npm run build"},
		Count:        0,
	}

	result := Analyze(before, after, "", false)
	if result.Passed {
		t.Error("expected test integrity to fail when test command changed")
	}
	if len(result.TestCommandsChanged) == 0 {
		t.Error("expected TestCommandsChanged to be non-empty")
	}
}

func TestDetectSkipMarkerAdded(t *testing.T) {
	before := &TestInventory{
		Files: []string{"foo_test.go"},
		Count: 1,
	}
	after := &TestInventory{
		Files: []string{"foo_test.go"},
		Count: 1,
	}
	diff := `diff --git a/foo_test.go b/foo_test.go
index abc..def 100644
--- a/foo_test.go
+++ b/foo_test.go
@@ -1,3 +1,4 @@
 package main
 
+it.skip("should do something", func() {})
+// xit("old test")
 `

	result := Analyze(before, after, diff, false)
	if result.Passed {
		t.Error("expected test integrity to fail when skip marker added")
	}
	if len(result.SkipMarkersAdded) == 0 {
		t.Error("expected SkipMarkersAdded to be non-empty")
	}
}

func TestDetectSkipMarkerRustIgnore(t *testing.T) {
	before := &TestInventory{
		Files: []string{"tests/test.rs", "lib.rs"},
		Count: 2,
	}
	after := &TestInventory{
		Files: []string{"tests/test.rs", "lib.rs"},
		Count: 2,
	}
	diff := `diff --git a/tests/test.rs b/tests/test.rs
index abc..def 100644
--- a/tests/test.rs
+++ b/tests/test.rs
@@ -1,3 +1,4 @@
+#[ignore]
+fn test_something() {}
 `

	result := Analyze(before, after, diff, false)
	if result.Passed {
		t.Error("expected test integrity to fail when #[ignore] added")
	}
	if len(result.SkipMarkersAdded) == 0 {
		t.Error("expected SkipMarkersAdded to contain #[ignore]")
	}
}

func TestDetectAssertionRemoved(t *testing.T) {
	before := &TestInventory{
		Files: []string{"foo_test.go"},
		Count: 1,
	}
	after := &TestInventory{
		Files: []string{"foo_test.go"},
		Count: 1,
	}
	diff := `diff --git a/foo_test.go b/foo_test.go
index abc..def 100644
--- a/foo_test.go
+++ b/foo_test.go
@@ -1,5 +1,3 @@
 package main
 
 import "testing"
-
-func TestFoo(t *testing.T) {
-    assert.Equal(t, 1, 1)
-}
 `

	result := Analyze(before, after, diff, false)
	if result.Passed {
		t.Error("expected test integrity to fail when assertion removed")
	}
	if len(result.AssertionsRemoved) == 0 {
		t.Error("expected AssertionsRemoved to be non-empty")
	}
}

func TestAllowTestModificationWhenTaskAllows(t *testing.T) {
	before := &TestInventory{
		Files: []string{"foo_test.go"},
		Count: 1,
	}
	after := &TestInventory{
		Files: []string{"bar_test.go"},
		Count: 1,
	}
	diff := `diff --git a/foo_test.go b/bar_test.go
index abc..def 100644
--- a/foo_test.go
+++ b/bar_test.go
@@ -1 +1 @@
-old content
+new content
`

	result := Analyze(before, after, diff, true)
	if !result.Passed {
		t.Error("expected test integrity to pass when task allows modifications")
	}
	if result.RecommendedVerdict != "NEEDS_HUMAN" {
		t.Errorf("expected NEEDS_HUMAN recommended verdict, got %s", result.RecommendedVerdict)
	}
}

func TestInventoryCountShrinkFails(t *testing.T) {
	before := &TestInventory{
		Files: []string{"a_test.go", "b_test.go", "c_test.go"},
		Count: 3,
	}
	after := &TestInventory{
		Files: []string{"a_test.go"},
		Count: 1,
	}

	result := Analyze(before, after, "", false)
	if result.Passed {
		t.Error("expected test integrity to fail when inventory shrinks")
	}
	if len(result.TestsRemoved) != 2 {
		t.Errorf("expected 2 tests removed, got %d", len(result.TestsRemoved))
	}
}

func TestNoTestChangesPasses(t *testing.T) {
	before := &TestInventory{
		Files:        []string{"foo_test.go", "bar_test.go"},
		Count:        2,
		TestCommands: []string{"go test ./..."},
	}
	after := &TestInventory{
		Files:        []string{"foo_test.go", "bar_test.go"},
		Count:        2,
		TestCommands: []string{"go test ./..."},
	}

	result := Analyze(before, after, "", false)
	if !result.Passed {
		t.Error("expected test integrity to pass when no test changes")
	}
	if result.RecommendedVerdict != "" {
		t.Errorf("expected empty recommended verdict, got %s", result.RecommendedVerdict)
	}
}

func TestCaptureInventoryFindsTestFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "bar_test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.MkdirAll(filepath.Join(dir, "tests"), 0755)
	os.WriteFile(filepath.Join(dir, "tests", "test_helpers.py"), []byte("# test helper"), 0644)
	os.MkdirAll(filepath.Join(dir, "__tests__"), 0755)
	os.WriteFile(filepath.Join(dir, "__tests__", "app.test.ts"), []byte("test"), 0644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0644)

	inv := CaptureInventory(dir, nil)
	if inv.Count != 4 {
		t.Errorf("expected 4 test files, got %d: %v", inv.Count, inv.Files)
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"foo_test.go", true},
		{"bar.test.ts", true},
		{"bar.test.tsx", true},
		{"baz.spec.ts", true},
		{"baz.spec.tsx", true},
		{"test_foo.py", true},
		{"foo_test.py", true},
	{"tests/helper.py", true},
	{"src/tests/helper.py", true},
	{"__tests__/app.ts", true},
	{"src/__tests__/app.ts", true},
		{"src/tests/helper.py", true},
		{"main.go", false},
		{"README.md", false},
		{"src/app.ts", false},
	}
	for _, tc := range tests {
		got := isTestFile(tc.path)
		if got != tc.expected {
			t.Errorf("isTestFile(%q) = %v, expected %v", tc.path, got, tc.expected)
		}
	}
}
