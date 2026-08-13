package gatechain

import (
	"testing"
)

func step(id, label string, status StepStatus, risk RiskLevel, cov CoverageState) ChainStep {
	return ChainStep{
		ID:              id,
		Label:           label,
		Status:          status,
		Coverage:        cov,
		Risk:            risk,
		ReasonCode:      "test_" + id,
		EvidenceRef:     "ref://test/" + id,
		RawTextIncluded: false,
		SecretsIncluded: false,
		Adapter:         "test",
	}
}

func TestValidateStep(t *testing.T) {
	tests := []struct {
		name string
		step ChainStep
		want string
	}{
		{"valid pass step", step("p1", "pass", StatusPass, RiskLow, CovCovered), ""},
		{"valid warn step", step("w1", "warn", StatusWarn, RiskMedium, CovPartial), ""},
		{"valid block step", step("b1", "block", StatusBlock, RiskBlocked, CovBlocked), ""},
		{"valid skip step", step("s1", "skip", StatusSkip, RiskLow, CovNotChecked), ""},
		{"empty id", ChainStep{ID: "", Label: "x", Status: StatusPass, Coverage: CovCovered, Risk: RiskLow, ReasonCode: "r", EvidenceRef: "e"}, "step.id must be non-empty"},
		{"empty label", ChainStep{ID: "x", Label: "", Status: StatusPass, Coverage: CovCovered, Risk: RiskLow, ReasonCode: "r", EvidenceRef: "e"}, "step.label must be non-empty"},
		{"invalid status", ChainStep{ID: "x", Label: "x", Status: "invalid", Coverage: CovCovered, Risk: RiskLow, ReasonCode: "r", EvidenceRef: "e"}, "step.status must be one of"},
		{"nil step", ChainStep{}, "step.id must be non-empty"},
		{"raw_text_included true", ChainStep{ID: "x", Label: "x", Status: StatusPass, Coverage: CovCovered, Risk: RiskLow, ReasonCode: "r", EvidenceRef: "e", RawTextIncluded: true}, "step.raw_text_included must be false"},
		{"secrets_included true", ChainStep{ID: "x", Label: "x", Status: StatusPass, Coverage: CovCovered, Risk: RiskLow, ReasonCode: "r", EvidenceRef: "e", SecretsIncluded: true}, "step.secrets_included must be false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStep(&tt.step)
			if tt.want == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.want != "" && (err == nil || !containsSubstring(err.Error(), tt.want)) {
				t.Errorf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSummarizeEmpty(t *testing.T) {
	s, err := Summarize(nil)
	if err == nil {
		t.Fatal("expected error for nil steps")
	}
	if s != nil {
		t.Fatal("expected nil summary for nil steps")
	}
}

func TestSummarizeSinglePass(t *testing.T) {
	steps := []ChainStep{step("p1", "pass", StatusPass, RiskLow, CovCovered)}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.FinalStatus != StatusPass {
		t.Errorf("final_status = %s, want pass", s.FinalStatus)
	}
	if s.StopRequired {
		t.Error("stop_required should be false")
	}
	if s.ReviewRequired {
		t.Error("review_required should be false")
	}
	if s.PassCount != 1 {
		t.Errorf("pass_count = %d, want 1", s.PassCount)
	}
}

func TestSummarizeSingleBlock(t *testing.T) {
	steps := []ChainStep{step("b1", "block", StatusBlock, RiskBlocked, CovBlocked)}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.FinalStatus != StatusBlock {
		t.Errorf("final_status = %s, want block", s.FinalStatus)
	}
	if !s.StopRequired {
		t.Error("stop_required should be true")
	}
	if !s.ReviewRequired {
		t.Error("review_required should be true")
	}
}

func TestSummarizeSingleWarn(t *testing.T) {
	steps := []ChainStep{step("w1", "warn", StatusWarn, RiskMedium, CovPartial)}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.FinalStatus != StatusWarn {
		t.Errorf("final_status = %s, want warn", s.FinalStatus)
	}
	if s.StopRequired {
		t.Error("stop_required should be false")
	}
	if !s.ReviewRequired {
		t.Error("review_required should be true")
	}
}

func TestSummarizeBlockOverridesWarn(t *testing.T) {
	steps := []ChainStep{
		step("w1", "warn", StatusWarn, RiskMedium, CovPartial),
		step("b1", "block", StatusBlock, RiskBlocked, CovBlocked),
	}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.FinalStatus != StatusBlock {
		t.Errorf("final_status = %s, want block (block overrides warn)", s.FinalStatus)
	}
	if !s.StopRequired {
		t.Error("stop_required should be true")
	}
}

// Validates that ValidateSummary checks for SEMANTIC_SPEC R1/R2/R3 constraints
func TestValidateSummaryR1R2R3(t *testing.T) {
	tests := []struct {
		name    string
		summary *ChainSummary
		wantErr string
	}{
		{"nil", nil, "must not be nil"},
		{"valid pass", &ChainSummary{FinalStatus: StatusPass, HighestRisk: RiskLow, Coverage: CovCovered, StepCount: 1}, ""},
		{"R1: block + stop=false", &ChainSummary{FinalStatus: StatusBlock, HighestRisk: RiskBlocked, Coverage: CovBlocked, StepCount: 1, StopRequired: false}, "R1"},
		{"R2: block + review=false", &ChainSummary{FinalStatus: StatusBlock, HighestRisk: RiskBlocked, Coverage: CovBlocked, StepCount: 1, StopRequired: true, ReviewRequired: false}, "R2"},
		{"R3: warn + review=false", &ChainSummary{FinalStatus: StatusWarn, HighestRisk: RiskMedium, Coverage: CovPartial, StepCount: 1, ReviewRequired: false}, "R3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSummary(tt.summary)
			if tt.wantErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !containsStr(err.Error(), tt.wantErr)) {
				t.Errorf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// Tests SEMANTIC_SPEC Decision Priority (§4.1) + 3 rules
func TestConsumeDecisionThreeRules(t *testing.T) {
	tests := []struct {
		name    string
		summary *ChainSummary
		want    Action
	}{
		{
			name: "Rule 1: stop_required=true blocks pass",
			summary: &ChainSummary{
				FinalStatus: StatusPass, HighestRisk: RiskBlocked, Coverage: CovBlocked,
				StopRequired: true, ReviewRequired: true,
			},
			want: ActionStop,
		},
		{
			name: "Rule 1: stop_required=true blocks warn",
			summary: &ChainSummary{
				FinalStatus: StatusWarn, HighestRisk: RiskBlocked, Coverage: CovBlocked,
				StopRequired: true, ReviewRequired: true,
			},
			want: ActionStop,
		},
		{
			name: "Rule 1: stop_required=true blocks block",
			summary: &ChainSummary{
				FinalStatus: StatusBlock, HighestRisk: RiskBlocked, Coverage: CovBlocked,
				StopRequired: true, ReviewRequired: true,
			},
			want: ActionStop,
		},
		{
			name: "Rule 2: review_required=true produces review",
			summary: &ChainSummary{
				FinalStatus: StatusPass, HighestRisk: RiskHigh, Coverage: CovCovered,
				StopRequired: false, ReviewRequired: true,
			},
			want: ActionReview,
		},
		{
			name: "Rule 2: warn without stop produces review",
			summary: &ChainSummary{
				FinalStatus: StatusWarn, HighestRisk: RiskMedium, Coverage: CovPartial,
				StopRequired: false, ReviewRequired: true,
			},
			want: ActionReview,
		},
		{
			name: "Rule 3: pass without stop/review produces pass",
			summary: &ChainSummary{
				FinalStatus: StatusPass, HighestRisk: RiskLow, Coverage: CovCovered,
				StopRequired: false, ReviewRequired: false,
			},
			want: ActionPass,
		},
		{
			name: "nil summary defaults to stop (fail-closed)",
			summary: nil,
			want: ActionStop,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ConsumeDecision(tt.summary)
			if v.Action != tt.want {
				t.Errorf("action = %s, want %s\nreason: %s", v.Action, tt.want, v.ActionReason)
			}
		})
	}
}

func TestVerifyDecision(t *testing.T) {
	tests := []struct {
		name    string
		verdict *AggregateVerdict
		wantErr string
	}{
		{"nil verdict", nil, "must not be nil"},
		{"pass ok", &AggregateVerdict{Action: ActionPass, FinalStatus: StatusPass}, ""},
		{"review ok", &AggregateVerdict{Action: ActionReview, ReviewRequired: true}, ""},
		{"stop ok", &AggregateVerdict{Action: ActionStop, StopRequired: true}, ""},
		{"stop_required=true must be stop",
			&AggregateVerdict{StopRequired: true, Action: ActionPass}, "stop_required=true must produce action=stop"},
		{"review_required=true must not be pass",
			&AggregateVerdict{ReviewRequired: true, StopRequired: false, Action: ActionPass}, "review_required=true without stop_required=true must not produce action=pass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyDecision(tt.verdict)
			if tt.wantErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !containsStr(err.Error(), tt.wantErr)) {
				t.Errorf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// === SEMANTIC_SPEC Mixed State Tests ===

func TestMixedTypeA_warnPlusStopRequired(t *testing.T) {
	cases := []struct {
		name  string
		steps []ChainStep
	}{
		{"A1: blocked risk, covered", []ChainStep{step("w1", "warn", StatusWarn, RiskBlocked, CovCovered)}},
		{"A2: blocked risk, not_checked", []ChainStep{step("w1", "warn", StatusWarn, RiskBlocked, CovNotChecked)}},
		{"A3: blocked risk, partial", []ChainStep{step("w1", "warn", StatusWarn, RiskBlocked, CovPartial)}},
		{"A4: blocked risk, blocked cov", []ChainStep{step("w1", "warn", StatusWarn, RiskBlocked, CovBlocked)}},
		{"A5: blocked coverage, low risk", []ChainStep{step("w1", "warn", StatusWarn, RiskLow, CovBlocked)}},
		{"A6: blocked coverage, medium risk", []ChainStep{step("w1", "warn", StatusWarn, RiskMedium, CovBlocked)}},
		{"A7: blocked coverage, high risk", []ChainStep{step("w1", "warn", StatusWarn, RiskHigh, CovBlocked)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Summarize(c.steps)
			if err != nil {
				t.Fatalf("Summarize failed: %v", err)
			}
			if s.FinalStatus != StatusWarn {
				t.Errorf("final_status=%s, want warn (Type A)", s.FinalStatus)
			}
			if !s.StopRequired {
				t.Error("stop_required must be true (Type A: warn + blocked component)")
			}
			v := ConsumeDecision(s)
			if v.Action != ActionStop {
				t.Errorf("action=%s, want stop (warn+stop=true must upgrade to block per SEMANTIC_SPEC §4.3)", v.Action)
			}
		})
	}
}

func TestMixedTypeB_passPlusReviewRequired(t *testing.T) {
	cases := []struct {
		name  string
		steps []ChainStep
	}{
		{"B1: high risk, covered", []ChainStep{step("p1", "pass", StatusPass, RiskHigh, CovCovered)}},
		{"B2: high risk, not_checked", []ChainStep{step("p1", "pass", StatusPass, RiskHigh, CovNotChecked)}},
		{"B3: high risk, partial", []ChainStep{step("p1", "pass", StatusPass, RiskHigh, CovPartial)}},
		{"B4: partial coverage, low risk", []ChainStep{step("p1", "pass", StatusPass, RiskLow, CovPartial)}},
		{"B5: partial coverage, medium risk", []ChainStep{step("p1", "pass", StatusPass, RiskMedium, CovPartial)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Summarize(c.steps)
			if err != nil {
				t.Fatalf("Summarize failed: %v", err)
			}
			if s.FinalStatus != StatusPass {
				t.Errorf("final_status=%s, want pass (Type B)", s.FinalStatus)
			}
			if !s.ReviewRequired {
				t.Error("review_required must be true (Type B: pass + high risk or partial coverage)")
			}
			if s.StopRequired {
				t.Error("stop_required must be false (Type B)")
			}
			v := ConsumeDecision(s)
			if v.Action != ActionReview {
				t.Errorf("action=%s, want review (pass+review=true must produce review per SEMANTIC_SPEC §4.4)", v.Action)
			}
		})
	}
}

func TestMixedTypeC_passPlusStopRequired(t *testing.T) {
	cases := []struct {
		name  string
		steps []ChainStep
	}{
		{"C1: blocked risk, covered", []ChainStep{step("p1", "pass", StatusPass, RiskBlocked, CovCovered)}},
		{"C2: blocked risk, not_checked", []ChainStep{step("p1", "pass", StatusPass, RiskBlocked, CovNotChecked)}},
		{"C3: blocked risk, partial", []ChainStep{step("p1", "pass", StatusPass, RiskBlocked, CovPartial)}},
		{"C4: blocked coverage, low risk", []ChainStep{step("p1", "pass", StatusPass, RiskLow, CovBlocked)}},
		{"C5: blocked coverage, medium risk", []ChainStep{step("p1", "pass", StatusPass, RiskMedium, CovBlocked)}},
		{"C6: blocked coverage, high risk", []ChainStep{step("p1", "pass", StatusPass, RiskHigh, CovBlocked)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Summarize(c.steps)
			if err != nil {
				t.Fatalf("Summarize failed: %v", err)
			}
			if s.FinalStatus != StatusPass {
				t.Errorf("final_status=%s, want pass (Type C)", s.FinalStatus)
			}
			if !s.StopRequired {
				t.Error("stop_required must be true (Type C: pass + blocked component)")
			}
			v := ConsumeDecision(s)
			if v.Action != ActionStop {
				t.Errorf("action=%s, want stop (pass+stop=true must produce stop per SEMANTIC_SPEC §4.1 Rule 1)", v.Action)
			}
		})
	}
}

func TestForbiddenFields(t *testing.T) {
	fields := []string{"raw_text", "raw_prompt", "raw_finding", "secret",
		"secret_value", "pnl", "position", "position_size", "leverage",
		"liquidation", "diff_content", "source_code"}
	for _, f := range fields {
		t.Run("forbidden_"+f, func(t *testing.T) {
			err := ValidateStepFields(map[string]any{f: "test"})
			if err == nil {
				t.Errorf("expected error for forbidden field: %s", f)
			}
		})
	}
}

func TestValidateStepFieldsClean(t *testing.T) {
	err := ValidateStepFields(map[string]any{"id": "x", "status": "pass"})
	if err != nil {
		t.Errorf("unexpected error on clean fields: %v", err)
	}
}

func TestMixedChain(t *testing.T) {
	steps := []ChainStep{
		step("p45", "P45 containment", StatusPass, RiskLow, CovCovered),
		step("ct", "CT scope check", StatusPass, RiskLow, CovCovered),
		step("redline", "Redline policy", StatusWarn, RiskMedium, CovPartial),
		step("tuttut", "tuttut exposure", StatusPass, RiskLow, CovCovered),
	}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	// Mixed chain: 1 warn + 3 pass => final=warn, review=T, stop=F
	if s.FinalStatus != StatusWarn {
		t.Errorf("final_status=%s, want warn", s.FinalStatus)
	}
	if !s.ReviewRequired {
		t.Error("review_required must be true (warn present)")
	}
	if s.StopRequired {
		t.Error("stop_required must be false (no block, no blocked component)")
	}
	v := ConsumeDecision(s)
	if v.Action != ActionReview {
		t.Errorf("action=%s, want review", v.Action)
	}
}

func TestBlockingChain(t *testing.T) {
	steps := []ChainStep{
		step("p45", "P45 stopped", StatusBlock, RiskBlocked, CovBlocked),
		step("ct", "CT clean", StatusPass, RiskLow, CovCovered),
	}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if s.FinalStatus != StatusBlock {
		t.Errorf("final_status=%s, want block", s.FinalStatus)
	}
	if !s.StopRequired {
		t.Error("stop_required must be true")
	}
	if !s.ReviewRequired {
		t.Error("review_required must be true (block present)")
	}
	v := ConsumeDecision(s)
	if v.Action != ActionStop {
		t.Errorf("action=%s, want stop", v.Action)
	}
}

func TestEmptySteps(t *testing.T) {
	s, err := Summarize([]ChainStep{})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if s.FinalStatus != StatusPass {
		t.Errorf("final_status=%s, want pass (empty = pass)", s.FinalStatus)
	}
	if s.StopRequired {
		t.Error("stop_required should be false for empty steps")
	}
	if s.ReviewRequired {
		t.Error("review_required should be false for empty steps")
	}
	if s.StepCount != 0 {
		t.Errorf("step_count=%d, want 0", s.StepCount)
	}
}

func TestAllPassButReviewRequired(t *testing.T) {
	steps := []ChainStep{
		step("p1", "pass", StatusPass, RiskHigh, CovCovered),
		step("p2", "pass", StatusPass, RiskLow, CovCovered),
	}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if s.FinalStatus != StatusPass {
		t.Errorf("final_status=%s, want pass", s.FinalStatus)
	}
	if !s.ReviewRequired {
		t.Error("review_required must be true (highest=high)")
	}
	if s.StopRequired {
		t.Error("stop_required must be false (no blocked component)")
	}
	v := ConsumeDecision(s)
	if v.Action != ActionReview {
		t.Errorf("action=%s, want review", v.Action)
	}
}

func TestAllPassButStopRequired(t *testing.T) {
	steps := []ChainStep{
		step("p1", "pass", StatusPass, RiskLow, CovBlocked),
		step("p2", "pass", StatusPass, RiskLow, CovCovered),
	}
	s, err := Summarize(steps)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if s.FinalStatus != StatusPass {
		t.Errorf("final_status=%s, want pass", s.FinalStatus)
	}
	if !s.StopRequired {
		t.Error("stop_required must be true (coverage=blocked)")
	}
	v := ConsumeDecision(s)
	if v.Action != ActionStop {
		t.Errorf("action=%s, want stop (pass+stop=true -> block per Rule 1)", v.Action)
	}
}

// Regression: VerifyDecision on valid verdicts
func TestVerifyDecisionValid(t *testing.T) {
	valid := []*AggregateVerdict{
		{Action: ActionPass, FinalStatus: StatusPass},
		{Action: ActionReview, ReviewRequired: true, FinalStatus: StatusPass},
		{Action: ActionStop, StopRequired: true, FinalStatus: StatusBlock},
		{Action: ActionStop, StopRequired: true, FinalStatus: StatusPass},
		{Action: ActionStop, StopRequired: true, FinalStatus: StatusWarn},
	}
	for _, v := range valid {
		if err := VerifyDecision(v); err != nil {
			t.Errorf("unexpected error for verdict action=%s stop=%v review=%v: %v",
				v.Action, v.StopRequired, v.ReviewRequired, err)
		}
	}
}
