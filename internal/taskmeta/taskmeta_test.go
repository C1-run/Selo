package taskmeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	content := `id: test-123
title: Test Task
repo: /tmp/repo
goal: Fix the bug
acceptance_criteria:
  - Tests pass
  - Lint clean
commands:
  - go test ./...
  - go vet ./...
deliverables:
  - Fixed bug
max_minutes: 45
max_rounds: 5
max_files: 20
max_patch_lines: 500
allowed_files:
  - src/*.go
  - tests/*
forbidden_files:
  - secrets/*
  - .env
forbidden_claims:
  - production ready
  - bug-free
allow_test_modifications: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.ID != "test-123" {
		t.Errorf("ID = %q, want %q", cfg.ID, "test-123")
	}
	if cfg.Title != "Test Task" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Test Task")
	}
	if cfg.Repo != "/tmp/repo" {
		t.Errorf("Repo = %q, want %q", cfg.Repo, "/tmp/repo")
	}
	if cfg.Goal != "Fix the bug" {
		t.Errorf("Goal = %q, want %q", cfg.Goal, "Fix the bug")
	}
	if len(cfg.AcceptanceCriteria) != 2 {
		t.Errorf("AcceptanceCriteria len = %d, want 2", len(cfg.AcceptanceCriteria))
	}
	if len(cfg.Commands) != 2 {
		t.Errorf("Commands len = %d, want 2", len(cfg.Commands))
	}
	if len(cfg.Deliverables) != 1 {
		t.Errorf("Deliverables len = %d, want 1", len(cfg.Deliverables))
	}
	if cfg.MaxMinutes != 45 {
		t.Errorf("MaxMinutes = %d, want 45", cfg.MaxMinutes)
	}
	if cfg.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", cfg.MaxRounds)
	}
	if cfg.MaxFiles != 20 {
		t.Errorf("MaxFiles = %d, want 20", cfg.MaxFiles)
	}
	if cfg.MaxPatchLines != 500 {
		t.Errorf("MaxPatchLines = %d, want 500", cfg.MaxPatchLines)
	}
	if len(cfg.AllowedFiles) != 2 {
		t.Errorf("AllowedFiles len = %d, want 2", len(cfg.AllowedFiles))
	}
	if len(cfg.ForbiddenFiles) != 2 {
		t.Errorf("ForbiddenFiles len = %d, want 2", len(cfg.ForbiddenFiles))
	}
	if len(cfg.ForbiddenClaims) != 2 {
		t.Errorf("ForbiddenClaims len = %d, want 2", len(cfg.ForbiddenClaims))
	}
	if !cfg.AllowTestModifications {
		t.Error("AllowTestModifications = false, want true")
	}
}

func TestParseDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	content := `id: minimal
goal: Just a goal
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.MaxMinutes != 30 {
		t.Errorf("MaxMinutes = %d, want default 30", cfg.MaxMinutes)
	}
	if cfg.MaxRounds != 3 {
		t.Errorf("MaxRounds = %d, want default 3", cfg.MaxRounds)
	}
	if cfg.MaxFiles != 10 {
		t.Errorf("MaxFiles = %d, want default 10", cfg.MaxFiles)
	}
	if cfg.MaxPatchLines != 200 {
		t.Errorf("MaxPatchLines = %d, want default 200", cfg.MaxPatchLines)
	}
	if cfg.AllowTestModifications {
		t.Error("AllowTestModifications = true, want default false")
	}
}

func TestParseMissingFile(t *testing.T) {
	_, err := Parse("/nonexistent/task.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	content := `this is not valid task syntax without a key
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}
}

func TestParseQuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	content := `id: "quoted-id"
title: 'single-quoted'
forbidden_files:
  - "secrets/*"
  - 'config/*'
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.ID != "quoted-id" {
		t.Errorf("ID = %q, want %q", cfg.ID, "quoted-id")
	}
	if cfg.Title != "single-quoted" {
		t.Errorf("Title = %q, want %q", cfg.Title, "single-quoted")
	}
	if len(cfg.ForbiddenFiles) != 2 {
		t.Fatalf("ForbiddenFiles len = %d, want 2", len(cfg.ForbiddenFiles))
	}
	if cfg.ForbiddenFiles[0] != "secrets/*" {
		t.Errorf("ForbiddenFiles[0] = %q, want %q", cfg.ForbiddenFiles[0], "secrets/*")
	}
	if cfg.ForbiddenFiles[1] != "config/*" {
		t.Errorf("ForbiddenFiles[1] = %q, want %q", cfg.ForbiddenFiles[1], "config/*")
	}
}
