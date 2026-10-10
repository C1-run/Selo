package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// TaskMeta holds parsed task metadata from task.md.
type TaskMeta struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Repo               string   `json:"repo"`
	Goal               string   `json:"goal"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Commands           []string `json:"commands,omitempty"`
	MaxMinutes         int      `json:"max_minutes"`
	MaxRounds          int      `json:"max_rounds"`
	Deliverables       []string `json:"deliverables,omitempty"`
}

// ForgeReceipt is the structured receipt for a task execution.
type ForgeReceipt struct {
	ReceiptID            string    `json:"receipt_id"`
	TaskID               string    `json:"task_id"`
	Verdict              string    `json:"verdict"`
	StartedAt            time.Time `json:"started_at"`
	FinishedAt           time.Time `json:"finished_at"`
	DurationSec          int64     `json:"duration_sec"`
	RoundsUsed           int       `json:"rounds_used"`
	ExitCode             int       `json:"exit_code"`
	TimedOut             bool      `json:"timed_out"`
	FilesChanged         int       `json:"files_changed"`
	PatchLines           int       `json:"patch_lines"`
	HasDiff              bool      `json:"has_diff"`
	TestOutput           string    `json:"test_output,omitempty"`
	DiffSummary          string    `json:"diff_summary,omitempty"`
	SafetyHits           []string  `json:"safety_hits,omitempty"`
	Error                string    `json:"error,omitempty"`
	RunnerMode           string    `json:"runner_mode,omitempty"`
	RunnerCommand        string    `json:"runner_command,omitempty"`
	RunnerExitCode       int       `json:"runner_exit_code,omitempty"`
	RunnerBinaryKind     string    `json:"runner_binary_kind,omitempty"`
	RunnerBinaryPath     string    `json:"runner_binary_path,omitempty"`
	RunnerBinaryVersion  string    `json:"runner_binary_version,omitempty"`
	RunnerBinaryVerified bool      `json:"runner_binary_verified"`
	C1RuntimeRequested   string    `json:"c1_runtime_requested,omitempty"`
	C1RuntimeUsed        string    `json:"c1_runtime_used,omitempty"`
	C1RuntimeIsMock      bool      `json:"c1_runtime_is_mock"`
	OpenCodeModel        string    `json:"opencode_model,omitempty"`
	OpenCodeAgent        string    `json:"opencode_agent,omitempty"`
	WorktreePath         string    `json:"worktree_path,omitempty"`
	BaseCommit           string    `json:"base_commit,omitempty"`
	// DiffHash is the sha256 (hex) of the full diff that was audited. It binds
	// the receipt to the exact change it judged, so a SUCCESS cannot be
	// replayed against a different diff. PolicyHash binds the effective rules
	// (forbidden claims/files, limits) so a SUCCESS under a strict policy is
	// distinguishable from one under an empty policy. SeloVersion records which
	// build produced the receipt. All three are covered by the signature.
	DiffHash                 string   `json:"diff_hash,omitempty"`
	PolicyHash               string   `json:"policy_hash,omitempty"`
	SeloVersion              string   `json:"selo_version,omitempty"`
	TestsPassed              int      `json:"tests_passed,omitempty"`
	ScansPassed              bool     `json:"scans_passed"`
	C1LoopReceiptPath        string   `json:"c1_loop_receipt_path,omitempty"`
	ForgeReceiptPath         string   `json:"forge_receipt_path,omitempty"`
	PinocchioVerified        bool     `json:"pinocchio_verified"`
	PinocchioResultPath      string   `json:"pinocchio_result_path,omitempty"`
	InitialVerdict           string   `json:"initial_verdict,omitempty"`
	FinalVerdict             string   `json:"final_verdict,omitempty"`
	VerdictOverridden        bool     `json:"verdict_overridden"`
	OverrideReason           string   `json:"override_reason,omitempty"`
	PinocchioFalseClaims     []string `json:"pinocchio_false_claims,omitempty"`
	PinocchioInconsistencies []string `json:"pinocchio_inconsistencies,omitempty"`
	TestIntegrityPassed      bool     `json:"test_integrity_passed"`
	TestIntegrityResultPath  string   `json:"test_integrity_result_path,omitempty"`
	TestsRemoved             []string `json:"tests_removed,omitempty"`
	TestsModified            []string `json:"tests_modified,omitempty"`
	TestCommandsChanged      []string `json:"test_commands_changed,omitempty"`
	TestInventoryBeforeCount int      `json:"test_inventory_before_count"`
	TestInventoryAfterCount  int      `json:"test_inventory_after_count"`
	OpenCodeRunInfoPath      string   `json:"opencode_run_info_path,omitempty"`
	OpenCodeTimedOut         bool     `json:"opencode_timed_out"`
	NotificationMode         string   `json:"notification_mode,omitempty"`
	NotificationSuccess      bool     `json:"notification_success"`
	NotificationError        string   `json:"notification_error,omitempty"`
	GateChainAction          string   `json:"gatechain_action,omitempty"`
	GateChainStopRequired    bool     `json:"gatechain_stop_required"`
	GateChainReviewRequired  bool     `json:"gatechain_review_required"`
	GateChainFinalStatus     string   `json:"gatechain_final_status,omitempty"`
	GateChainActionReason    string   `json:"gatechain_action_reason,omitempty"`
	Signature                string   `json:"signature,omitempty"`
	PublicKey                string   `json:"public_key,omitempty"`
	// KeyMode records whether the signing key was attributable across runs
	// ("persistent") or generated for this process only ("ephemeral"). It is
	// set by SignReceipt and cleared by CanonicalJSON like the other signature
	// fields. Downstream policy can refuse to accept ephemeral receipts.
	KeyMode string `json:"key_mode,omitempty"`
	// KeySource records where the signing key came from (env | file | keychain |
	// command | ephemeral). It is signed like key_mode. See ADR-008: only
	// "command" places the signer outside the audited agent's trust domain.
	KeySource    string     `json:"key_source,omitempty"`
	ReceiptHash  string     `json:"receipt_hash,omitempty"`
	AnchorCommit string     `json:"anchor_commit,omitempty"`
	AnchorBranch string     `json:"anchor_branch,omitempty"`
	AnchoredAt   *time.Time `json:"anchored_at,omitempty"`
	// Timestamp is an RFC3161 trusted timestamp over the canonical receipt
	// (ADR-005). Like the anchor fields it is produced after signing — it
	// attests to when the signature existed — and is therefore excluded from
	// CanonicalJSON. Its message imprint is the receipt_hash.
	Timestamp *TimestampAnchor `json:"timestamp,omitempty"`
	// Transparency is the receipt's Sigstore Rekor log record (ADR-006),
	// likewise produced after signing and excluded from CanonicalJSON. Only the
	// receipt hash is logged, so no task name or file path is published.
	Transparency     *TransparencyAnchor `json:"transparency,omitempty"`
	SupplyComponents []string            `json:"supply_components,omitempty"`
	SupplyHits       []string            `json:"supply_hits,omitempty"`
	SupplyCheckedAt  *time.Time          `json:"supply_checked_at,omitempty"`
}

// PolicySpec is the effective safety policy a receipt was produced under.
// Hashing it into the signed receipt means a SUCCESS cannot be confused with a
// SUCCESS produced under an empty or weaker policy.
type PolicySpec struct {
	ForbiddenClaims []string `json:"forbidden_claims,omitempty"`
	ForbiddenFiles  []string `json:"forbidden_files,omitempty"`
	AllowedFiles    []string `json:"allowed_files,omitempty"`
	MaxFiles        int      `json:"max_files,omitempty"`
	MaxPatchLines   int      `json:"max_patch_lines,omitempty"`
	MaxRounds       int      `json:"max_rounds,omitempty"`
}

// PolicyHash returns a stable sha256 (hex) over the effective policy. Slices
// are sorted on copies so the hash does not depend on declaration order.
func PolicyHash(p PolicySpec) string {
	sorted := PolicySpec{
		ForbiddenClaims: append([]string(nil), p.ForbiddenClaims...),
		ForbiddenFiles:  append([]string(nil), p.ForbiddenFiles...),
		AllowedFiles:    append([]string(nil), p.AllowedFiles...),
		MaxFiles:        p.MaxFiles,
		MaxPatchLines:   p.MaxPatchLines,
		MaxRounds:       p.MaxRounds,
	}
	sort.Strings(sorted.ForbiddenClaims)
	sort.Strings(sorted.ForbiddenFiles)
	sort.Strings(sorted.AllowedFiles)
	b, err := json.Marshal(sorted)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Verdict constants
const (
	VerdictSuccess       = "SUCCESS_WITH_RECEIPT"
	VerdictNoop          = "NOOP_WITH_RECEIPT"
	VerdictPartial       = "PARTIAL_FAILURE"
	VerdictNeedsHuman    = "NEEDS_HUMAN"
	VerdictTimedOut      = "FAILED_TIMEOUT"
	VerdictSafety        = "FAILED_SAFETY"
	VerdictLimitExceeded = "FAILED_LIMIT_EXCEEDED"
	VerdictStaleLock     = "FAILED_STALE_LOCK"
	VerdictInternalError = "FAILED_INTERNAL_ERROR"
)

// receiptIDMu guards lastReceiptID.
var (
	receiptIDMu   sync.Mutex
	lastReceiptID int64
)

// GenerateReceiptID returns a receipt id that is unique within the process.
//
// A bare timestamp is not enough. time.Now() is reported at microsecond
// granularity on macOS (measured: 1000 ns), so two calls in the same
// microsecond produced the same id -- 683 duplicates out of 1000 back-to-back
// calls. That is not cosmetic: the id is the receipt's file name
// (receipts/c1f-<id>.json), so a collision silently overwrites an earlier
// receipt instead of writing a new one.
//
// The format is unchanged; a call that lands in the same tick as the previous
// id steps past it, which also keeps ids strictly increasing.
func GenerateReceiptID() string {
	receiptIDMu.Lock()
	defer receiptIDMu.Unlock()

	now := time.Now().UnixNano()
	if now <= lastReceiptID {
		now = lastReceiptID + 1
	}
	lastReceiptID = now
	return fmt.Sprintf("c1f-%d", now)
}

// taskKeyRe matches the key of a "key: value" line.
var taskKeyRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):`)

