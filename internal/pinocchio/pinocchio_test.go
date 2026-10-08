package pinocchio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/C1-run/selo/internal/runner"
)

func TestPinocchioSuccessPasses(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop --task-file x --workdir y"},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
		TaskMeta:       nil,
	})

	if !r.Verified {
		t.Errorf("expected Verified=true, got false. FalseClaims=%v Inconsistencies=%v", r.FalseClaims, r.Inconsistencies)
	}
	if r.FinalVerdict != "SUCCESS_WITH_RECEIPT" {
		t.Errorf("expected FinalVerdict=SUCCESS_WITH_RECEIPT, got %s", r.FinalVerdict)
	}
}

func TestPinocchioSuccessWithFailedTestsDowngrades(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop --task-file x --workdir y"},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		TestOutput:     "--- FAIL: TestAdd (0.01s)\nFAIL",
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
		TaskMeta:       &receipt.TaskMeta{Commands: []string{"go test ./..."}},
	})

	if r.Verified {
		t.Error("expected Verified=false for failed tests")
	}
	if len(r.FalseClaims) == 0 {
		t.Error("expected FalseClaims non-empty")
	}
	if r.FinalVerdict != "NEEDS_HUMAN" {
		t.Errorf("expected FinalVerdict=NEEDS_HUMAN, got %s", r.FinalVerdict)
	}
}

func TestPinocchioSuccessWithScanFailureFailsSafety(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		SafetyHits:     []string{"forbidden file modified: secret.key"},
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for safety hit")
	}
	if r.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("expected FinalVerdict=FAILED_SAFETY, got %s", r.FinalVerdict)
	}
}

func TestPinocchioNoopWithDiffDowngrades(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		Diff:           "diff --git a/README.md b/README.md\nnew file mode 100644\n--- /dev/null\n+++ b/README.md\n@@ -0,0 +1 @@\n+hello",
		InitialVerdict: "NOOP_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for noop with diff")
	}
	if len(r.FalseClaims) == 0 {
		t.Error("expected FalseClaims for diff detected")
	}
	if r.FinalVerdict != "NEEDS_HUMAN" {
		t.Errorf("expected FinalVerdict=NEEDS_HUMAN, got %s", r.FinalVerdict)
	}
}

func TestPinocchioMissingC1ReceiptDowngrades(t *testing.T) {
	// C1LoopReceiptPath set but file doesn't exist
	dir := t.TempDir()
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:            &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:        runner.ModeReal,
		WorktreePath:      dir,
		C1LoopReceiptPath: filepath.Join(dir, "c1-receipt.json"), // does not exist
		InitialVerdict:    "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for missing C1 receipt")
	}
}

func TestPinocchioC1ReceiptVerdictMismatchDowngrades(t *testing.T) {
	dir := t.TempDir()
	receiptPath := filepath.Join(dir, "c1-receipt.json")
	os.WriteFile(receiptPath, []byte(`{"status":"completed","exit_code":1}`), 0644)

	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:            &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:        runner.ModeReal,
		WorktreePath:      dir,
		C1LoopReceiptPath: receiptPath,
		InitialVerdict:    "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for C1 receipt mismatch")
	}
	if len(r.Inconsistencies) == 0 {
		t.Error("expected inconsistencies for C1 receipt exit code mismatch")
	}
}

func TestPinocchioForbiddenClaimCaught(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0},
		RunnerMode:     runner.ModeReal,
		SafetyHits:     []string{"PROFITABLE: found in code comments"},
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for forbidden claim")
	}
	if len(r.SafetyFindings) == 0 {
		t.Error("expected SafetyFindings non-empty")
	}
	if r.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("expected FinalVerdict=FAILED_SAFETY, got %s", r.FinalVerdict)
	}
}

func TestPinocchioSecretScanHitFailsSafety(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0},
		RunnerMode:     runner.ModeReal,
		SafetyHits:     []string{"secret: found api_key in config"},
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
	})

	if r.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("expected FinalVerdict=FAILED_SAFETY, got %s", r.FinalVerdict)
	}
	if len(r.SafetyFindings) == 0 {
		t.Error("expected SafetyFindings for secret hit")
	}
}

func TestPinocchioFailedSafetyPreserved(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		SafetyHits:     []string{"forbidden file modified: src/secret.rs"},
		InitialVerdict: "FAILED_SAFETY",
	})

	if r.Verified {
		t.Error("expected Verified=false for safety")
	}
	if r.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("expected FinalVerdict=FAILED_SAFETY, got %s", r.FinalVerdict)
	}
}

func TestPinocchioPartialFailurePreserved(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 1, CommandLog: "c1 loop ..."},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		Diff:           "diff --git a/src/main_test.go b/src/main_test.go\n@@ -1 +1 @@\n-old\n+new",
		InitialVerdict: "PARTIAL_FAILURE",
	})

	if !r.Verified {
		t.Errorf("expected Verified=true for genuine partial failure, got false. Inconsistencies=%v", r.Inconsistencies)
	}
	if r.FinalVerdict != "PARTIAL_FAILURE" {
		t.Errorf("expected FinalVerdict=PARTIAL_FAILURE, got %s", r.FinalVerdict)
	}
}

