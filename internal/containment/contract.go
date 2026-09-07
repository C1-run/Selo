package containment

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RunContract records the contractual bounds of a forge run.
// Inspired by Loop SAV's SavState (Status, StartTime, MaxRounds, StopReason)
// but replaces the 11-stage pipeline with deadline + patch-count record.
//
// PatchesAfterDeadline is a recorded historical fact, not a live clock check.
// PatchCount is the total patches made; MaxPatchCount is the limit.
type RunContract struct {
	Goal                 string `json:"goal"`
	DeadlineUTC          string `json:"deadline_utc,omitempty"`
	MaxPatchCount        int    `json:"max_patch_count"`
	PatchCount           int    `json:"patch_count"`
	PatchWindowClosedAt  string `json:"patch_window_closed_at,omitempty"`
	AuditWindowStartedAt string `json:"audit_window_started_at,omitempty"`
	ReceiptFinalizedAt   string `json:"receipt_finalized_at,omitempty"`
	PatchesAfterDeadline int    `json:"patches_after_deadline"`
}

// VerifyRunContract checks deadline compliance.
// Returns (true, "") if the contract is satisfied, (false, reason) if violated.
func VerifyRunContract(c *RunContract) (bool, string) {
	if c == nil {
		return false, "nil contract"
	}
	if c.DeadlineUTC == "" {
		return false, "deadline_utc is empty"
	}
	if c.PatchesAfterDeadline > 0 {
		return false, fmt.Sprintf("%d patch(es) after deadline", c.PatchesAfterDeadline)
	}
	if c.MaxPatchCount > 0 && c.PatchCount > c.MaxPatchCount {
		return false, fmt.Sprintf("%d patches exceed max %d", c.PatchCount, c.MaxPatchCount)
	}
	return true, ""
}

// ScopeContract records allowed and forbidden file paths for a run.
// Inspired by Loop SIEVE's FileBoundary (AllowedPatterns, ForbiddenPatterns, CheckFile).
//
// Path matching uses filepath.Match (standard Go glob), NOT Loop SIEVE's
// recursive ** matching. Patterns like "src/**/*.go" will not work as expected.
// Use simple single-level globs: "src/*.go", "docs/*.md".
type ScopeContract struct {
	AllowedPaths    []string `json:"allowed_paths,omitempty"`
	ForbiddenPaths  []string `json:"forbidden_paths,omitempty"`
	ChangedPaths    []string `json:"changed_paths,omitempty"`
	ScopeViolations []string `json:"scope_violations,omitempty"`
}

// VerifyScopeContract checks changed paths against the scope boundary.
// Returns (true, nil) if all paths are in scope, (false, violations) otherwise.
func VerifyScopeContract(c *ScopeContract) (bool, []string) {
	if c == nil {
		return false, []string{"nil contract"}
	}
	var violations []string
	for _, changed := range c.ChangedPaths {
		normalized := strings.TrimPrefix(changed, "./")
		for _, forbid := range c.ForbiddenPaths {
			matched, err := filepath.Match(forbid, normalized)
			if err == nil && matched {
				violations = append(violations,
					fmt.Sprintf("path %q matches forbidden pattern %q", changed, forbid))
				goto nextPath
			}
			if strings.Contains(normalized, forbid) {
				violations = append(violations,
					fmt.Sprintf("path %q contains forbidden pattern %q", changed, forbid))
				goto nextPath
			}
		}
		if len(c.AllowedPaths) > 0 {
			allowed := false
			for _, allow := range c.AllowedPaths {
				matched, err := filepath.Match(allow, normalized)
				if err == nil && matched {
					allowed = true
					break
				}
			}
			if !allowed {
				violations = append(violations,
					fmt.Sprintf("path %q not in allowed paths", changed))
			}
		}
	nextPath:
	}
	if len(violations) > 0 {
		c.ScopeViolations = violations
		return false, violations
	}
	return true, nil
}
