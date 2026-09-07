package gatechain

import (
	"fmt"

	"github.com/selo-dev/selo/internal/receipt"
)

// AdaptReceipt converts a completed ForgeReceipt into GateChain steps.
// Each gate in the receipt becomes one ChainStep; the final verdict step
// drives stop/block decisions. Returns a validation error if the receipt
// is nil.
func AdaptReceipt(rec *receipt.ForgeReceipt) ([]ChainStep, error) {
	if rec == nil {
		return nil, fmt.Errorf("receipt must not be nil")
	}

	steps := []ChainStep{}

	// Pinocchio consistency gate
	pino := ChainStep{
		ID:              "pinocchio",
		Label:           "Pinocchio: consistency verification",
		Status:          StatusPass,
		Coverage:        CovCovered,
		Risk:            RiskLow,
		ReasonCode:      "verified",
		EvidenceRef:     "receipt.pinocchio_verified",
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "pinocchio",
	}
	if !rec.PinocchioVerified {
		pino.Status = StatusBlock
		pino.Coverage = CovBlocked
		pino.Risk = RiskBlocked
		pino.ReasonCode = "not_verified"
		if len(rec.PinocchioInconsistencies) > 0 {
			pino.Label = fmt.Sprintf("Pinocchio: %d inconsistency(ies)", len(rec.PinocchioInconsistencies))
		}
	}
	steps = append(steps, pino)

	// Test integrity gate
	ti := ChainStep{
		ID:              "test-integrity",
		Label:           "Test integrity: inventory delta check",
		Status:          StatusPass,
		Coverage:        CovCovered,
		Risk:            RiskLow,
		ReasonCode:      "no_delta",
		EvidenceRef:     "receipt.test_integrity_passed",
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "test-integrity",
	}
	if !rec.TestIntegrityPassed {
		ti.Status = StatusWarn
		ti.Coverage = CovPartial
		ti.Risk = RiskHigh
		ti.ReasonCode = "tests_changed"
		removed := len(rec.TestsRemoved)
		modified := len(rec.TestsModified)
		commands := len(rec.TestCommandsChanged)
		ti.Label = fmt.Sprintf("Test integrity: %d removed, %d modified, %d commands changed", removed, modified, commands)
	}
	steps = append(steps, ti)

	// Safety gate
	safety := ChainStep{
		ID:              "safety",
		Label:           "Safety: forbidden file/claim/secret scan",
		Status:          StatusPass,
		Coverage:        CovCovered,
		Risk:            RiskLow,
		ReasonCode:      "scans_passed",
		EvidenceRef:     "receipt.scans_passed",
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "safety",
	}
	if !rec.ScansPassed || len(rec.SafetyHits) > 0 {
		safety.Status = StatusBlock
		safety.Coverage = CovBlocked
		safety.Risk = RiskBlocked
		safety.ReasonCode = "violation"
		safety.Label = fmt.Sprintf("Safety: %d hit(s)", len(rec.SafetyHits))
	}
	steps = append(steps, safety)

	// Binary trust gate
	binary := ChainStep{
		ID:              "binary-trust",
		Label:           "Binary trust: runner identity check",
		Status:          StatusPass,
		Coverage:        CovCovered,
		Risk:            RiskLow,
		ReasonCode:      "binary_verified",
		EvidenceRef:     "receipt.runner_binary_verified",
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "binary",
	}
	if !rec.RunnerBinaryVerified {
		binary.Status = StatusWarn
		binary.Coverage = CovPartial
		binary.Risk = RiskMedium
		binary.ReasonCode = "binary_unverified"
	}
	steps = append(steps, binary)

	// Final verdict gate (drives overall stop/block)
	final := ChainStep{
		ID:              "final-verdict",
		Label:           fmt.Sprintf("Runner verdict: %s", rec.FinalVerdict),
		Status:          verdictToStatus(rec.FinalVerdict),
		Coverage:        verdictToCoverage(rec.FinalVerdict),
		Risk:            verdictToRisk(rec.FinalVerdict),
		ReasonCode:      "verdict_mapped",
		EvidenceRef:     "receipt.final_verdict",
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "runner",
	}
	steps = append(steps, final)

	// Sanity check: every step must be valid
	for i := range steps {
		if err := ValidateStep(&steps[i]); err != nil {
			return nil, fmt.Errorf("adapted step %s invalid: %w", steps[i].ID, err)
		}
	}
	return steps, nil
}

func verdictToStatus(v string) StepStatus {
	switch v {
	case receipt.VerdictSuccess, receipt.VerdictNoop:
		return StatusPass
	case receipt.VerdictPartial, receipt.VerdictNeedsHuman, receipt.VerdictTimedOut:
		return StatusWarn
	case receipt.VerdictSafety, receipt.VerdictLimitExceeded, receipt.VerdictInternalError, receipt.VerdictStaleLock:
		return StatusBlock
	default:
		return StatusWarn
	}
}

func verdictToCoverage(v string) CoverageState {
	switch v {
	case receipt.VerdictSuccess, receipt.VerdictNoop:
		return CovCovered
	case receipt.VerdictPartial, receipt.VerdictNeedsHuman, receipt.VerdictTimedOut:
		return CovPartial
	default:
		return CovBlocked
	}
}

func verdictToRisk(v string) RiskLevel {
	switch v {
	case receipt.VerdictSuccess, receipt.VerdictNoop:
		return RiskLow
	case receipt.VerdictPartial, receipt.VerdictNeedsHuman, receipt.VerdictTimedOut:
		return RiskHigh
	default:
		return RiskBlocked
	}
}

// VerifyAdaptation runs the full gatechain pipeline over adapted steps.
// Returns the aggregate verdict, or an error if adaptation/validation fails.
func VerifyAdaptation(rec *receipt.ForgeReceipt) (*AggregateVerdict, error) {
	steps, err := AdaptReceipt(rec)
	if err != nil {
		return nil, err
	}
	summary, err := Summarize(steps)
	if err != nil {
		return nil, err
	}
	if err := ValidateSummary(summary); err != nil {
		return nil, err
	}
	verdict := ConsumeDecision(summary)
	if err := VerifyDecision(verdict); err != nil {
		return nil, err
	}
	return verdict, nil
}