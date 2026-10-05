package gatechain

import (
	"encoding/json"
	"testing"

	"github.com/C1-run/selo/internal/receipt"
)

func baseReceipt() *receipt.ForgeReceipt {
	return &receipt.ForgeReceipt{
		TaskID:               "task-1",
		Verdict:              receipt.VerdictNoop,
		FinalVerdict:         receipt.VerdictNoop,
		PinocchioVerified:    true,
		TestIntegrityPassed:  true,
		ScansPassed:          true,
		RunnerBinaryVerified: true,
	}
}

func TestAdaptReceiptAllPass(t *testing.T) {
	steps, err := AdaptReceipt(baseReceipt())
	if err != nil {
		t.Fatalf("AdaptReceipt: %v", err)
	}
	if len(steps) != 5 {
		t.Fatalf("got %d steps, want 5", len(steps))
	}
	for _, s := range steps {
		if s.Status != StatusPass {
			t.Errorf("step %s status = %s, want pass", s.ID, s.Status)
		}
		if s.RawTextIncluded {
			t.Errorf("step %s raw_text_included must be false", s.ID)
		}
	}
}

func TestAdaptReceiptAllPassVerdict(t *testing.T) {
	v, err := VerifyAdaptation(baseReceipt())
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionPass {
		t.Errorf("action = %s, want pass", v.Action)
	}
	if v.StopRequired || v.ReviewRequired {
		t.Errorf("stop/review should be false, got stop=%v review=%v", v.StopRequired, v.ReviewRequired)
	}
}

func TestAdaptReceiptStopOnSafety(t *testing.T) {
	rec := baseReceipt()
	rec.ScansPassed = false
	rec.SafetyHits = []string{"forbidden file: src/secret.rs"}

	v, err := VerifyAdaptation(rec)
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionStop {
		t.Errorf("action = %s, want stop", v.Action)
	}
	if !v.StopRequired {
		t.Error("stop_required should be true")
	}
}

func TestAdaptReceiptReviewOnNeedsHuman(t *testing.T) {
	rec := baseReceipt()
	rec.FinalVerdict = receipt.VerdictNeedsHuman
	rec.Verdict = receipt.VerdictNeedsHuman

	v, err := VerifyAdaptation(rec)
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionReview {
		t.Errorf("action = %s, want review", v.Action)
	}
	if !v.ReviewRequired {
		t.Error("review_required should be true")
	}
	if v.StopRequired {
		t.Error("stop_required should be false")
	}
}

func TestAdaptReceiptWarnOnTestIntegrity(t *testing.T) {
	rec := baseReceipt()
	rec.TestIntegrityPassed = false
	rec.TestsRemoved = []string{"TestFoo"}

	v, err := VerifyAdaptation(rec)
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionReview {
		t.Errorf("action = %s, want review", v.Action)
	}
}

func TestAdaptReceiptWarnOnUnverifiedBinary(t *testing.T) {
	rec := baseReceipt()
	rec.RunnerBinaryVerified = false

	v, err := VerifyAdaptation(rec)
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionReview {
		t.Errorf("action = %s, want review", v.Action)
	}
}

func TestAdaptReceiptBlockOnInternalError(t *testing.T) {
	rec := baseReceipt()
	rec.FinalVerdict = receipt.VerdictInternalError
	rec.Verdict = receipt.VerdictInternalError

	v, err := VerifyAdaptation(rec)
	if err != nil {
		t.Fatalf("VerifyAdaptation: %v", err)
	}
	if v.Action != ActionStop {
		t.Errorf("action = %s, want stop", v.Action)
	}
}

func TestAdaptReceiptNil(t *testing.T) {
	if _, err := AdaptReceipt(nil); err == nil {
		t.Fatal("AdaptReceipt(nil) should error")
	}
}

func TestAdaptReceiptJSONRoundTrip(t *testing.T) {
	rec := baseReceipt()
	rec.ScansPassed = false
	rec.SafetyHits = []string{"secret found"}

	steps, err := AdaptReceipt(rec)
	if err != nil {
		t.Fatalf("AdaptReceipt: %v", err)
	}
	data, err := json.MarshalIndent(steps, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Must parse back into ChainStep (check CLI compatibility)
	var back []ChainStep
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != len(steps) {
		t.Errorf("round-trip length mismatch: %d vs %d", len(back), len(steps))
	}
	// Forbidden fields must not appear as JSON keys (raw_text_included/secrets_included are allowed)
	var parsed []map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal maps: %v", err)
	}
	for _, step := range parsed {
		for key := range step {
			for _, f := range forbiddenFields {
				if key == f {
					t.Errorf("forbidden field %q leaked into steps.json", f)
				}
			}
		}
	}
}