package containment

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeContainment_Setup(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("skipping worktree test outside CI")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	os.MkdirAll(repoDir, 0755)

	runCmd(t, repoDir, "git", "init")
	runCmd(t, repoDir, "git", "config", "user.email", "test@test.com")
	runCmd(t, repoDir, "git", "config", "user.name", "Test")
	runCmd(t, repoDir, "git", "commit", "--allow-empty", "-m", "init")

	cfg := Config{
		Strategy: StrategyWorktree,
		WorkDir:  filepath.Join(tmpDir, "worktrees"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path, err := c.Setup(repoDir, "test-1")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if path == "" {
		t.Error("Setup returned empty path")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("worktree path does not exist: %s", path)
	}

	err = c.Teardown(repoDir, "test-1")
	if err != nil {
		t.Fatalf("Teardown: %v", err)
	}
}

func TestUnknownStrategyRefused(t *testing.T) {
	cfg := Config{
		Strategy: "docker",
		WorkDir:  t.TempDir(),
	}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported strategy")
	}
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Run()
}
