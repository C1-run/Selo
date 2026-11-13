package governor

import (
	"fmt"
	"os"
	"strings"

	"github.com/C1-run/selo/internal/taskmeta"
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
	cfg, err := taskmeta.Parse(taskPath)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, nil, false, err
	}
	return ConfigFromTask(cfg), nil
}

// ConfigFromTask derives the governor config from an already-parsed task
// config. Callers that parsed the task file with taskmeta.Parse should use
// this instead of re-reading and re-parsing the file.
func ConfigFromTask(cfg *taskmeta.TaskConfig) (int, int, int, int, []string, []string, []string, []string, bool) {
	return cfg.MaxMinutes, cfg.MaxRounds, cfg.MaxFiles, cfg.MaxPatchLines,
		cfg.Commands, cfg.AllowedFiles, cfg.ForbiddenFiles, cfg.ForbiddenClaims,
		cfg.AllowTestModifications
}
