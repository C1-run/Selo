package pinocchio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/desmondkam/selo/internal/receipt"
	"github.com/desmondkam/selo/internal/runner"
)

// PinocchioResult captures the deterministic consistency verification outcome.
type PinocchioResult struct {
	Verified           bool     `json:"verified"`
	FalseClaims        []string `json:"false_claims"`
	Inconsistencies    []string `json:"inconsistencies"`
	SafetyFindings     []string `json:"safety_findings"`
	RecommendedVerdict string   `json:"recommended_verdict"`
	FinalVerdict       string   `json:"final_verdict"`
	CheckedArtifacts   []string `json:"checked_artifacts"`
	CheckedAtUTC       string   `json:"checked_at_utc"`
}

// C1Receipt is a minimal parser for c1-receipt.json output by the C1 Loop binary.
type C1Receipt struct {
	C1LoopVersion string `json:"c1_loop_version"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	Goal          string `json:"goal"`
	TaskFile      string `json:"task_file"`
	Workdir       string `json:"workdir"`
	StartedAt     string `json:"started_at"`
	ExitedAt      string `json:"exited_at"`
}

// VerificationArtifacts bundles all data the verifier needs.
type VerificationArtifacts struct {
	TaskMeta          *receipt.TaskMeta
	Result            *runner.C1Result
	SafetyHits        []string
	Diff              string
	TestOutput        string
	WorktreePath      string
	RunnerMode        runner.RunnerMode
	C1LoopReceiptPath string
	InitialVerdict    string
}

// VerifyForgeReceipt runs deterministic consistency checks and returns a verdict recommendation.
func VerifyForgeReceipt(artifacts *VerificationArtifacts) *PinocchioResult {
	r := &PinocchioResult{
		RecommendedVerdict: artifacts.InitialVerdict,
		FinalVerdict:       artifacts.InitialVerdict,
	}

	// Record checked artifacts
	r.CheckedArtifacts = []string{
		"forge_receipt",
		"runner_exit_code",
		"safety_hits",
		"diff_evidence",
		"test_output",
	}
	if artifacts.C1LoopReceiptPath != "" {
		r.CheckedArtifacts = append(r.CheckedArtifacts, "c1_loop_receipt")
	}
	if artifacts.TaskMeta != nil {
		r.CheckedArtifacts = append(r.CheckedArtifacts, "task_metadata")
	}

	// Categorise safety hits
	r.SafetyFindings = classifySafetyFindings(artifacts.SafetyHits)

	// Count files changed and patch lines from diff
	filesChanged, patchLines := countDiffStats(artifacts.Diff)

	// Run checks based on initial verdict
	switch artifacts.InitialVerdict {
	case receipt.VerdictSuccess:
		r.checkSuccess(artifacts, filesChanged, patchLines)
	case receipt.VerdictNoop:
		r.checkNoop(artifacts, filesChanged, patchLines)
	case receipt.VerdictPartial, receipt.VerdictNeedsHuman:
		r.checkPartialOrHuman(artifacts)
	case receipt.VerdictSafety:
		r.checkSafety(artifacts)
	case receipt.VerdictTimedOut:
		r.checkTimeout(artifacts)
	}

	// If safety findings exist, upgrade to FAILED_SAFETY regardless of initial verdict
	if len(r.SafetyFindings) > 0 && r.FinalVerdict != receipt.VerdictSafety {
		r.FinalVerdict = receipt.VerdictSafety
		r.Inconsistencies = append(r.Inconsistencies,
			fmt.Sprintf("initial verdict %s but safety findings exist", artifacts.InitialVerdict))
	}

	// If there are false claims or inconsistencies (and no safety override), upgrade to NEEDS_HUMAN
	if r.FinalVerdict != receipt.VerdictSafety && r.FinalVerdict != receipt.VerdictTimedOut &&
		r.FinalVerdict != receipt.VerdictLimitExceeded && r.FinalVerdict != receipt.VerdictInternalError {
		if len(r.FalseClaims) > 0 || len(r.Inconsistencies) > 0 {
			if severity(receipt.VerdictNeedsHuman) > severity(r.FinalVerdict) {
				r.FinalVerdict = receipt.VerdictNeedsHuman
			}
		}
	}

	r.Verified = len(r.FalseClaims) == 0 && len(r.Inconsistencies) == 0 && len(r.SafetyFindings) == 0
	return r
}

func (r *PinocchioResult) checkSuccess(artifacts *VerificationArtifacts, filesChanged, patchLines int) {
	// 1. Runner exit code must be 0
	if artifacts.Result != nil && artifacts.Result.ExitCode != 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("SUCCESS claimed but runner exit code = %d", artifacts.Result.ExitCode))
	}

	// 2. Scans must have passed
	if len(artifacts.SafetyHits) > 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("SUCCESS claimed but %d safety hits exist", len(artifacts.SafetyHits)))
	}

	// 3. Tests must have passed if test commands exist
	if artifacts.TaskMeta != nil && len(artifacts.TaskMeta.Commands) > 0 {
		if !testOutputPassed(artifacts.TestOutput) {
			r.FalseClaims = append(r.FalseClaims,
				"SUCCESS claimed but test output indicates failure")
		}
	}

	// 4. C1 receipt must exist and be compatible if runner is c1-compatible
	if artifacts.RunnerMode == runner.ModeReal && fileExists(artifacts.C1LoopReceiptPath) {
		c1Rec := parseC1Receipt(artifacts.C1LoopReceiptPath)
		if c1Rec == nil {
			r.Inconsistencies = append(r.Inconsistencies,
				fmt.Sprintf("C1 receipt exists but could not be parsed: %s", artifacts.C1LoopReceiptPath))
		} else if c1Rec.Status == "completed" && c1Rec.ExitCode != 0 {
			r.Inconsistencies = append(r.Inconsistencies,
				fmt.Sprintf("C1 receipt status=completed but exit_code=%d", c1Rec.ExitCode))
		}
	} else if artifacts.RunnerMode == runner.ModeReal && artifacts.C1LoopReceiptPath != "" && !fileExists(artifacts.C1LoopReceiptPath) {
		r.Inconsistencies = append(r.Inconsistencies,
			fmt.Sprintf("C1LoopReceiptPath set but file missing: %s", artifacts.C1LoopReceiptPath))
	}

	// 5. Required fields must be present
	if artifacts.WorktreePath == "" {
		r.Inconsistencies = append(r.Inconsistencies, "worktree_path is empty")
	}
	if artifacts.Result != nil && artifacts.Result.CommandLog == "" {
		r.Inconsistencies = append(r.Inconsistencies, "runner_command is empty")
	}
	if artifacts.RunnerMode == "" {
		r.Inconsistencies = append(r.Inconsistencies, "runner_mode is empty")
	}

	// 6. Diff evidence must be compatible with files_changed/patch_lines
	if artifacts.Diff != "" && filesChanged <= 0 {
		r.Inconsistencies = append(r.Inconsistencies,
			"diff exists but files_changed = 0")
	}
	if artifacts.Diff != "" && patchLines <= 0 {
		r.Inconsistencies = append(r.Inconsistencies,
			"diff exists but patch_lines = 0")
	}
	if artifacts.Diff == "" && filesChanged > 0 {
		r.Inconsistencies = append(r.Inconsistencies,
			"files_changed > 0 but diff is empty")
	}

	// If any false claims, recommended verdict changes
	if len(r.FalseClaims) > 0 {
		r.RecommendedVerdict = receipt.VerdictNeedsHuman
	}
}

func (r *PinocchioResult) checkNoop(artifacts *VerificationArtifacts, filesChanged, patchLines int) {
	// 1. Runner exit code must be 0
	if artifacts.Result != nil && artifacts.Result.ExitCode != 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("NOOP claimed but runner exit code = %d", artifacts.Result.ExitCode))
	}

	// 2. Files changed must be 0
	if filesChanged > 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("NOOP claimed but files_changed = %d", filesChanged))
	}

	// 3. Patch lines must be 0
	if patchLines > 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("NOOP claimed but patch_lines = %d", patchLines))
	}

	// 4. Safety hits must be empty
	if len(artifacts.SafetyHits) > 0 {
		r.FalseClaims = append(r.FalseClaims,
			fmt.Sprintf("NOOP claimed but %d safety hits", len(artifacts.SafetyHits)))
	}

	if len(r.FalseClaims) > 0 {
		r.RecommendedVerdict = receipt.VerdictNeedsHuman
	}
}

func (r *PinocchioResult) checkPartialOrHuman(artifacts *VerificationArtifacts) {
	// Verify that there IS evidence of failure
	hasFailureEvidence := false

	if artifacts.TaskMeta != nil && len(artifacts.TaskMeta.Commands) > 0 {
		if !testOutputPassed(artifacts.TestOutput) {
			hasFailureEvidence = true
		}
	}

	if artifacts.Result != nil && artifacts.Result.ExitCode != 0 && artifacts.Diff != "" {
		hasFailureEvidence = true
	}

	if artifacts.Diff == "" && (artifacts.Result == nil || artifacts.Result.ExitCode == 0) {
		if len(artifacts.SafetyHits) == 0 {
			r.Inconsistencies = append(r.Inconsistencies,
				fmt.Sprintf("%s claimed but no diff, no error, no safety hits", artifacts.InitialVerdict))
		}
	}

	if !hasFailureEvidence && len(artifacts.SafetyHits) == 0 && len(r.Inconsistencies) == 0 {
		r.Inconsistencies = append(r.Inconsistencies,
			fmt.Sprintf("%s claimed but no test failure, non-zero exit, or safety evidence", artifacts.InitialVerdict))
	}
}

func (r *PinocchioResult) checkSafety(artifacts *VerificationArtifacts) {
	// Safety verdict requires safety hits or safety findings
	if len(artifacts.SafetyHits) == 0 && len(r.SafetyFindings) == 0 {
		r.Inconsistencies = append(r.Inconsistencies,
			"FAILED_SAFETY claimed but no safety hits found")
	}
}

func (r *PinocchioResult) checkTimeout(artifacts *VerificationArtifacts) {
	// Timeout verdict requires evidence of timeout
	if artifacts.Result != nil && !artifacts.Result.TimedOut {
		r.Inconsistencies = append(r.Inconsistencies,
			"FAILED_TIMEOUT claimed but runner did not time out")
	}
}

// classifySafetyFindings extracts safety-related hits and checks for specific patterns.
func classifySafetyFindings(safetyHits []string) []string {
	var findings []string
	forward := map[string]string{
		"git push":              "push/deploy command detected",
		"deploy":                "deploy keyword detected",
		"npm publish":           "publish command detected",
		"wrangler deploy":       "wrangler deploy detected",
		"vercel deploy":         "vercel deploy detected",
		"docker push":           "docker push detected",
		".env":                  ".env access detected",
		"secret":                "secret/key detected",
		"forbidden":             "forbidden file detected",
		"LIVE_READY":            "forbidden claim LIVE_READY",
		"PROFITABLE":            "forbidden claim PROFITABLE",
		"MONEY_ENGINE":          "forbidden claim MONEY_ENGINE",
		"CAPITAL_APPROVED":      "forbidden claim CAPITAL_APPROVED",
		"LIVE_CAPITAL_APPROVED": "forbidden claim LIVE_CAPITAL_APPROVED",
		"MONEY_MACHINE":         "forbidden claim MONEY_MACHINE",
		"PRODUCTION_READY":      "forbidden claim PRODUCTION_READY",
		"BATTLE_TESTED":         "forbidden claim BATTLE_TESTED",
		"FULLY_TESTED":          "forbidden claim FULLY_TESTED",
		"ENTERPRISE_GRADE":      "forbidden claim ENTERPRISE_GRADE",
		"SOC2_COMPLIANT":        "forbidden claim SOC2_COMPLIANT",
		"SECURE_BY_DESIGN":      "forbidden claim SECURE_BY_DESIGN",
	}
	for _, hit := range safetyHits {
		lower := strings.ToLower(hit)
		for keyword, label := range forward {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				findings = append(findings, fmt.Sprintf("%s: %s", label, hit))
				break
			}
		}
	}
	return findings
}

// testOutputPassed checks if test output indicates all tests passed.
func testOutputPassed(output string) bool {
	if output == "" {
		return true // no test output = no failing tests
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "FAIL") {
			return false
		}
	}
	return true
}

// countDiffStats counts files changed and patch lines from a diff string.
func countDiffStats(diff string) (int, int) {
	if diff == "" {
		return 0, 0
	}
	files, lines := 0, 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			files++
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			lines++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			lines++
		}
	}
	return files, lines
}

// parseC1Receipt reads and parses a C1 receipt JSON file.
func parseC1Receipt(path string) *C1Receipt {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rec C1Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	return &rec
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WritePinocchioArtifact writes the Pinocchio result to a JSON file in the run directory.
func WritePinocchioArtifact(runsDir, taskID string, result *PinocchioResult) string {
	if runsDir == "" || taskID == "" {
		return ""
	}
	runDir := filepath.Join(runsDir, fmt.Sprintf("run-%s", taskID))
	os.MkdirAll(runDir, 0755)
	path := filepath.Join(runDir, "pinocchio.json")
	result.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	data, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(path, data, 0644)
	return path
}

// severity returns a numeric severity for verdict comparison (higher = more severe).
func severity(v string) int {
	switch v {
	case receipt.VerdictNoop:
		return 0
	case receipt.VerdictSuccess:
		return 1
	case receipt.VerdictPartial:
		return 2
	case receipt.VerdictNeedsHuman:
		return 3
	case receipt.VerdictTimedOut:
		return 4
	case receipt.VerdictLimitExceeded:
		return 5
	case receipt.VerdictSafety:
		return 6
	case receipt.VerdictInternalError:
		return 7
	default:
		return 3
	}
}
