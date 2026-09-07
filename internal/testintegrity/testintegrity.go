package testintegrity

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TestInventory represents the test infrastructure of a worktree.
type TestInventory struct {
	Files        []string
	TestCommands []string
	Count        int
}

// TestIntegrityResult holds the outcome of a test integrity analysis.
type TestIntegrityResult struct {
	Passed               bool
	TestsRemoved         []string
	TestsModified        []string
	TestCommandsChanged  []string
	SkipMarkersAdded     []string
	AssertionsRemoved    []string
	InventoryBeforeCount int
	InventoryAfterCount  int
	AllowedByTask        bool
	RecommendedVerdict   string
	Findings             []string
}

// isTestFile checks if a file path matches known test file patterns.
func isTestFile(path string) bool {
	name := filepath.Base(path)
	dir := filepath.Dir(path)

	if strings.HasSuffix(name, "_test.go") {
		return true
	}
	if strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") {
		return true
	}
	if strings.HasSuffix(name, ".spec.ts") || strings.HasSuffix(name, ".spec.tsx") {
		return true
	}
	if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") {
		return true
	}
	if strings.HasSuffix(name, "_test.py") {
		return true
	}
	if dir == "tests" || dir == "__tests__" || strings.HasSuffix(dir, "/tests") || strings.HasSuffix(dir, "/__tests__") {
		return true
	}
	if strings.Contains(path, "/tests/") || strings.Contains(path, "/__tests__/") {
		return true
	}
	return false
}

// CaptureInventory scans a worktree directory and returns its test inventory.
func CaptureInventory(workDir string, testCommands []string) *TestInventory {
	inv := &TestInventory{}
	inv.TestCommands = append([]string{}, testCommands...)

	filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}
		if isTestFile(rel) {
			inv.Files = append(inv.Files, rel)
		}
		return nil
	})

	inv.Count = len(inv.Files)
	return inv
}

var skipMarkerRegex = regexp.MustCompile(`(\.skip|xit\(|describe\.skip\(|test\.skip\(|it\.skip\(|xdescribe|xtest|#\[ignore\])`)
var assertionLikeRegex = regexp.MustCompile(`\b(expect\(|assert|require\.|t\.Fatal\b|t\.Errorf\b|should\.)`)

// Analyze compares test inventories and a diff to detect test integrity violations.
func Analyze(before, after *TestInventory, diff string, allowTestModifications bool) *TestIntegrityResult {
	result := &TestIntegrityResult{
		InventoryBeforeCount: before.Count,
		InventoryAfterCount:  after.Count,
		AllowedByTask:        allowTestModifications,
		RecommendedVerdict:   "",
	}

	// 1. Detect deleted test files
	beforeSet := make(map[string]bool)
	for _, f := range before.Files {
		beforeSet[f] = true
	}
	afterSet := make(map[string]bool)
	for _, f := range after.Files {
		afterSet[f] = true
	}
	for _, f := range before.Files {
		if !afterSet[f] {
			result.TestsRemoved = append(result.TestsRemoved, f)
			result.Findings = append(result.Findings, fmt.Sprintf("test file removed: %s", f))
		}
	}

	// 2. Detect modified test files (present in both but changed in diff)
	if diff != "" {
		diffLines := strings.Split(diff, "\n")
		currentFile := ""
		for _, line := range diffLines {
			if strings.HasPrefix(line, "diff --git ") {
				parts := strings.Split(line, " b/")
				if len(parts) >= 2 {
					currentFile = strings.TrimSpace(parts[len(parts)-1])
				} else {
					currentFile = ""
				}
				continue
			}
			if currentFile == "" {
				continue
			}
			if !isTestFile(currentFile) {
				continue
			}
			if afterSet[currentFile] && beforeSet[currentFile] {
				// File exists in both, but has changes in diff
				if !contains(result.TestsModified, currentFile) {
					result.TestsModified = append(result.TestsModified, currentFile)
				}
			}
			// Check for skip markers in added lines
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				matches := skipMarkerRegex.FindAllString(line, -1)
				for _, m := range matches {
					if !contains(result.SkipMarkersAdded, m) {
						result.SkipMarkersAdded = append(result.SkipMarkersAdded, m)
					}
					finding := fmt.Sprintf("skip marker added in %s: %s", currentFile, m)
					if !contains(result.Findings, finding) {
						result.Findings = append(result.Findings, finding)
					}
				}
			}
			// Check for assertion removal in removed lines
			if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				matches := assertionLikeRegex.FindAllString(line, -1)
				for _, m := range matches {
					if !contains(result.AssertionsRemoved, m) {
						result.AssertionsRemoved = append(result.AssertionsRemoved, m)
					}
					finding := fmt.Sprintf("assertion-like string removed in %s: %s", currentFile, m)
					if !contains(result.Findings, finding) {
						result.Findings = append(result.Findings, finding)
					}
				}
			}
		}
	}

	// 3. Detect test command changes
	beforeCmds := make(map[string]bool)
	for _, c := range before.TestCommands {
		beforeCmds[c] = true
	}
	for _, c := range after.TestCommands {
		if !beforeCmds[c] {
			result.TestCommandsChanged = append(result.TestCommandsChanged, c)
			result.Findings = append(result.Findings, fmt.Sprintf("test command changed/added: %s", c))
		}
	}
	for _, c := range before.TestCommands {
		if !contains(after.TestCommands, c) {
			result.TestCommandsChanged = append(result.TestCommandsChanged, c)
			finding := fmt.Sprintf("test command removed: %s", c)
			if !contains(result.Findings, finding) {
				result.Findings = append(result.Findings, finding)
			}
		}
	}

	// 4. Determine pass/fail and verdict
	violations := len(result.TestsRemoved) + len(result.TestsModified) + len(result.TestCommandsChanged) + len(result.SkipMarkersAdded) + len(result.AssertionsRemoved)

	if violations == 0 {
		result.Passed = true
		result.RecommendedVerdict = ""
		return result
	}

	if allowTestModifications {
		result.Passed = true
		result.RecommendedVerdict = "NEEDS_HUMAN"
		result.Findings = append(result.Findings, "test modifications allowed by task config but require human review")
		return result
	}

	result.Passed = false
	result.RecommendedVerdict = "NEEDS_HUMAN"
	result.Findings = append([]string{"test integrity violation: test modifications not allowed by task"}, result.Findings...)
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
