package p45

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/selo-dev/selo/internal/containment"
)

func writeFixture(t *testing.T, dir, name string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestP45StopsOnScopeViolation(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "contracts")
	if err := os.MkdirAll(contracts, 0755); err != nil {
		t.Fatal(err)
	}

	writeFixture(t, contracts, "goal_contract.json", map[string]interface{}{
		"goal":       "Fix pagination bug on users admin page",
		"splittable": false,
	})

	writeFixture(t, contracts, "scope_contract.json", map[string]interface{}{
		"allowed_paths":   []string{"src/admin/users/*"},
		"forbidden_paths": []string{".env", ".env.*", "secrets/**"},
	})

	writeFixture(t, contracts, "run_contract.json", map[string]interface{}{
		"goal":             "Fix pagination bug on users admin page",
		"deadline_utc":     "2026-06-02T23:59:59Z",
		"max_patch_count":  3,
		"max_files":        2,
		"max_patch_lines":  80,
		"stop_file":        ".containment-stop",
		"receipt_required": true,
	})

	goal, err := LoadGoal(contracts)
	if err != nil {
		t.Fatalf("LoadGoal: %v", err)
	}
	if goal.Goal == "" {
		t.Fatal("goal.Goal is empty")
	}
	if goal.Splittable {
		t.Fatal("expected splittable = false")
	}

	scope, err := LoadScope(contracts)
	if err != nil {
		t.Fatalf("LoadScope: %v", err)
	}
	if len(scope.AllowedPaths) != 1 {
		t.Fatalf("expected 1 allowed path, got %d", len(scope.AllowedPaths))
	}

	run, err := LoadRun(contracts)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if run.MaxPatchCount != 3 {
		t.Fatalf("expected max_patch_count=3, got %d", run.MaxPatchCount)
	}
	if !strings.Contains(run.StopFile, ".containment-stop") {
		t.Fatalf("stop_file should reference .containment-stop, got %q", run.StopFile)
	}
	if !run.ReceiptRequired {
		t.Fatal("expected receipt_required = true")
	}

	scope.ChangedPaths = []string{
		"src/admin/users/query.go",
		".env",
	}

	ok, violations := containment.VerifyScopeContract(scope)
	if ok {
		t.Fatal("VerifyScopeContract returned true, expected false (scope violation for .env)")
	}
	if len(violations) == 0 {
		t.Fatal("expected at least 1 scope violation")
	}
	foundEnvViolation := false
	for _, v := range violations {
		if strings.Contains(v, ".env") {
			foundEnvViolation = true
			break
		}
	}
	if !foundEnvViolation {
		t.Fatalf("expected violation mentioning .env, got: %v", violations)
	}

	var buf bytes.Buffer
	err = WriteSlip(&buf, &P45Slip{
		RunID:              "stop-test-001",
		Goal:               goal.Goal,
		Scope:              "src/admin/users/* (allowed); .env, secrets/** (forbidden)",
		PatchBudget:        "3 patches, 2 files max, 80 lines max",
		ChangedFiles:       scope.ChangedPaths,
		Violation:          violations[0],
		Warnings:           []string{"fixture data only; no real .env accessed"},
		Verdict:            "SCOPE_VIOLATION",
		Reason:             "changed file matches forbidden path pattern",
		NextAllowedAction:  "review receipt, inspect changes, decide next step",
		ForbiddenNextAction: "do not start a new run without reviewing this receipt",
	})
	if err != nil {
		t.Fatalf("WriteSlip: %v", err)
	}

	slip := buf.String()
	if !strings.Contains(slip, "SCOPE_VIOLATION") {
		t.Fatal("slip should contain SCOPE_VIOLATION")
	}
	if !strings.Contains(slip, "STOP_RECEIPT is an end-of-run record") {
		t.Fatal("slip should contain STOP_RECEIPT disclaimer")
	}
	if !strings.Contains(slip, "does not prove semantic safety") {
		t.Fatal("slip should contain semantic safety disclaimer")
	}
	if !strings.Contains(slip, "does not certify code correctness") {
		t.Fatal("slip should contain certification disclaimer")
	}
	if !strings.Contains(slip, "does not stop agents automatically") {
		t.Fatal("slip should contain automatic stopping disclaimer")
	}

	if strings.Contains(slip, "c1f-") {
		t.Fatal("slip should not contain c1f- prefix")
	}
	if strings.Contains(slip, "safety_hits") {
		t.Fatal("slip should not contain safety_hits field")
	}
	if strings.Contains(slip, ".selo-stop") {
		t.Fatal("slip should not contain .selo-stop")
	}
}

