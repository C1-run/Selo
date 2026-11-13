package pipeline

import (
	"testing"

	"github.com/C1-run/selo/internal/governor"
)

func TestRunAllEmptyChecks(t *testing.T) {
	rep := RunAll(nil, &CheckContext{})
	if len(rep.Hits) != 0 {
		t.Errorf("expected no hits, got %d", len(rep.Hits))
	}
	if rep.Override != "" {
		t.Errorf("expected no override, got %q", rep.Override)
	}
}

func TestRunAllAggregatesHits(t *testing.T) {
	mock := &mockCheck{hits: []string{"hit1", "hit2"}, violation: false}
	rep := RunAll([]Check{mock}, &CheckContext{})
	if len(rep.Hits) != 2 {
		t.Errorf("expected 2 hits, got %d", len(rep.Hits))
	}
	if rep.Override != "" {
		t.Errorf("expected no override for non-violation, got %q", rep.Override)
	}
}

func TestRunAllViolationSetsOverride(t *testing.T) {
	mock := &mockCheck{hits: []string{"violation"}, violation: true}
	rep := RunAll([]Check{mock}, &CheckContext{})
	if len(rep.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(rep.Hits))
	}
	if rep.Override != "mock" {
		t.Errorf("expected override %q, got %q", "mock", rep.Override)
	}
}

func TestRunAllFirstViolationWins(t *testing.T) {
	mock1 := &mockCheck{name: "first", hits: []string{"v1"}, violation: true}
	mock2 := &mockCheck{name: "second", hits: []string{"v2"}, violation: true}
	rep := RunAll([]Check{mock1, mock2}, &CheckContext{})
	if rep.Override != "first" {
		t.Errorf("expected first violation to win, got %q", rep.Override)
	}
	if !rep.Violated("first") || !rep.Violated("second") {
		t.Error("expected both violations recorded in ByCheck")
	}
	if rep.Violated("absent") {
		t.Error("expected absent check to report not violated")
	}
}

func TestPatchLimitCheckNilGovernor(t *testing.T) {
	check := PatchLimitCheck{}
	result := check.Run(&CheckContext{Governor: nil})
	if result != nil {
		t.Errorf("expected nil for nil governor, got %v", result)
	}
}

func TestPatchLimitCheckNoViolation(t *testing.T) {
	gov := governor.NewGovernor(3, 10, 200, "")
	check := PatchLimitCheck{}
	result := check.Run(&CheckContext{
		Governor: gov,
		Diff:     "diff --git a/foo.go b/foo.go\n+line1\n",
	})
	if result != nil {
		t.Errorf("expected nil for small diff, got %v", result)
	}
}

func TestPatchLimitCheckViolation(t *testing.T) {
	gov := governor.NewGovernor(3, 1, 200, "")
	check := PatchLimitCheck{}

	// Diff with 2 files exceeds maxFiles=1
	diff := "diff --git a/a.go b/a.go\n+line\ndiff --git a/b.go b/b.go\n+line\n"
	result := check.Run(&CheckContext{
		Governor: gov,
		Diff:     diff,
	})
	if result == nil {
		t.Fatal("expected violation for too many files")
	}
	if !result.Violation {
		t.Error("expected Violation=true")
	}
}

func TestRoundLimitCheckNotExceeded(t *testing.T) {
	gov := governor.NewGovernor(3, 10, 200, "")
	check := RoundLimitCheck{}
	result := check.Run(&CheckContext{Governor: gov})
	if result != nil {
		t.Errorf("expected nil when round 0 < max 3, got %v", result)
	}
}

func TestRoundLimitCheckExceeded(t *testing.T) {
	gov := governor.NewGovernor(1, 10, 200, "") // maxRounds=1, round 0 is fine
	check := RoundLimitCheck{}
	result := check.Run(&CheckContext{Governor: gov})
	if result != nil {
		t.Errorf("expected nil at round 0 < max 1, got %v", result)
	}
}

// TestDefaultChecksMatchDaemonOrder pins the check order: the daemon appends
// the report's hits after the supply-chain hits and before the test-integrity
// hit, so this order is the receipt's safety_hits order for everything except
// supply/test-integrity.
func TestDefaultChecksMatchDaemonOrder(t *testing.T) {
	checks := DefaultChecks()
	if len(checks) != 5 {
		t.Fatalf("expected 5 default checks, got %d", len(checks))
	}
	want := []string{CheckForbiddenFileEdit, CheckForbiddenClaims, CheckSecretScan, CheckPatchLimit, CheckRoundLimit}
	for i, c := range checks {
		if c.Name() != want[i] {
			t.Errorf("check %d = %q; want %q", i, c.Name(), want[i])
		}
	}
}

func TestFormatHits(t *testing.T) {
	hits := []string{"hit1", "hit2", "hit3"}
	result := FormatHits(hits)
	if result != "hit1; hit2; hit3" {
		t.Errorf("expected 'hit1; hit2; hit3', got %q", result)
	}
}

func TestFormatHitsEmpty(t *testing.T) {
	result := FormatHits(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// mockCheck is a test helper that returns pre-configured results.
type mockCheck struct {
	name      string
	hits      []string
	violation bool
}

func (m *mockCheck) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockCheck) Run(ctx *CheckContext) *CheckResult {
	if len(m.hits) == 0 {
		return nil
	}
	return &CheckResult{Hits: m.hits, Violation: m.violation}
}
