package pipeline

import (
	"fmt"
	"strings"

	"github.com/desmondkam/openselo/internal/governor"
	"github.com/desmondkam/openselo/internal/runner"
	"github.com/desmondkam/openselo/internal/testintegrity"
)

// NOTE: POST-EXECUTION AUDIT ONLY — checks run on Diff/TestOutput after the
// agent has executed. This is not inline interception of agent syscalls.
//
// Check is a safety gate that runs against a task execution.
type Check interface {
	Name() string
	Run(ctx *CheckContext) *CheckResult
}

// CheckContext bundles all inputs a safety check needs.
type CheckContext struct {
	Diff            string
	TestOutput      string
	WorktreePath    string
	Commands        []string
	AllowedFiles    []string
	ForbiddenFiles  []string
	ForbiddenClaims []string
	Governor        *governor.Governor
	AllowTestMods   bool
	TestInvBefore   *testintegrity.TestInventory
	RunnerMode      runner.RunnerMode
	Verdict         string
}

// CheckResult captures what a single check found.
type CheckResult struct {
	Hits      []string
	Violation bool
}

// RunAll executes all checks and returns aggregated hits and any override verdict.
func RunAll(checks []Check, ctx *CheckContext) ([]string, string) {
	var allHits []string
	var overrideVerdict string

	for _, check := range checks {
		result := check.Run(ctx)
		if result == nil {
			continue
		}
		allHits = append(allHits, result.Hits...)
		if result.Violation && overrideVerdict == "" {
			overrideVerdict = check.Name()
		}
	}
	return allHits, overrideVerdict
}

// --- Individual checks ---

// ForbiddenFileEditCheck verifies no disallowed files were modified.
type ForbiddenFileEditCheck struct{}

func (ForbiddenFileEditCheck) Name() string { return "forbidden_file_edit" }
func (ForbiddenFileEditCheck) Run(ctx *CheckContext) *CheckResult {
	violation, msg := runner.CheckForbiddenFileEdit(ctx.Diff, ctx.WorktreePath, ctx.AllowedFiles, ctx.ForbiddenFiles)
	if violation {
		return &CheckResult{Hits: []string{msg}, Violation: true}
	}
	return nil
}

// ForbiddenClaimsCheck scans for forbidden claim terms.
type ForbiddenClaimsCheck struct{}

func (ForbiddenClaimsCheck) Name() string { return "forbidden_claims" }
func (ForbiddenClaimsCheck) Run(ctx *CheckContext) *CheckResult {
	hits, _ := runner.RunForbiddenClaimsScan(ctx.WorktreePath, ctx.ForbiddenClaims)
	if len(hits) > 0 {
		return &CheckResult{Hits: hits}
	}
	return nil
}

// SecretScanCheck scans for secrets in the worktree.
type SecretScanCheck struct{}

func (SecretScanCheck) Name() string { return "secret_scan" }
func (SecretScanCheck) Run(ctx *CheckContext) *CheckResult {
	hits, _ := runner.RunSecretScan(ctx.WorktreePath)
	if len(hits) > 0 {
		return &CheckResult{Hits: hits}
	}
	return nil
}

// PatchLimitCheck verifies patch size constraints.
type PatchLimitCheck struct{}

func (PatchLimitCheck) Name() string { return "patch_limit" }
func (PatchLimitCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx.Governor == nil {
		return nil
	}
	violation, msg := ctx.Governor.CheckPatchLimits(ctx.Diff)
	if violation {
		return &CheckResult{Hits: []string{msg}, Violation: true}
	}
	return nil
}

// RoundLimitCheck verifies round count constraints.
type RoundLimitCheck struct{}

func (RoundLimitCheck) Name() string { return "round_limit" }
func (RoundLimitCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx.Governor == nil {
		return nil
	}
	if ctx.Governor.CheckRoundsExceeded(0) {
		return &CheckResult{Hits: []string{"max rounds exceeded"}, Violation: true}
	}
	return nil
}

// TestIntegrityCheck verifies test files weren't improperly modified.
type TestIntegrityCheck struct{}

func (TestIntegrityCheck) Name() string { return "test_integrity" }
func (TestIntegrityCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx.TestInvBefore == nil {
		return nil
	}
	after := testintegrity.CaptureInventory(ctx.WorktreePath, ctx.Commands)
	tiResult := testintegrity.Analyze(ctx.TestInvBefore, after, ctx.Diff, ctx.AllowTestMods)
	if !tiResult.Passed || tiResult.RecommendedVerdict != "" {
		msg := fmt.Sprintf("test integrity violation: %d tests removed, %d tests modified, %d commands changed",
			len(tiResult.TestsRemoved), len(tiResult.TestsModified), len(tiResult.TestCommandsChanged))
		return &CheckResult{Hits: []string{msg}, Violation: true}
	}
	return nil
}

// DefaultChecks returns the standard safety pipeline.
func DefaultChecks() []Check {
	return []Check{
		ForbiddenFileEditCheck{},
		ForbiddenClaimsCheck{},
		SecretScanCheck{},
		PatchLimitCheck{},
		RoundLimitCheck{},
		TestIntegrityCheck{},
	}
}

// FormatHits joins hit messages for display.
func FormatHits(hits []string) string {
	return strings.Join(hits, "; ")
}
