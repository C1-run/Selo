package p45

import (
	"fmt"
	"io"
	"strings"
)

type P45Slip struct {
	RunID              string   `json:"run_id"`
	Goal               string   `json:"goal"`
	Scope              string   `json:"scope"`
	PatchBudget        string   `json:"patch_budget"`
	ChangedFiles       []string `json:"changed_files"`
	Violation          string   `json:"violation"`
	Warnings           []string `json:"warnings"`
	Verdict            string   `json:"verdict"`
	Reason             string   `json:"reason"`
	NextAllowedAction  string   `json:"next_allowed_action"`
	ForbiddenNextAction string `json:"forbidden_next_action"`
}

var slipDisclaimers = []string{
	"STOP_RECEIPT is an end-of-run record, not a release gate.",
	"This run does not prove semantic safety of AI-generated code.",
	"This run does not certify code correctness or security.",
	"This run does not stop agents automatically; external enforcement is required.",
	"This run does not prove production readiness.",
}

func WriteSlip(w io.Writer, s *P45Slip) error {
	var b strings.Builder

	b.WriteString("# P45 Slip\n\n")
	b.WriteString(fmt.Sprintf("**run_id**: %s\n", s.RunID))
	b.WriteString(fmt.Sprintf("**goal**: %s\n", s.Goal))
	b.WriteString(fmt.Sprintf("**scope**: %s\n", s.Scope))
	b.WriteString(fmt.Sprintf("**patch_budget**: %s\n", s.PatchBudget))
	b.WriteString("\n")

	b.WriteString("## Changes\n")
	b.WriteString(fmt.Sprintf("**files_changed**: %d\n", len(s.ChangedFiles)))
	for _, f := range s.ChangedFiles {
		b.WriteString(fmt.Sprintf("- %s\n", f))
	}
	b.WriteString("\n")

	b.WriteString("## Violation\n")
	b.WriteString(fmt.Sprintf("%s\n\n", s.Violation))

	if len(s.Warnings) > 0 {
		b.WriteString("## Warnings\n")
		for _, w := range s.Warnings {
			b.WriteString(fmt.Sprintf("- %s\n", w))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Result\n")
	b.WriteString(fmt.Sprintf("**verdict**: %s\n", s.Verdict))
	b.WriteString(fmt.Sprintf("**reason**: %s\n\n", s.Reason))

	b.WriteString("## Next\n")
	b.WriteString(fmt.Sprintf("**next_allowed_action**: %s\n", s.NextAllowedAction))
	b.WriteString(fmt.Sprintf("**forbidden_next_action**: %s\n\n", s.ForbiddenNextAction))

	b.WriteString("---\n\n")
	b.WriteString("## Required disclaimers\n\n")
	for _, d := range slipDisclaimers {
		b.WriteString(fmt.Sprintf("- %s\n", d))
	}
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}
