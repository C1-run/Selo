package taskmeta

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type TaskConfig struct {
	ID                     string
	Title                  string
	Repo                   string
	Goal                   string
	AcceptanceCriteria     []string
	Commands               []string
	Deliverables           []string
	MaxMinutes             int
	MaxRounds              int
	MaxFiles               int
	MaxPatchLines          int
	AllowedFiles           []string
	ForbiddenFiles         []string
	ForbiddenClaims        []string
	AllowTestModifications bool
}

var taskKeyRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):`)

var recognizedTaskKeys = map[string]bool{
	"id": true, "title": true, "repo": true, "goal": true,
	"acceptance_criteria": true, "commands": true, "commands_to_run": true,
	"deliverables": true, "max_minutes": true, "max_rounds": true,
	"max_files": true, "max_patch_lines": true,
	"allowed_files": true, "forbidden_files": true, "forbidden_claims": true,
	"allow_test_modifications": true,
}

func Parse(taskPath string) (*TaskConfig, error) {
	data, err := os.ReadFile(taskPath)
	if err != nil {
		return nil, fmt.Errorf("read task: %w", err)
	}

	cfg := &TaskConfig{
		MaxMinutes:    30,
		MaxRounds:     3,
		MaxFiles:      10,
		MaxPatchLines: 200,
	}

	lines := strings.Split(string(data), "\n")
	inAC := false
	inCommands := false
	inDeliverables := false
	inAllowed := false
	inForbidden := false
	inForbiddenClaims := false

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
			cfg.ID = stripQuotes(strings.TrimPrefix(trimmed, "id:"))
		} else if strings.HasPrefix(trimmed, "title:") {
			cfg.Title = stripQuotes(strings.TrimPrefix(trimmed, "title:"))
		} else if strings.HasPrefix(trimmed, "repo:") {
			cfg.Repo = stripQuotes(strings.TrimPrefix(trimmed, "repo:"))
		} else if strings.HasPrefix(trimmed, "goal:") {
			cfg.Goal = stripQuotes(strings.TrimPrefix(trimmed, "goal:"))
		} else if strings.HasPrefix(trimmed, "acceptance_criteria:") {
			inAC = true
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if trimmed == "commands:" || trimmed == "commands_to_run:" {
			inCommands = true
			inAC = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if trimmed == "deliverables:" {
			inDeliverables = true
			inAC = false
			inCommands = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if trimmed == "allowed_files:" {
			inAllowed = true
			inAC = false
			inCommands = false
			inDeliverables = false
			inForbidden = false
			inForbiddenClaims = false
		} else if trimmed == "forbidden_files:" {
			inForbidden = true
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbiddenClaims = false
		} else if trimmed == "forbidden_claims:" {
			inForbiddenClaims = true
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
		} else if strings.HasPrefix(trimmed, "max_minutes:") {
			fmt.Sscanf(trimmed, "max_minutes: %d", &cfg.MaxMinutes)
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if strings.HasPrefix(trimmed, "max_rounds:") {
			fmt.Sscanf(trimmed, "max_rounds: %d", &cfg.MaxRounds)
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if strings.HasPrefix(trimmed, "max_files:") {
			fmt.Sscanf(trimmed, "max_files: %d", &cfg.MaxFiles)
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if strings.HasPrefix(trimmed, "max_patch_lines:") {
			fmt.Sscanf(trimmed, "max_patch_lines: %d", &cfg.MaxPatchLines)
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if strings.HasPrefix(trimmed, "allow_test_modifications:") {
			fmt.Sscanf(trimmed, "allow_test_modifications: %t", &cfg.AllowTestModifications)
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		} else if inAC && strings.HasPrefix(trimmed, "- ") {
			cfg.AcceptanceCriteria = append(cfg.AcceptanceCriteria, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inCommands && strings.HasPrefix(trimmed, "- ") {
			cfg.Commands = append(cfg.Commands, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inDeliverables && strings.HasPrefix(trimmed, "- ") {
			cfg.Deliverables = append(cfg.Deliverables, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inAllowed && strings.HasPrefix(trimmed, "- ") {
			cfg.AllowedFiles = append(cfg.AllowedFiles, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inForbidden && strings.HasPrefix(trimmed, "- ") {
			cfg.ForbiddenFiles = append(cfg.ForbiddenFiles, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if inForbiddenClaims && strings.HasPrefix(trimmed, "- ") {
			cfg.ForbiddenClaims = append(cfg.ForbiddenClaims, stripQuotes(strings.TrimPrefix(trimmed, "- ")))
		} else if trimmed != "" && !strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "#") {
			return nil, fmt.Errorf("invalid task syntax at line: %q", trimmed)
		} else {
			inAC = false
			inCommands = false
			inDeliverables = false
			inAllowed = false
			inForbidden = false
			inForbiddenClaims = false
		}
	}

	return cfg, nil
}
