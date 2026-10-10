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

// TestValidateScopePatterns pins the validation itself. The malformed entries
// after the first group are the interesting ones: filepath.Match stops walking
// once a chunk has failed to match, so probing a pattern with a short name such
// as "x" accepts "abc*[" -- the unterminated class sits in a chunk that is
// never reached. Probing the pattern against its own bytes walks every chunk.
func TestValidateScopePatterns(t *testing.T) {
	valid := []struct {
		name string
		c    *ScopeContract
	}{
		{"literal", &ScopeContract{ForbiddenPaths: []string{".env"}}},
		{"star", &ScopeContract{ForbiddenPaths: []string{"*secret*"}}},
		{"suffix", &ScopeContract{ForbiddenPaths: []string{"*.pem"}}},
		{"single level", &ScopeContract{AllowedPaths: []string{"src/*.go", "docs/*.md"}}},
		{"doublestar is valid, just not recursive", &ScopeContract{ForbiddenPaths: []string{"src/**/*.go"}}},
		{"character class", &ScopeContract{ForbiddenPaths: []string{"*.[pP][eE][mM]"}}},
		{"empty", &ScopeContract{}},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := ValidateScopePatterns(tc.c); err != nil {
				t.Fatalf("rejected a valid pattern set: %v", err)
			}
		})
	}

	invalid := []struct {
		name string
		c    *ScopeContract
		want string
	}{
		{"unterminated class", &ScopeContract{ForbiddenPaths: []string{"[a-z"}}, "forbidden_paths"},
		{"unterminated class mid-pattern", &ScopeContract{ForbiddenPaths: []string{"abc["}}, "forbidden_paths"},
		{"unterminated class in a later chunk", &ScopeContract{ForbiddenPaths: []string{"abc*["}}, "forbidden_paths"},
		{"trailing backslash", &ScopeContract{ForbiddenPaths: []string{"a\\"}}, "forbidden_paths"},
		{"bad class in allowed_paths", &ScopeContract{AllowedPaths: []string{"src/*["}}, "allowed_paths"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			err := ValidateScopePatterns(tc.c)
			if err == nil {
				t.Fatal("accepted a malformed glob; the boundary it describes would be unenforced")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should name the field %q, got: %v", tc.want, err)
			}
		})
	}

	if err := ValidateScopePatterns(nil); err == nil {
		t.Fatal("expected an error for a nil contract")
	}
}

// TestScopeContract_InvalidGlobFailsClosed is the guard for the original defect:
// an unevaluable pattern used to be read as "does not match", so the run passed
// and the receipt recorded no violation.
func TestScopeContract_InvalidGlobFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    *ScopeContract
	}{
		{"forbidden", &ScopeContract{
			ForbiddenPaths: []string{"[a-z"},
			ChangedPaths:   []string{"src/main.go"},
		}},
		{"forbidden, later chunk", &ScopeContract{
			ForbiddenPaths: []string{"abc*["},
			ChangedPaths:   []string{"src/main.go"},
		}},
		{"allowed", &ScopeContract{
			AllowedPaths: []string{"src/*["},
			ChangedPaths: []string{"src/main.go"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, violations := VerifyScopeContract(tc.c)
			if ok {
				t.Fatal("a malformed glob passed verification; the run would proceed with an unenforced boundary")
			}
			if len(violations) == 0 {
				t.Fatal("expected a violation explaining the malformed glob")
			}
			if !strings.Contains(violations[0], "invalid glob") {
				t.Errorf("violation should name the invalid glob, got: %v", violations[0])
			}
		})
	}
}