func TestPinocchioFalsePartialFailureDowngrades(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		Diff:           "",
		InitialVerdict: "PARTIAL_FAILURE",
	})

	if r.Verified {
		t.Error("expected Verified=false for partial failure without evidence")
	}
	if len(r.Inconsistencies) == 0 {
		t.Error("expected inconsistencies for false partial failure")
	}
}

func TestPinocchioNoopWithNonzeroExitDowngrades(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 1},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		InitialVerdict: "NOOP_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false for noop with nonzero exit")
	}
	if len(r.FalseClaims) == 0 {
		t.Error("expected FalseClaims for nonzero exit")
	}
	if r.FinalVerdict != "NEEDS_HUMAN" {
		t.Errorf("expected FinalVerdict=NEEDS_HUMAN, got %s", r.FinalVerdict)
	}
}

// --- Phase 0.4.1: Forced Verdict Override Negative Control ---

func TestPinocchioOverridesForcedSuccessWhenTestsFail(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		Diff:           "diff --git a/README.md b/README.md\n@@ -0,0 +1 @@\n+change",
		TestOutput:     "--- FAIL: TestAdd (0.01s)\nFAIL\nFAIL\texample\t0.5s",
		TaskMeta:       &receipt.TaskMeta{Commands: []string{"go test ./..."}},
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false when SUCCESS forced but tests fail")
	}
	if r.RecommendedVerdict == "SUCCESS_WITH_RECEIPT" {
		t.Error("RecommendedVerdict must not be SUCCESS when tests fail")
	}
	if r.FinalVerdict == "SUCCESS_WITH_RECEIPT" {
		t.Error("FinalVerdict must not be SUCCESS when tests fail")
	}
	found := false
	for _, fc := range r.FalseClaims {
		if fc == "SUCCESS claimed but test output indicates failure" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected false claim about test failure, got FalseClaims=%v", r.FalseClaims)
	}

	t.Logf("FinalVerdict=%s RecommendedVerdict=%s FalseClaims=%v",
		r.FinalVerdict, r.RecommendedVerdict, r.FalseClaims)
}

func TestPinocchioOverridesForcedSuccessWhenScanFails(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		SafetyHits:     []string{"forbidden file modified: secret.key"},
		InitialVerdict: "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false when SUCCESS forced but scan fails")
	}
	if r.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("expected FinalVerdict=FAILED_SAFETY, got %s", r.FinalVerdict)
	}
	if len(r.SafetyFindings) == 0 {
		t.Error("expected SafetyFindings non-empty for secret/key hit")
	}

	t.Logf("FinalVerdict=%s SafetyFindings=%v", r.FinalVerdict, r.SafetyFindings)
}

func TestPinocchioOverridesForcedSuccessWhenC1ReceiptFails(t *testing.T) {
	dir := t.TempDir()
	receiptPath := filepath.Join(dir, "c1-receipt.json")
	os.WriteFile(receiptPath, []byte(`{"status":"completed","exit_code":1}`), 0644)

	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:            &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:        runner.ModeReal,
		WorktreePath:      dir,
		C1LoopReceiptPath: receiptPath,
		InitialVerdict:    "SUCCESS_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false when SUCCESS forced but C1 receipt exits non-zero")
	}
	if r.FinalVerdict == "SUCCESS_WITH_RECEIPT" {
		t.Error("FinalVerdict must not be SUCCESS when C1 receipt is inconsistent")
	}
	if len(r.Inconsistencies) == 0 {
		t.Error("expected inconsistencies when C1 receipt exit code mismatches SUCCESS")
	}
	foundInconsistency := false
	for _, inc := range r.Inconsistencies {
		if inc == "C1 receipt status=completed but exit_code=1" {
			foundInconsistency = true
			break
		}
	}
	if !foundInconsistency {
		t.Errorf("expected C1 receipt exit_code inconsistency, got Inconsistencies=%v", r.Inconsistencies)
	}

	t.Logf("FinalVerdict=%s Inconsistencies=%v", r.FinalVerdict, r.Inconsistencies)
}

func TestPinocchioOverridesForcedNoopWhenDiffExists(t *testing.T) {
	r := VerifyForgeReceipt(&VerificationArtifacts{
		Result:         &runner.C1Result{ExitCode: 0, CommandLog: "c1 loop ..."},
		RunnerMode:     runner.ModeReal,
		WorktreePath:   "/tmp/worktree",
		Diff:           "diff --git a/README.md b/README.md\nnew file mode 100644\n--- /dev/null\n+++ b/README.md\n@@ -0,0 +1 @@\n+hello\n",
		InitialVerdict: "NOOP_WITH_RECEIPT",
	})

	if r.Verified {
		t.Error("expected Verified=false when NOOP forced but diff exists")
	}
	if r.FinalVerdict == "NOOP_WITH_RECEIPT" {
		t.Error("FinalVerdict must not be NOOP when diff exists")
	}
	found := false
	for _, fc := range r.FalseClaims {
		if fc == "NOOP claimed but files_changed = 1" || fc == "NOOP claimed but patch_lines = 2" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected false claim about diff/NOOP mismatch, got FalseClaims=%v", r.FalseClaims)
	}

	t.Logf("FinalVerdict=%s FalseClaims=%v", r.FinalVerdict, r.FalseClaims)
}
