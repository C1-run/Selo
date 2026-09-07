package gatechain

import (
	"fmt"
)

var riskOrder = map[RiskLevel]int{
	RiskLow:     0,
	RiskMedium:  1,
	RiskHigh:    2,
	RiskBlocked: 3,
}

var coverageOrder = map[CoverageState]int{
	CovCovered:    0,
	CovNotChecked: 1,
	CovPartial:    2,
	CovBlocked:    3,
}

func Summarize(steps []ChainStep) (*ChainSummary, error) {
	if steps == nil {
		return nil, fmt.Errorf("steps must not be nil")
	}

	var passCount, warnCount, blockCount, skipCount int
	var highestR RiskLevel = RiskLow
	var worstC CoverageState = CovCovered

	for i := range steps {
		if err := ValidateStep(&steps[i]); err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		switch steps[i].Status {
		case StatusPass:
			passCount++
		case StatusWarn:
			warnCount++
		case StatusBlock:
			blockCount++
		case StatusSkip:
			skipCount++
		}
		if riskOrder[steps[i].Risk] > riskOrder[highestR] {
			highestR = steps[i].Risk
		}
		if coverageOrder[steps[i].Coverage] > coverageOrder[worstC] {
			worstC = steps[i].Coverage
		}
	}

	finalStatus := computeFinalStatus(blockCount, warnCount)
	reviewRequired := warnCount > 0 || blockCount > 0 ||
		highestR == RiskHigh || highestR == RiskBlocked ||
		worstC == CovPartial || worstC == CovBlocked
	stopRequired := blockCount > 0 ||
		highestR == RiskBlocked ||
		worstC == CovBlocked

	var hasRaw, hasSecrets bool
	for _, s := range steps {
		if s.RawTextIncluded {
			hasRaw = true
		}
		if s.SecretsIncluded {
			hasSecrets = true
		}
	}

	return &ChainSummary{
		StepCount:       len(steps),
		PassCount:       passCount,
		WarnCount:       warnCount,
		BlockCount:      blockCount,
		SkipCount:       skipCount,
		HighestRisk:     highestR,
		FinalStatus:     finalStatus,
		Coverage:        worstC,
		ReviewRequired:  reviewRequired,
		StopRequired:    stopRequired,
		RawTextIncluded: hasRaw,
		SecretsIncluded: hasSecrets,
	}, nil
}

func computeFinalStatus(blockCount, warnCount int) StepStatus {
	switch {
	case blockCount > 0:
		return StatusBlock
	case warnCount > 0:
		return StatusWarn
	default:
		return StatusPass
	}
}

func ConsumeDecision(summary *ChainSummary) *AggregateVerdict {
	if summary == nil {
		return &AggregateVerdict{
			Action:       ActionStop,
			ActionReason: "nil summary — fail-closed: blocked",
		}
	}

	v := &AggregateVerdict{
		FinalStatus:    summary.FinalStatus,
		StopRequired:   summary.StopRequired,
		ReviewRequired: summary.ReviewRequired,
	}

	if summary.BlockCount > 0 {
		v.BlockingTools = append(v.BlockingTools, fmt.Sprintf("%d blocked step(s)", summary.BlockCount))
	}
	if summary.WarnCount > 0 {
		v.Warnings = append(v.Warnings, fmt.Sprintf("%d warning(s)", summary.WarnCount))
	}

	stopTools := summary.BlockCount
	if summary.HighestRisk == RiskBlocked {
		stopTools++
	}
	if summary.Coverage == CovBlocked {
		stopTools++
	}
	if stopTools > 0 {
		v.BlockingTools = append(v.BlockingTools, fmt.Sprintf("%d blocking signal(s)", stopTools))
	}

	// SEMANTIC_SPEC §4.1: Decision priority
	// Rule 1: stop_required=true → block, regardless of final_status
	// Rule 2: review_required=true (and stop_required=false) → review
	// Rule 3: else → pass (only for final_status=pass with no stop/review)
	switch {
	case summary.StopRequired:
		v.Action = ActionStop
		v.ActionReason = fmt.Sprintf("stop_required=true (final_status=%s, risk=%s, coverage=%s) — SEMANTIC_SPEC Rule 1: stop overrides pass/warn",
			summary.FinalStatus, summary.HighestRisk, summary.Coverage)
	case summary.ReviewRequired:
		v.Action = ActionReview
		v.ActionReason = fmt.Sprintf("review_required=true (final_status=%s, risk=%s, coverage=%s) — SEMANTIC_SPEC Rule 2: review required",
			summary.FinalStatus, summary.HighestRisk, summary.Coverage)
	default:
		v.Action = ActionPass
		v.ActionReason = fmt.Sprintf("final_status=%s, no stop/review required — SEMANTIC_SPEC Rule 3: pass",
			summary.FinalStatus)
	}

	return v
}

func ValidateSummary(summary *ChainSummary) error {
	if summary == nil {
		return fmt.Errorf("summary must not be nil")
	}
	if summary.StepCount < 0 {
		return fmt.Errorf("step_count must be >= 0")
	}
	if !contains(validStatuses, summary.FinalStatus) {
		return fmt.Errorf("final_status must be one of: pass, warn, block")
	}
	if !contains(validRisks, summary.HighestRisk) {
		return fmt.Errorf("highest_risk must be one of: low, medium, high, blocked")
	}
	if !contains(validCoverages, summary.Coverage) {
		return fmt.Errorf("coverage must be one of: not_checked, partial, covered, blocked")
	}
	// SEMANTIC_SPEC R1–R3 constraints
	if summary.FinalStatus == StatusBlock && !summary.StopRequired {
		return fmt.Errorf("final_status=block requires stop_required=true (SEMANTIC_SPEC R1)")
	}
	if summary.FinalStatus == StatusBlock && !summary.ReviewRequired {
		return fmt.Errorf("final_status=block requires review_required=true (SEMANTIC_SPEC R2)")
	}
	if summary.FinalStatus == StatusWarn && !summary.ReviewRequired {
		return fmt.Errorf("final_status=warn requires review_required=true (SEMANTIC_SPEC R3)")
	}
	return nil
}

func VerifyDecision(verdict *AggregateVerdict) error {
	if verdict == nil {
		return fmt.Errorf("verdict must not be nil")
	}
	switch verdict.Action {
	case ActionPass, ActionReview, ActionStop:
	default:
		return fmt.Errorf("action must be one of: pass, review, stop")
	}
	// SEMANTIC_SPEC §4.1: stop_required=true must always produce stop action
	if verdict.StopRequired && verdict.Action != ActionStop {
		return fmt.Errorf("stop_required=true must produce action=stop (SEMANTIC_SPEC §4.1)")
	}
	// SEMANTIC_SPEC §4.2: review_required=true with stop_required=false must produce review or stop
	if verdict.ReviewRequired && !verdict.StopRequired && verdict.Action == ActionPass {
		return fmt.Errorf("review_required=true without stop_required=true must not produce action=pass (SEMANTIC_SPEC §4.2)")
	}
	return nil
}
