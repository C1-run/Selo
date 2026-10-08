package containment

import (
	"strings"
	"testing"
)

func TestRunContract_PassesValidContract(t *testing.T) {
	c := &RunContract{
		Goal:                 "fix-bug-42",
		DeadlineUTC:          "2026-06-02T00:00:00Z",
		MaxPatchCount:        5,
		PatchCount:           3,
		PatchesAfterDeadline: 0,
	}
	ok, reason := VerifyRunContract(c)
	if !ok {
		t.Fatalf("expected pass, got: %s", reason)
	}
}

func TestRunContract_FailsPatchAfterDeadline(t *testing.T) {
	c := &RunContract{
		Goal:                 "fix-bug-42",
		DeadlineUTC:          "2026-06-01T00:00:00Z",
		MaxPatchCount:        5,
		PatchCount:           1,
		PatchesAfterDeadline: 3,
	}
	ok, reason := VerifyRunContract(c)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if !strings.Contains(reason, "3 patch(es) after deadline") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestRunContract_FailsExceedsMaxPatches(t *testing.T) {
	c := &RunContract{
		Goal:                 "fix-bug-42",
		DeadlineUTC:          "2026-06-02T00:00:00Z",
		MaxPatchCount:        2,
		PatchCount:           5,
		PatchesAfterDeadline: 0,
	}
	ok, reason := VerifyRunContract(c)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if !strings.Contains(reason, "5 patches exceed max 2") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestRunContract_FailsNilContract(t *testing.T) {
	ok, reason := VerifyRunContract(nil)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if !strings.Contains(reason, "nil contract") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestRunContract_FailsEmptyDeadline(t *testing.T) {
	c := &RunContract{
		Goal:                 "fix-bug-42",
		DeadlineUTC:          "",
		MaxPatchCount:        5,
		PatchesAfterDeadline: 0,
	}
	ok, reason := VerifyRunContract(c)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if !strings.Contains(reason, "deadline_utc is empty") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestScopeContract_PassesInScope(t *testing.T) {
	c := &ScopeContract{
		AllowedPaths:   []string{"src/main.go", "docs/readme.md"},
		ForbiddenPaths: []string{".env", "*.pem", "*secret*"},
		ChangedPaths:   []string{"src/main.go", "docs/readme.md"},
	}
	ok, violations := VerifyScopeContract(c)
	if !ok {
		t.Fatalf("expected pass, got violations: %v", violations)
	}
}

func TestScopeContract_FailsForbiddenPath(t *testing.T) {
	c := &ScopeContract{
		AllowedPaths:   []string{"src/*"},
		ForbiddenPaths: []string{".env", "*secret*"},
		ChangedPaths:   []string{"src/main.go", ".env"},
	}
	ok, violations := VerifyScopeContract(c)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if len(violations) == 0 {
		t.Fatal("expected at least one violation")
	}
}

func TestScopeContract_FailsOutsideAllowed(t *testing.T) {
	c := &ScopeContract{
		AllowedPaths:   []string{"src/main.go"},
		ForbiddenPaths: nil,
		ChangedPaths:   []string{"src/main.go", "node_modules/pkg/index.js"},
	}
	ok, violations := VerifyScopeContract(c)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	found := false
	for _, v := range violations {
		if strings.Contains(v, "node_modules/pkg/index.js") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected violation for node_modules path, got: %v", violations)
	}
}

func TestScopeContract_PassesEmptyAllowed(t *testing.T) {
	c := &ScopeContract{
		AllowedPaths:   nil,
		ForbiddenPaths: []string{".env"},
		ChangedPaths:   []string{"src/main.go", "docs/readme.md"},
	}
	ok, violations := VerifyScopeContract(c)
	if !ok {
		t.Fatalf("expected pass (open mode), got violations: %v", violations)
	}
}

func TestScopeContract_FailsNilContract(t *testing.T) {
	ok, violations := VerifyScopeContract(nil)
	if ok {
		t.Fatal("expected fail, got pass")
	}
	if len(violations) == 0 {
		t.Fatal("expected at least one violation")
	}
}