// recognizedTaskKeys is every task.md key Selo honors (across ParseTaskMeta and
// governor.ParseTaskConfig). A "key:" line outside this set is almost always a
// typo, so it is warned about instead of silently ignored: a misspelled
// `max_minutes` used to leave the default in force with no signal at all.
var recognizedTaskKeys = map[string]bool{
	"id": true, "title": true, "repo": true, "goal": true,
	"acceptance_criteria": true, "commands": true, "commands_to_run": true,
	"deliverables": true, "max_minutes": true, "max_rounds": true,
	"max_files": true, "max_patch_lines": true,
	"allowed_files": true, "forbidden_files": true, "forbidden_claims": true,
	"allow_test_modifications": true,
}

func ParseTaskMeta(taskPath string) (*TaskMeta, error) {
	data, err := os.ReadFile(taskPath)
	if err != nil {
		return nil, fmt.Errorf("read task: %w", err)
	}
	meta := &TaskMeta{
		MaxMinutes: 30,
		MaxRounds:  3,
	}
	lines := strings.Split(string(data), "\n")
	inAC := false
	inCommands := false
	inDeliverables := false

	stripQuotes := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "\"")
		s = strings.Trim(s, "'")
		return s
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := taskKeyRe.FindStringSubmatch(trimmed); m != nil && !recognizedTaskKeys[m[1]] {
			fmt.Fprintf(os.Stderr, "[selo] warning: unknown task key %q in %s (ignored; check for a typo)\n", m[1], taskPath)
		}
		if strings.HasPrefix(trimmed, "id:") {
			meta.ID = stripQuotes(strings.TrimPrefix(trimmed, "id:"))
		} else if strings.HasPrefix(trimmed, "title:") {
			meta.Title = stripQuotes(strings.TrimPrefix(trimmed, "title:"))
		} else if strings.HasPrefix(trimmed, "repo:") {
			meta.Repo = stripQuotes(strings.TrimPrefix(trimmed, "repo:"))
		} else if strings.HasPrefix(trimmed, "goal:") {
			meta.Goal = stripQuotes(strings.TrimPrefix(trimmed, "goal:"))
		} else if strings.HasPrefix(trimmed, "acceptance_criteria:") {
			inAC = true
			inCommands = false
			inDeliverables = false
		} else if trimmed == "commands:" || trimmed == "commands_to_run:" {
			inCommands = true
			inAC = false
			inDeliverables = false
		} else if trimmed == "deliverables:" {
			inDeliverables = true
			inAC = false
			inCommands = false
		} else if strings.HasPrefix(trimmed, "max_minutes:") {
			fmt.Sscanf(trimmed, "max_minutes: %d", &meta.MaxMinutes)
			inAC = false
			inCommands = false
			inDeliverables = false
		} else if strings.HasPrefix(trimmed, "max_rounds:") {
			fmt.Sscanf(trimmed, "max_rounds: %d", &meta.MaxRounds)
			inAC = false
			inCommands = false
			inDeliverables = false
		} else if inAC && strings.HasPrefix(trimmed, "- ") {
			meta.AcceptanceCriteria = append(meta.AcceptanceCriteria, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inCommands && strings.HasPrefix(trimmed, "- ") {
			meta.Commands = append(meta.Commands, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inDeliverables && strings.HasPrefix(trimmed, "- ") {
			meta.Deliverables = append(meta.Deliverables, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if trimmed != "" && !strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "#") {
			return nil, fmt.Errorf("invalid task syntax at line: %q", trimmed)
		} else {
			inAC = false
			inCommands = false
			inDeliverables = false
		}
	}
	return meta, nil
}

// ReceiptWriter writes receipt.json and receipt.md files.
type ReceiptWriter struct {
	ReceiptsDir string
	RunsDir     string
}

// NewReceiptWriter creates a new receipt writer.
func NewReceiptWriter(receiptsDir, runsDir string) *ReceiptWriter {
	return &ReceiptWriter{ReceiptsDir: receiptsDir, RunsDir: runsDir}
}

// WriteReceipt writes both JSON and Markdown receipts. Every filesystem and
// marshal error is returned: a receipt that silently failed to persist is
// worse than no receipt, because the caller believes the evidence exists.
func (rw *ReceiptWriter) WriteReceipt(receipt *ForgeReceipt, taskMeta *TaskMeta, diff, testOutput string, safetyHits []string) error {
	if err := os.MkdirAll(rw.ReceiptsDir, 0755); err != nil {
		return fmt.Errorf("create receipts dir: %w", err)
	}
	if err := os.MkdirAll(rw.RunsDir, 0755); err != nil {
		return fmt.Errorf("create runs dir: %w", err)
	}

	receipt.HasDiff = diff != ""
	receipt.TestOutput = testOutput
	receipt.SafetyHits = safetyHits
	if len(diff) > 1000 {
		receipt.DiffSummary = diff[:1000] + "..."
	} else {
		receipt.DiffSummary = diff
	}

	// Count files changed and patch lines
	if diff != "" {
		lines := strings.Split(diff, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "diff --git") {
				receipt.FilesChanged++
			}
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				receipt.PatchLines++
			}
			if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				receipt.PatchLines++
			}
		}
	}

	jsonData, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal receipt json: %w", err)
	}
	mdContent := rw.formatReceiptMD(receipt, taskMeta, diff, testOutput, safetyHits)

	// Write JSON + Markdown receipts
	jsonPath := filepath.Join(rw.ReceiptsDir, fmt.Sprintf("%s.json", receipt.ReceiptID))
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write receipt json: %w", err)
	}
	mdPath := filepath.Join(rw.ReceiptsDir, fmt.Sprintf("%s.md", receipt.ReceiptID))
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		return fmt.Errorf("write receipt markdown: %w", err)
	}

	// Also write to runs dir
	runDir := filepath.Join(rw.RunsDir, fmt.Sprintf("run-%s", receipt.TaskID))
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "receipt.json"), jsonData, 0644); err != nil {
		return fmt.Errorf("write run receipt json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "receipt.md"), []byte(mdContent), 0644); err != nil {
		return fmt.Errorf("write run receipt markdown: %w", err)
	}
	if diff != "" {
		if err := os.WriteFile(filepath.Join(runDir, "diff.patch"), []byte(diff), 0644); err != nil {
			return fmt.Errorf("write diff patch: %w", err)
		}
	}
	if testOutput != "" {
		if err := os.WriteFile(filepath.Join(runDir, "test_output.txt"), []byte(testOutput), 0644); err != nil {
			return fmt.Errorf("write test output: %w", err)
		}
	}

	return nil
}

