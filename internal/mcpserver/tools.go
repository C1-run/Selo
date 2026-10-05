package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/C1-run/selo/internal/runner"
)

func defaultTools() []Tool {
	return []Tool{
		{
			Name:        "selo_check_diff",
			Description: "Run Selo's post-run audit over a unified diff: forbidden file edits (allowlist and denylist), forbidden claims, and secret scanning. Returns every safety hit found.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"diff":             map[string]any{"type": "string", "description": "Unified diff of the changes to audit"},
					"worktree_path":    map[string]any{"type": "string", "description": "Worktree root the diff applies to (for file-content scans)"},
					"allowed_files":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional repo-relative glob allowlist; non-empty means files outside it violate"},
					"forbidden_files":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional path substrings that must not be modified"},
					"forbidden_claims": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional claim terms (case-insensitive, evasion-folding)"},
				},
				"required": []string{"diff", "worktree_path"},
			},
			Call: callCheckDiff,
		},
		{
			Name:        "selo_scan_path",
			Description: "Scan a directory recursively for secrets (private keys, cloud/API tokens) and forbidden claim terms.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":             map[string]any{"type": "string", "description": "Directory to scan"},
					"forbidden_claims": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional claim terms to scan for"},
				},
				"required": []string{"path"},
			},
			Call: callScanPath,
		},
		{
			Name:        "selo_preflight",
			Description: "Check a planned change against Governor limits (max files, max patch lines) before work starts.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"files_changed":   map[string]any{"type": "integer", "description": "Planned number of files to modify"},
					"patch_lines":     map[string]any{"type": "integer", "description": "Planned patch size in changed lines"},
					"max_files":       map[string]any{"type": "integer", "description": "Limit for files (default 10)"},
					"max_patch_lines": map[string]any{"type": "integer", "description": "Limit for patch lines (default 200)"},
				},
				"required": []string{"files_changed", "patch_lines"},
			},
			Call: callPreflight,
		},
	}
}

func asStringList(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func asInt(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

func callCheckDiff(args map[string]any) (string, error) {
	diff, _ := args["diff"].(string)
	worktree, _ := args["worktree_path"].(string)
	if diff == "" {
		return "", fmt.Errorf("diff is required")
	}
	if worktree == "" {
		return "", fmt.Errorf("worktree_path is required")
	}
	if _, err := os.Stat(worktree); err != nil {
		return "", fmt.Errorf("worktree_path %q is not readable: %w", worktree, err)
	}

	allowed := asStringList(args, "allowed_files")
	forbidden := asStringList(args, "forbidden_files")
	claims := asStringList(args, "forbidden_claims")

	var lines []string
	violations := 0

	violation, msg := runner.CheckForbiddenFileEdit(diff, worktree, allowed, forbidden)
	if violation {
		violations++
		lines = append(lines, "VIOLATION: "+msg)
	}

	changedFiles, _ := runner.GetChangedFiles(worktree, diff)
	claimHits, _ := runner.RunForbiddenClaimsScanDiffScoped(worktree, claims, changedFiles)
	for _, h := range claimHits {
		lines = append(lines, "HIT (forbidden claim): "+h)
	}
	secretHits, _ := runner.RunSecretScanDiffScoped(worktree, changedFiles)
	for _, h := range secretHits {
		lines = append(lines, "HIT (secret): "+h)
	}

	if len(lines) == 0 {
		return "PASS — no forbidden file edits, claims, or secrets in the audited diff.", nil
	}
	verdict := "PASS_WITH_FINDINGS"
	if violations > 0 {
		verdict = "FAIL"
	}
	return fmt.Sprintf("%s\n%s", verdict, strings.Join(lines, "\n")), nil
}

func callScanPath(args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("path %q is not readable: %w", abs, err)
	}

	secretHits, _ := runner.RunSecretScan(abs)
	claimHits, _ := runner.RunForbiddenClaimsScan(abs, asStringList(args, "forbidden_claims"))

	if len(secretHits) == 0 && len(claimHits) == 0 {
		return fmt.Sprintf("PASS — no secrets or forbidden claims found under %s.", abs), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "FINDINGS under %s:\n", abs)
	for _, h := range secretHits {
		fmt.Fprintf(&b, "HIT (secret): %s\n", h)
	}
	for _, h := range claimHits {
		fmt.Fprintf(&b, "HIT (forbidden claim): %s\n", h)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func callPreflight(args map[string]any) (string, error) {
	files := asInt(args, "files_changed", 0)
	lines := asInt(args, "patch_lines", 0)
	maxFiles := asInt(args, "max_files", 10)
	maxPatchLines := asInt(args, "max_patch_lines", 200)

	var issues []string
	if files > maxFiles {
		issues = append(issues, fmt.Sprintf("files_changed %d exceeds max_files %d", files, maxFiles))
	}
	if lines > maxPatchLines {
		issues = append(issues, fmt.Sprintf("patch_lines %d exceeds max_patch_lines %d", lines, maxPatchLines))
	}
	if len(issues) > 0 {
		return fmt.Sprintf("EXCEEDS LIMITS:\n%s", strings.Join(issues, "\n")), nil
	}
	return fmt.Sprintf("PASS — %d files and %d patch lines are within limits (%d, %d).", files, lines, maxFiles, maxPatchLines), nil
}
