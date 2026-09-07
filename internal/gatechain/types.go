package gatechain

import (
	"fmt"
)

type StepStatus string

const (
	StatusPass  StepStatus = "pass"
	StatusWarn  StepStatus = "warn"
	StatusBlock StepStatus = "block"
	StatusSkip  StepStatus = "skip"
)

type RiskLevel string

const (
	RiskLow     RiskLevel = "low"
	RiskMedium  RiskLevel = "medium"
	RiskHigh    RiskLevel = "high"
	RiskBlocked RiskLevel = "blocked"
)

type CoverageState string

const (
	CovCovered    CoverageState = "covered"
	CovNotChecked CoverageState = "not_checked"
	CovPartial    CoverageState = "partial"
	CovBlocked    CoverageState = "blocked"
)

type Action string

const (
	ActionPass   Action = "pass"
	ActionReview Action = "review"
	ActionStop   Action = "stop"
)

var (
	validStatuses   = []StepStatus{StatusPass, StatusWarn, StatusBlock, StatusSkip}
	validRisks      = []RiskLevel{RiskLow, RiskMedium, RiskHigh, RiskBlocked}
	validCoverages  = []CoverageState{CovCovered, CovNotChecked, CovPartial, CovBlocked}
	forbiddenFields = []string{"raw_text", "raw_prompt", "raw_finding", "secret",
		"secret_value", "pnl", "position", "position_size", "leverage",
		"liquidation", "diff_content", "source_code"}
)

func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func ValidateStepFields(fields map[string]any) error {
	for _, f := range forbiddenFields {
		if _, ok := fields[f]; ok {
			return fmt.Errorf("step contains forbidden field: %s", f)
		}
	}
	return nil
}

type ChainStep struct {
	ID              string        `json:"id"`
	Label           string        `json:"label"`
	Status          StepStatus    `json:"status"`
	Coverage        CoverageState `json:"coverage"`
	Risk            RiskLevel     `json:"risk"`
	ReasonCode      string        `json:"reason_code"`
	EvidenceRef     string        `json:"evidence_ref"`
	RawTextIncluded bool          `json:"raw_text_included"`
	SecretsIncluded bool          `json:"secrets_included"`
	Adapter         string        `json:"adapter,omitempty"`
}

func ValidateStep(s *ChainStep) error {
	if s == nil {
		return fmt.Errorf("step must not be nil")
	}
	if s.ID == "" {
		return fmt.Errorf("step.id must be non-empty")
	}
	if s.Label == "" {
		return fmt.Errorf("step.label must be non-empty")
	}
	if !contains(validStatuses, s.Status) {
		return fmt.Errorf("step.status must be one of: pass, warn, block, skip")
	}
	if !contains(validCoverages, s.Coverage) {
		return fmt.Errorf("step.coverage must be one of: not_checked, partial, covered, blocked")
	}
	if !contains(validRisks, s.Risk) {
		return fmt.Errorf("step.risk must be one of: low, medium, high, blocked")
	}
	if s.ReasonCode == "" {
		return fmt.Errorf("step.reason_code must be non-empty")
	}
	if s.EvidenceRef == "" {
		return fmt.Errorf("step.evidence_ref must be non-empty")
	}
	if s.RawTextIncluded {
		return fmt.Errorf("step.raw_text_included must be false")
	}
	if s.SecretsIncluded {
		return fmt.Errorf("step.secrets_included must be false")
	}
	return nil
}

type ChainSummary struct {
	StepCount       int           `json:"step_count"`
	PassCount       int           `json:"pass_count"`
	WarnCount       int           `json:"warn_count"`
	BlockCount      int           `json:"block_count"`
	SkipCount       int           `json:"skip_count"`
	HighestRisk     RiskLevel     `json:"highest_risk"`
	FinalStatus     StepStatus    `json:"final_status"`
	Coverage        CoverageState `json:"coverage"`
	ReviewRequired  bool          `json:"review_required"`
	StopRequired    bool          `json:"stop_required"`
	RawTextIncluded bool          `json:"raw_text_included"`
	SecretsIncluded bool          `json:"secrets_included"`
}

type AggregateVerdict struct {
	FinalStatus    StepStatus `json:"final_status"`
	StopRequired   bool       `json:"stop_required"`
	ReviewRequired bool       `json:"review_required"`
	BlockingTools  []string   `json:"blocking_tools,omitempty"`
	Warnings       []string   `json:"warnings,omitempty"`
	Action         Action     `json:"action"`
	ActionReason   string     `json:"action_reason"`
}
