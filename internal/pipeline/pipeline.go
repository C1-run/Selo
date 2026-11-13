package pipeline

import (
	"strings"

	"github.com/C1-run/selo/internal/governor"
	"github.com/C1-run/selo/internal/runner"
)

// NOTE: POST-EXECUTION AUDIT ONLY — checks run on the diff after the agent has
// executed. This is not inline interception of agent syscalls.
//
// ChecksVersion identifies the post-run check set the daemon executes (the
// checks here plus the daemon's supply-chain BOM and test-integrity gate,
// which need their full result objects and stay in the daemon flow). Bump it
// whenever the meaning of a check changes, so a receipt is never judged by
// check semantics it was not produced under. "1" is the v0.7.0 set: diff-scoped
// forbidden-claims and secret scans, permission_allowlist merged into the
// forbidden-file scope, exit-code-based command results.
const ChecksVersion = "1"

// Canonical check names, used in receipts (override verdicts) and to query a
// Report for a specific check's outcome.
const (
	CheckForbiddenFileEdit = "forbidden_file_edit"
	CheckForbiddenClaims   = "forbidden_claims"
	CheckSecretScan        = "secret_scan"
	CheckPatchLimit        = "patch_limit"
	CheckRoundLimit        = "round_limit"
)

// Check is a safety gate that runs against a task execution.
type Check interface {
	Name() string
	Run(ctx *CheckContext) *CheckResult
}

// CheckContext bundles all inputs a safety check needs.
type CheckContext struct {
	Diff            string
	WorktreePath    string
	AllowedFiles    []string
	ForbiddenFiles  []string
	ForbiddenClaims []string
	// ChangedFiles is the set of files the diff touched. The forbidden-claims
	// and secret scans are diff-scoped: scanning the whole worktree would
	// flag pre-existing content the agent never wrote.
	ChangedFiles []string
	Governor     *governor.Governor
}

// CheckResult captures what a single check found.
type CheckResult struct {
	Hits      []string
	Violation bool
}

// Report aggregates what a pipeline run found, per check and overall.
type Report struct {
	// Hits holds every hit in check order.
	Hits []string
	// ByCheck maps a check name to its result (nil when the check was skipped
	// or found nothing).
	ByCheck map[string]*CheckResult
	// Override is the name of the first violating check, "" when none.
	Override string
}

// Violated reports whether the named check found a violation.
func (r *Report) Violated(name string) bool {
	return r.ByCheck[name] != nil && r.ByCheck[name].Violation
}

// RunAll executes all checks in order and aggregates their results.
func RunAll(checks []Check, ctx *CheckContext) *Report {
	rep := &Report{ByCheck: map[string]*CheckResult{}}

	for _, check := range checks {
		result := check.Run(ctx)
		rep.ByCheck[check.Name()] = result
		if result == nil {
			continue
		}
		rep.Hits = append(rep.Hits, result.Hits...)
		if result.Violation && rep.Override == "" {
			rep.Override = check.Name()
		}
	}
	return rep
}

// --- Individual checks ---

// ForbiddenFileEditCheck verifies no disallowed files were modified.
type ForbiddenFileEditCheck struct{}

func (ForbiddenFileEditCheck) Name() string { return CheckForbiddenFileEdit }
func (ForbiddenFileEditCheck) Run(ctx *CheckContext) *CheckResult {
	violation, msg := runner.CheckForbiddenFileEdit(ctx.Diff, ctx.WorktreePath, ctx.AllowedFiles, ctx.ForbiddenFiles)
	if violation {
		return &CheckResult{Hits: []string{msg}, Violation: true}
	}
	return nil
}

// ForbiddenClaimsCheck scans the changed files for forbidden claim terms.
type ForbiddenClaimsCheck struct{}

func (ForbiddenClaimsCheck) Name() string { return CheckForbiddenClaims }
func (ForbiddenClaimsCheck) Run(ctx *CheckContext) *CheckResult {
	hits, _ := runner.RunForbiddenClaimsScanDiffScoped(ctx.WorktreePath, ctx.ForbiddenClaims, ctx.ChangedFiles)
	if len(hits) > 0 {
		return &CheckResult{Hits: hits}
	}
	return nil
}

// SecretScanCheck scans the changed files for secrets.
type SecretScanCheck struct{}

func (SecretScanCheck) Name() string { return CheckSecretScan }
func (SecretScanCheck) Run(ctx *CheckContext) *CheckResult {
	hits, _ := runner.RunSecretScanDiffScoped(ctx.WorktreePath, ctx.ChangedFiles)
	if len(hits) > 0 {
		return &CheckResult{Hits: hits}
	}
	return nil
}

// PatchLimitCheck verifies patch size constraints.
type PatchLimitCheck struct{}

func (PatchLimitCheck) Name() string { return CheckPatchLimit }
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

func (RoundLimitCheck) Name() string { return CheckRoundLimit }
func (RoundLimitCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx.Governor == nil {
		return nil
	}
	if ctx.Governor.CheckRoundsExceeded(0) {
		return &CheckResult{Hits: []string{"max rounds exceeded"}, Violation: true}
	}
	return nil
}

// DefaultChecks returns the standard post-diff safety pipeline. The daemon's
// supply-chain BOM and test-integrity gate are deliberately not here: they
// need their full result objects (OSV component list, test inventory diff)
// beyond a boolean, so they run in the daemon flow — but they are still part
// of what ChecksVersion "1" means.
func DefaultChecks() []Check {
	return []Check{
		ForbiddenFileEditCheck{},
		ForbiddenClaimsCheck{},
		SecretScanCheck{},
		PatchLimitCheck{},
		RoundLimitCheck{},
	}
}

// FormatHits joins hit messages for display.
func FormatHits(hits []string) string {
	return strings.Join(hits, "; ")
}
