package p45

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/C1-run/selo/internal/containment"
)

type P45GoalContract struct {
	Goal       string `json:"goal"`
	Splittable bool   `json:"splittable"`
}

type P45RunContract struct {
	Goal            string `json:"goal"`
	DeadlineUTC     string `json:"deadline_utc"`
	MaxPatchCount   int    `json:"max_patch_count"`
	MaxFiles        int    `json:"max_files"`
	MaxPatchLines   int    `json:"max_patch_lines"`
	StopFile        string `json:"stop_file"`
	ReceiptRequired bool   `json:"receipt_required"`
}

var forbiddenNames = []string{
	"c1f-",
	"safety_hits",
	".selo-stop",
}

func LoadGoal(dir string) (*P45GoalContract, error) {
	path := filepath.Join(dir, "goal_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("p45: read goal_contract.json: %w", err)
	}
	var c P45GoalContract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("p45: parse goal_contract.json: %w", err)
	}
	if c.Goal == "" {
		return nil, fmt.Errorf("p45: goal_contract.json: goal is required")
	}
	return &c, nil
}

func LoadScope(dir string) (*containment.ScopeContract, error) {
	path := filepath.Join(dir, "scope_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("p45: read scope_contract.json: %w", err)
	}
	var c containment.ScopeContract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("p45: parse scope_contract.json: %w", err)
	}
	if err := validateScopeNaming(&c); err != nil {
		return nil, err
	}
	// Refuse to start on a glob we cannot evaluate. Silently ignoring one would
	// leave the boundary it describes unenforced while the run reports clean.
	if err := containment.ValidateScopePatterns(&c); err != nil {
		return nil, fmt.Errorf("p45: scope_contract.json: %w", err)
	}
	return &c, nil
}

func LoadRun(dir string) (*P45RunContract, error) {
	path := filepath.Join(dir, "run_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("p45: read run_contract.json: %w", err)
	}
	var c P45RunContract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("p45: parse run_contract.json: %w", err)
	}
	if c.Goal == "" {
		return nil, fmt.Errorf("p45: run_contract.json: goal is required")
	}
	if c.DeadlineUTC == "" {
		return nil, fmt.Errorf("p45: run_contract.json: deadline_utc is required")
	}
	if c.MaxPatchCount < 1 {
		return nil, fmt.Errorf("p45: run_contract.json: max_patch_count must be >= 1")
	}
	if c.StopFile == "" {
		return nil, fmt.Errorf("p45: run_contract.json: stop_file is required")
	}
	if !strings.Contains(c.StopFile, ".containment-stop") {
		return nil, fmt.Errorf("p45: run_contract.json: stop_file must reference .containment-stop, got %q", c.StopFile)
	}
	if err := validateRunNaming(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func validateScopeNaming(c *containment.ScopeContract) error {
	all := append([]string{}, c.AllowedPaths...)
	all = append(all, c.ForbiddenPaths...)
	all = append(all, c.ChangedPaths...)
	all = append(all, c.ScopeViolations...)
	return checkForbidden(all...)
}

func validateRunNaming(c *P45RunContract) error {
	return checkForbidden(c.Goal)
}

func checkForbidden(values ...string) error {
	for _, v := range values {
		lower := strings.ToLower(v)
		for _, f := range forbiddenNames {
			if strings.Contains(lower, f) {
				return fmt.Errorf("p45: contract contains forbidden reference %q: %q", f, v)
			}
		}
	}
	return nil
}