// WriteReviewMD writes a review.md file for human review.
func (rw *ReceiptWriter) WriteReviewMD(taskPath, taskID, diff, testOutput string, safetyHits []string, verdict string, pinocchioVerdict string, pinocchioIncons []string, testIntegrityPassed bool, testsRemoved, testsModified, testCommandsChanged []string) error {
	reviewDir := filepath.Dir(taskPath)
	os.MkdirAll(reviewDir, 0755)
	reviewPath := filepath.Join(reviewDir, "review.md")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Review: %s\n\n", taskID))
	b.WriteString(fmt.Sprintf("## Verdict\n**%s**\n\n", verdict))

	if pinocchioVerdict != "" || len(pinocchioIncons) > 0 {
		b.WriteString("## Pinocchio / Consistency Gate\n")
		if pinocchioVerdict != "" {
			b.WriteString(fmt.Sprintf("- **%s**\n", pinocchioVerdict))
		}
		for _, inc := range pinocchioIncons {
			b.WriteString(fmt.Sprintf("- Inconsistency: %s\n", inc))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Changes\n```diff\n")
	b.WriteString(diff)
	b.WriteString("\n```\n\n")

	b.WriteString("## Test Output\n```\n")
	b.WriteString(testOutput)
	b.WriteString("\n```\n\n")

	if len(safetyHits) > 0 {
		b.WriteString("## Safety Hits\n")
		for _, h := range safetyHits {
			b.WriteString(fmt.Sprintf("- %s\n", h))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Test Integrity Gate\n")
	if testIntegrityPassed {
		b.WriteString("- **passed**\n")
	} else {
		b.WriteString("- **failed**\n")
	}
	if len(testsRemoved) > 0 {
		b.WriteString(fmt.Sprintf("- tests removed: %s\n", strings.Join(testsRemoved, ", ")))
	}
	if len(testsModified) > 0 {
		b.WriteString(fmt.Sprintf("- tests modified: %s\n", strings.Join(testsModified, ", ")))
	}
	if len(testCommandsChanged) > 0 {
		b.WriteString(fmt.Sprintf("- test command changes: %s\n", strings.Join(testCommandsChanged, ", ")))
	}
	if !testIntegrityPassed {
		b.WriteString("- **human action required**: test inventory changed without explicit task permission\n")
	}
	b.WriteString("\n")

	b.WriteString("## Notification\n")
	b.WriteString("- **sent** (stdout)\n\n")

	b.WriteString("## Human Review Required\nThis task completed and needs human review before acceptance.\n")

	return os.WriteFile(reviewPath, []byte(b.String()), 0644)
}

func (rw *ReceiptWriter) formatReceiptMD(receipt *ForgeReceipt, taskMeta *TaskMeta, diff, testOutput string, safetyHits []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Receipt: %s\n\n", receipt.ReceiptID))
	b.WriteString(fmt.Sprintf("**Task**: %s\n", receipt.TaskID))
	if taskMeta != nil && taskMeta.Title != "" {
		b.WriteString(fmt.Sprintf("**Title**: %s\n", taskMeta.Title))
	}
	b.WriteString(fmt.Sprintf("**Verdict**: %s\n\n", receipt.Verdict))

	b.WriteString("## Timing\n")
	b.WriteString(fmt.Sprintf("- Started: %s\n", receipt.StartedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Finished: %s\n", receipt.FinishedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Duration: %ds\n", receipt.DurationSec))
	b.WriteString(fmt.Sprintf("- Timed out: %v\n\n", receipt.TimedOut))

	b.WriteString("## Stats\n")
	b.WriteString(fmt.Sprintf("- Exit code: %d\n", receipt.ExitCode))
	b.WriteString(fmt.Sprintf("- Rounds used: %d\n", receipt.RoundsUsed))
	b.WriteString(fmt.Sprintf("- Files changed: %d\n", receipt.FilesChanged))
	b.WriteString(fmt.Sprintf("- Patch lines: %d\n", receipt.PatchLines))
	if receipt.RunnerMode != "" {
		b.WriteString(fmt.Sprintf("- Runner mode: %s\n", receipt.RunnerMode))
	}
	if receipt.RunnerCommand != "" {
		b.WriteString(fmt.Sprintf("- Runner command: %s\n", receipt.RunnerCommand))
	}
	if receipt.RunnerExitCode != 0 {
		b.WriteString(fmt.Sprintf("- Runner exit code: %d\n", receipt.RunnerExitCode))
	}
	if receipt.WorktreePath != "" {
		b.WriteString(fmt.Sprintf("- Worktree: %s\n", receipt.WorktreePath))
	}
	if receipt.BaseCommit != "" {
		b.WriteString(fmt.Sprintf("- Base commit: %s\n", receipt.BaseCommit))
	}
	if receipt.OpenCodeModel != "" {
		b.WriteString(fmt.Sprintf("- OpenCode model: %s\n", receipt.OpenCodeModel))
	}
	if receipt.OpenCodeAgent != "" {
		b.WriteString(fmt.Sprintf("- OpenCode agent: %s\n", receipt.OpenCodeAgent))
	}
	if receipt.TestsPassed > 0 {
		b.WriteString(fmt.Sprintf("- Tests passed: %d\n", receipt.TestsPassed))
	}
	b.WriteString(fmt.Sprintf("- Scans passed: %v\n", receipt.ScansPassed))
	if receipt.C1LoopReceiptPath != "" {
		b.WriteString(fmt.Sprintf("- C1 loop receipt: %s\n", receipt.C1LoopReceiptPath))
	}
	if receipt.ForgeReceiptPath != "" {
		b.WriteString(fmt.Sprintf("- Forge receipt: %s\n", receipt.ForgeReceiptPath))
	}
	b.WriteString("\n")

	if len(safetyHits) > 0 {
		b.WriteString("## Safety Hits\n")
		for _, h := range safetyHits {
			b.WriteString(fmt.Sprintf("- %s\n", h))
		}
		b.WriteString("\n")
	}

	if receipt.Error != "" {
		b.WriteString("## Error\n")
		b.WriteString(fmt.Sprintf("%s\n\n", receipt.Error))
	}

	if diff != "" {
		if len(diff) > 5000 {
			b.WriteString("## Diff (truncated)\n```diff\n")
			b.WriteString(diff[:5000])
			b.WriteString("\n...\n```\n\n")
			b.WriteString(fmt.Sprintf("Full diff: %d bytes\n\n", len(diff)))
		} else {
			b.WriteString("## Diff\n```diff\n")
			b.WriteString(diff)
			b.WriteString("\n```\n\n")
		}
	}

	if testOutput != "" {
		b.WriteString("## Test Output\n```\n")
		b.WriteString(testOutput)
		b.WriteString("\n```\n\n")
	}

	if receipt.PinocchioResultPath != "" {
		b.WriteString("## Pinocchio / Consistency Gate\n")
		b.WriteString(fmt.Sprintf("- Verified: %v\n", receipt.PinocchioVerified))
		b.WriteString(fmt.Sprintf("- Initial verdict: %s\n", receipt.InitialVerdict))
		b.WriteString(fmt.Sprintf("- Final verdict: %s\n", receipt.FinalVerdict))
		b.WriteString(fmt.Sprintf("- Overridden: %v\n", receipt.VerdictOverridden))
		if receipt.VerdictOverridden {
			b.WriteString(fmt.Sprintf("- Override reason: %s\n", receipt.OverrideReason))
		}
		if len(receipt.PinocchioFalseClaims) > 0 {
			b.WriteString("- False claims:\n")
			for _, fc := range receipt.PinocchioFalseClaims {
				b.WriteString(fmt.Sprintf("  - %s\n", fc))
			}
		}
		if len(receipt.PinocchioInconsistencies) > 0 {
			b.WriteString("- Inconsistencies:\n")
			for _, inc := range receipt.PinocchioInconsistencies {
				b.WriteString(fmt.Sprintf("  - %s\n", inc))
			}
		}
		b.WriteString(fmt.Sprintf("- Pinocchio artifact: %s\n", receipt.PinocchioResultPath))
		b.WriteString("\n")
	}

	if receipt.GateChainAction != "" {
		b.WriteString("## GateChain Decision\n")
		b.WriteString(fmt.Sprintf("- Action: %s\n", receipt.GateChainAction))
		b.WriteString(fmt.Sprintf("- Final status: %s\n", receipt.GateChainFinalStatus))
		b.WriteString(fmt.Sprintf("- Stop required: %v\n", receipt.GateChainStopRequired))
		b.WriteString(fmt.Sprintf("- Review required: %v\n", receipt.GateChainReviewRequired))
		b.WriteString(fmt.Sprintf("- Reason: %s\n", receipt.GateChainActionReason))
		b.WriteString("\n")
	}

	b.WriteString("## Notification\n")
	b.WriteString(fmt.Sprintf("- Mode: %s\n", receipt.NotificationMode))
	b.WriteString(fmt.Sprintf("- Success: %v\n", receipt.NotificationSuccess))
	if receipt.NotificationError != "" {
		b.WriteString(fmt.Sprintf("- Error: %s\n", receipt.NotificationError))
	}
	b.WriteString("\n")

	if taskMeta != nil {
		b.WriteString("## Task Metadata\n")
		b.WriteString(fmt.Sprintf("- ID: %s\n", taskMeta.ID))
		b.WriteString(fmt.Sprintf("- Repo: %s\n", taskMeta.Repo))
		b.WriteString(fmt.Sprintf("- Goal: %s\n", taskMeta.Goal))
		b.WriteString(fmt.Sprintf("- Max minutes: %d\n", taskMeta.MaxMinutes))
		b.WriteString(fmt.Sprintf("- Max rounds: %d\n\n", taskMeta.MaxRounds))
	}

	return b.String()
}