func TestLoadGoal_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "goal_contract.json", map[string]interface{}{
		"goal": "",
	})
	_, err := LoadGoal(dir)
	if err == nil {
		t.Fatal("expected error for empty goal")
	}
}

func TestLoadScope_RejectsForbiddenNaming(t *testing.T) {
	tests := []struct {
		name     string
		forbidden []string
	}{
		{"c1f- in allowed_paths", []string{"c1f-src"}},
		{"safety_hits in forbidden_paths", []string{"safety_hits"}},
		{".selo-stop in forbidden_paths", []string{".selo-stop"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "scope_contract.json", map[string]interface{}{
				"allowed_paths":   []string{"src/*"},
				"forbidden_paths": tc.forbidden,
			})
			_, err := LoadScope(dir)
			if err == nil {
				t.Fatal("expected error for forbidden naming")
			}
		})
	}
}

func TestLoadRun_RejectsMissingFields(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "run_contract.json", map[string]interface{}{
		"goal": "test",
	})
	_, err := LoadRun(dir)
	if err == nil {
		t.Fatal("expected error for missing deadline_utc")
	}
}

func TestLoadRun_RejectsBadStopFile(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "run_contract.json", map[string]interface{}{
		"goal":            "test",
		"deadline_utc":    "2026-06-02T23:59:59Z",
		"max_patch_count": 1,
		"stop_file":       ".selo-stop",
	})
	_, err := LoadRun(dir)
	if err == nil {
		t.Fatal("expected error for .selo-stop in stop_file")
	}
}

func TestWriteSlip_AllRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	err := WriteSlip(&buf, &P45Slip{
		RunID:              "stop-abc-123",
		Goal:               "test goal",
		Scope:              "src/*",
		PatchBudget:        "5 patches",
		ChangedFiles:       []string{"src/main.go", ".env"},
		Violation:          ".env is forbidden",
		Warnings:           []string{"fixture only"},
		Verdict:            "SCOPE_VIOLATION",
		Reason:             "forbidden path",
		NextAllowedAction:  "review",
		ForbiddenNextAction: "no restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	slip := buf.String()
	required := []string{
		"run_id", "goal", "scope", "patch_budget",
		"files_changed", "Violation", "Warnings",
		"verdict", "reason",
		"next_allowed_action", "forbidden_next_action",
		"STOP_RECEIPT is an end-of-run record",
		"does not prove semantic safety",
		"does not certify code correctness",
		"does not stop agents automatically",
		"does not prove production readiness",
	}
	for _, r := range required {
		if !strings.Contains(slip, r) {
			t.Errorf("slip should contain %q", r)
		}
	}
	if strings.Contains(slip, "c1f-") || strings.Contains(slip, "safety_hits") || strings.Contains(slip, ".selo-stop") {
		t.Fatal("slip should not contain forbidden naming")
	}
}

func TestCheckForbidden_DetectsTerms(t *testing.T) {
	if err := checkForbidden("src/c1f-foo.go"); err == nil {
		t.Error("expected error for c1f-")
	}
	if err := checkForbidden("safety_hits.txt"); err == nil {
		t.Error("expected error for safety_hits")
	}
	if err := checkForbidden(".selo-stop"); err == nil {
		t.Error("expected error for .selo-stop")
	}
	if err := checkForbidden("src/main.go"); err != nil {
		t.Errorf("unexpected error for clean path: %v", err)
	}
}
