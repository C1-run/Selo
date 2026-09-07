package receipt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTaskMetaFull(t *testing.T) {
	content := `id: "task-1"
title: "Test Task"
repo: "/tmp/test"
goal: "Fix the bug"
commands:
  - "go test ./..."
max_minutes: 5
max_rounds: 2
deliverables:
  - "receipt.md"
`
	f := writeTempFile(t, "task.md", content)
	meta, err := ParseTaskMeta(f)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "task-1" {
		t.Errorf("ID=%q, want %q", meta.ID, "task-1")
	}
	if meta.Goal != "Fix the bug" {
		t.Errorf("Goal=%q, want %q", meta.Goal, "Fix the bug")
	}
	if len(meta.Commands) != 1 || meta.Commands[0] != "go test ./..." {
		t.Errorf("Commands=%v, want [go test ./...]", meta.Commands)
	}
	if meta.MaxMinutes != 5 {
		t.Errorf("MaxMinutes=%d, want 5", meta.MaxMinutes)
	}
	if meta.MaxRounds != 2 {
		t.Errorf("MaxRounds=%d, want 2", meta.MaxRounds)
	}
}

func TestParseTaskMetaDefaults(t *testing.T) {
	content := `id: "task-default"
goal: "Do nothing"
`
	f := writeTempFile(t, "task.md", content)
	meta, err := ParseTaskMeta(f)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MaxMinutes != 30 {
		t.Errorf("MaxMinutes=%d, want default 30", meta.MaxMinutes)
	}
	if meta.MaxRounds != 3 {
		t.Errorf("MaxRounds=%d, want default 3", meta.MaxRounds)
	}
}

func TestParseTaskMetaCommandsToRun(t *testing.T) {
	content := `id: "task-cmds"
goal: "Run tests"
commands_to_run:
  - "npm test"
`
	f := writeTempFile(t, "task.md", content)
	meta, err := ParseTaskMeta(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Commands) != 1 || meta.Commands[0] != "npm test" {
		t.Errorf("Commands=%v, want [npm test]", meta.Commands)
	}
}

func TestParseTaskMetaMissingFile(t *testing.T) {
	_, err := ParseTaskMeta("/nonexistent/task.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseTaskMetaInvalidYAML(t *testing.T) {
	f := writeTempFile(t, "task.md", "{{invalid yaml}}")
	_, err := ParseTaskMeta(f)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestGenerateReceiptID(t *testing.T) {
	id1 := GenerateReceiptID()
	id2 := GenerateReceiptID()
	if id1 == "" {
		t.Error("expected non-empty receipt ID")
	}
	if id1 == id2 {
		t.Error("expected unique receipt IDs")
	}
}

func TestWriteReceiptJSON(t *testing.T) {
	tmp := t.TempDir()
	receiptsDir := filepath.Join(tmp, "receipts")
	runsDir := filepath.Join(tmp, "runs")
	rw := NewReceiptWriter(receiptsDir, runsDir)

	receipt := &ForgeReceipt{
		ReceiptID:   "test-receipt-1",
		TaskID:      "test-task",
		Verdict:     VerdictSuccess,
		StartedAt:   time.Now(),
		FinishedAt:  time.Now(),
		DurationSec: 10,
		ScansPassed: true,
	}
	meta := &TaskMeta{ID: "test-task", Goal: "test"}

	err := rw.WriteReceipt(receipt, meta, "diff content", "PASS", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check receipts dir has a JSON file
	entries, _ := os.ReadDir(receiptsDir)
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			found = true
			data, _ := os.ReadFile(filepath.Join(receiptsDir, e.Name()))
			var parsed map[string]interface{}
			json.Unmarshal(data, &parsed)
			if parsed["receipt_id"] != "test-receipt-1" {
				t.Errorf("receipt_id=%v, want test-receipt-1", parsed["receipt_id"])
			}
			if parsed["verdict"] != VerdictSuccess {
				t.Errorf("verdict=%v, want %s", parsed["verdict"], VerdictSuccess)
			}
		}
	}
	if !found {
		t.Error("no JSON receipt file found")
	}

	// Check runs dir
	runReceipt := filepath.Join(runsDir, "run-test-task", "receipt.json")
	if _, err := os.Stat(runReceipt); os.IsNotExist(err) {
		t.Error("run receipt.json not found")
	}
}

func TestWriteReceiptMarkdown(t *testing.T) {
	tmp := t.TempDir()
	receiptsDir := filepath.Join(tmp, "receipts")
	runsDir := filepath.Join(tmp, "runs")
	rw := NewReceiptWriter(receiptsDir, runsDir)

	receipt := &ForgeReceipt{
		ReceiptID:   "test-md",
		TaskID:      "test-task",
		Verdict:     VerdictNoop,
		StartedAt:   time.Now(),
		FinishedAt:  time.Now(),
		DurationSec: 5,
		ScansPassed: true,
	}
	meta := &TaskMeta{ID: "test-task", Goal: "test"}

	err := rw.WriteReceipt(receipt, meta, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check markdown receipt exists
	entries, _ := os.ReadDir(receiptsDir)
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			found = true
			data, _ := os.ReadFile(filepath.Join(receiptsDir, e.Name()))
			content := string(data)
			if !contains(content, "NOOP_WITH_RECEIPT") {
				t.Error("markdown receipt should contain verdict")
			}
		}
	}
	if !found {
		t.Error("no markdown receipt file found")
	}
}

func TestWriteReviewMD(t *testing.T) {
	tmp := t.TempDir()
	runsDir := filepath.Join(tmp, "runs")
	rw := NewReceiptWriter(filepath.Join(tmp, "receipts"), runsDir)

	// WriteReviewMD writes review.md to filepath.Dir(taskPath)
	// so place task.md inside runsDir/<run-taskID>/
	runDir := filepath.Join(runsDir, "run-review-task")
	os.MkdirAll(runDir, 0755)
	taskPath := writeTempFile(t, "task.md", `id: "review-task"
goal: "review test"
`)
	// Move task.md into the run directory so WriteReviewMD puts review.md there
	finalTaskPath := filepath.Join(runDir, "task.md")
	os.Rename(taskPath, finalTaskPath)

	err := rw.WriteReviewMD(
		finalTaskPath,
		"review-task",
		"diff content",
		"some test output",
		[]string{"forbidden file modified"},
		VerdictNeedsHuman,
		"",
		nil,
		false,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	reviewPath := filepath.Join(runDir, "review.md")
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review.md not found: %v", err)
	}
	content := string(data)
	if !contains(content, "NEEDS_HUMAN") {
		t.Error("review.md should contain NEEDS_HUMAN")
	}
	if !contains(content, "forbidden file modified") {
		t.Error("review.md should contain safety hits")
	}
}

func TestVerdictConstants(t *testing.T) {
	if VerdictSuccess != "SUCCESS_WITH_RECEIPT" {
		t.Errorf("VerdictSuccess=%q", VerdictSuccess)
	}
	if VerdictSafety != "FAILED_SAFETY" {
		t.Errorf("VerdictSafety=%q", VerdictSafety)
	}
}

// --- helpers ---

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	os.WriteFile(path, []byte(content), 0644)
	return path
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
