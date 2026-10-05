package governor

import (
	"fmt"
	"os"
	"strings"
)

// Governor enforces task limits and constraints.
type Governor struct {
	MaxRounds     int
	MaxFiles      int
	MaxPatchLines int
	StopFile      string
}

// NewGovernor creates a new governor.
func NewGovernor(maxRounds, maxFiles, maxPatchLines int, stopFile string) *Governor {
	return &Governor{
		MaxRounds:     maxRounds,
		MaxFiles:      maxFiles,
		MaxPatchLines: maxPatchLines,
		StopFile:      stopFile,
	}
}

// IsStopped checks if a stop file exists.
func (g *Governor) IsStopped() bool {
	if g.StopFile == "" {
		return false
	}
	_, err := os.Stat(g.StopFile)
	return err == nil
}

// CheckPatchLimits checks if the diff violates defined constraints.
// Returns (violation, message).
func (g *Governor) CheckPatchLimits(diff string) (bool, string) {
	if diff == "" {
		return false, ""
	}

	// Count files modified
	fileCount := 0
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			fileCount++
		}
	}

	if g.MaxFiles > 0 && fileCount > g.MaxFiles {
		return true, fmt.Sprintf("too many files modified: %d (max %d)", fileCount, g.MaxFiles)
	}

	// Count patch lines (lines starting with + or -, excluding --- and +++)
	patchLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			patchLines++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			patchLines++
		}
	}

	if g.MaxPatchLines > 0 && patchLines > g.MaxPatchLines {
		return true, fmt.Sprintf("too many patch lines: %d (max %d)", patchLines, g.MaxPatchLines)
	}

	return false, ""
}

// CheckRoundsExceeded checks if the round count exceeds the max.
func (g *Governor) CheckRoundsExceeded(round int) bool {
	return g.MaxRounds > 0 && round >= g.MaxRounds
}

// ParseTaskConfig extracts governor config from a task.md file.
// Returns (maxMinutes, maxRounds, maxFiles, maxPatchLines, commands, allowedFiles, forbiddenFiles, forbiddenClaims, allowTestModifications).
func ParseTaskConfig(taskPath string) (int, int, int, int, []string, []string, []string, []string, bool, error) {
	data, err := os.ReadFile(taskPath)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, nil, false, fmt.Errorf("read task: %w", err)
	}

	content := string(data)
	maxMinutes := 30
	maxRounds := 3
	maxFiles := 10
	maxPatchLines := 200
	var commands []string
	var allowedFiles []string
	var forbiddenFiles []string
	var forbiddenClaims []string
	allowTestMods := false

	// Simple YAML-like parser for task.md
	lines := strings.Split(content, "\n")
	inCommands := false
	inAllowed := false
	inForbidden := false
	inForbiddenClaims := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "commands:" || trimmed == "commands_to_run:" {
			inCommands = true
			continue
		}
		if trimmed == "allowed_files:" {
			inAllowed = true
			inCommands = false
			continue
		}
		if trimmed == "forbidden_files:" {
			inForbidden = true
			inAllowed = false
			continue
		}
		if trimmed == "forbidden_claims:" {
			inForbiddenClaims = true
			inForbidden = false
			continue
		}
		if trimmed == "deliverables:" {
			inCommands = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
			continue
		}

		// Check for next section
		if strings.HasPrefix(trimmed, "acceptance_criteria:") || strings.HasPrefix(trimmed, "goal:") || strings.HasPrefix(trimmed, "title:") || strings.HasPrefix(trimmed, "repo:") || strings.HasPrefix(trimmed, "id:") {
			inCommands = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		}

		if strings.HasPrefix(trimmed, "max_minutes:") {
			fmt.Sscanf(trimmed, "max_minutes: %d", &maxMinutes)
		}
		if strings.HasPrefix(trimmed, "max_rounds:") {
			fmt.Sscanf(trimmed, "max_rounds: %d", &maxRounds)
		}
		if strings.HasPrefix(trimmed, "max_files:") {
			fmt.Sscanf(trimmed, "max_files: %d", &maxFiles)
		}
		if strings.HasPrefix(trimmed, "max_patch_lines:") {
			fmt.Sscanf(trimmed, "max_patch_lines: %d", &maxPatchLines)
		}
		if strings.HasPrefix(trimmed, "allow_test_modifications:") {
			fmt.Sscanf(trimmed, "allow_test_modifications: %t", &allowTestMods)
		}

		if inCommands && strings.HasPrefix(trimmed, "- ") {
			commands = append(commands, unquoteTaskItem(strings.TrimPrefix(trimmed, "- ")))
		}
		if inAllowed && strings.HasPrefix(trimmed, "- ") {
			allowedFiles = append(allowedFiles, unquoteTaskItem(strings.TrimPrefix(trimmed, "- ")))
		}
		if inForbidden && strings.HasPrefix(trimmed, "- ") {
			forbiddenFiles = append(forbiddenFiles, unquoteTaskItem(strings.TrimPrefix(trimmed, "- ")))
		}
		if inForbiddenClaims && strings.HasPrefix(trimmed, "- ") {
			forbiddenClaims = append(forbiddenClaims, unquoteTaskItem(strings.TrimPrefix(trimmed, "- ")))
		}
	}

	return maxMinutes, maxRounds, maxFiles, maxPatchLines, commands, allowedFiles, forbiddenFiles, forbiddenClaims, allowTestMods, nil
}

// unquoteTaskItem strips surrounding quotes from a task.md list value.
// cmd_run writes list items quoted; a quoted forbidden_files entry never
// substring-matches the paths it is audited against, and a quoted
// allowed_files entry never prefix-matches, so the quotes must go.
func unquoteTaskItem(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	return strings.Trim(s, "'")
}
