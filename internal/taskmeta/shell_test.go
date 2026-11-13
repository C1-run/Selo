package taskmeta

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestShellParsersMatchTaskmeta is the writer↔shell-reader drift guard: a
// task file written by Write must extract to the same fields through
// scripts/task-parse.sh (what the shell runners actually run) as it parses to
// through Parse. This is the drift class behind the section-leak bug
// (BUG-2026-10-13) and the never-matching quoted forbidden_files entry; the
// shell side lives in one file, so there is exactly one place to drift and
// one test watching it.
func TestShellParsersMatchTaskmeta(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell runners do not run on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	scriptPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "scripts", "task-parse.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("scripts/task-parse.sh not found at %s: %v", scriptPath, err)
	}

	// shellFields sources task-parse.sh and returns TASK_GOAL, TASK_ID and
	// the two list fields as parsed items (quotes and "- " prefixes stripped).
	shellFields := func(t *testing.T, path string) (goal, id string, commands, forbidden []string) {
		t.Helper()
		script := `source "$TASK_PARSE"
task_parse_fields "$TASK"
printf '%s\x1e%s\x1e%s\x1e%s' "$TASK_GOAL" "$TASK_ID" "$TASK_COMMANDS" "$TASK_FORBIDDEN_FILES"
`
		cmd := exec.Command("bash", "-c", script)
		cmd.Env = append(os.Environ(),
			"TASK_PARSE="+scriptPath,
			"TASK="+path,
		)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("task_parse_fields: %v\n%s", err, out)
		}
		parts := strings.SplitN(string(out), "\x1e", 4)
		if len(parts) != 4 {
			t.Fatalf("unexpected shell output shape: %q", out)
		}
		goal, id = parts[0], parts[1]
		commands = shellItems(parts[2])
		forbidden = shellItems(parts[3])
		return goal, id, commands, forbidden
	}

	goWrites := func(t *testing.T, cfg *TaskConfig) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "task.md")
		if err := Write(path, cfg); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return path
	}

	// want mirrors the shell expectations against the Go parse of the same
	// file, so both readers are held to the writer's output.
	t.Run("documented layout", func(t *testing.T) {
		cfg := &TaskConfig{
			ID:             "run-abc123",
			Goal:           "Fix the bug",
			Repo:           "/repo",
			Commands:       []string{"go test ./...", "go vet ./..."},
			ForbiddenFiles: []string{".env", "secrets/*"},
			MaxMinutes:     42,
		}
		path := goWrites(t, cfg)

		parsed, err := Parse(path)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		goal, id, commands, forbidden := shellFields(t, path)

		if goal != parsed.Goal || id != parsed.ID {
			t.Errorf("shell goal/id = %q/%q; Parse = %q/%q", goal, id, parsed.Goal, parsed.ID)
		}
		if !equalStrings(commands, parsed.Commands) {
			t.Errorf("shell commands = %v; Parse = %v", commands, parsed.Commands)
		}
		if !equalStrings(forbidden, parsed.ForbiddenFiles) {
			t.Errorf("shell forbidden_files = %v; Parse = %v", forbidden, parsed.ForbiddenFiles)
		}
	})

	t.Run("commands_to_run alias", func(t *testing.T) {
		// The alias key is recognized by the Go parser and by the shared
		// shell extractor; both must see the same list.
		path := filepath.Join(t.TempDir(), "task.md")
		content := "id: alias-task\ngoal: NOOP\ncommands_to_run:\n  - \"go test ./...\"\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		parsed, err := Parse(path)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		_, _, commands, _ := shellFields(t, path)

		if !equalStrings(commands, parsed.Commands) {
			t.Errorf("shell commands = %v; Parse = %v", commands, parsed.Commands)
		}
	})
}

// shellItems turns a raw list field (newline-joined "  - \"item\"" lines)
// into parsed items.
func shellItems(raw string) []string {
	var items []string
	for _, line := range strings.Split(raw, "\n") {
		item := strings.TrimSpace(line)
		item = strings.TrimPrefix(item, "- ")
		item = strings.Trim(item, `"'`)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
