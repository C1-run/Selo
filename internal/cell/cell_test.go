package cell

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalCellRunsAndCapturesOutput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work")
	c := NewLocal(dir)
	res, err := c.Run([]string{"sh", "-c", "echo hello from cell; exit 0"}, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if res.Output == "" {
		t.Error("no output captured")
	}
}

func TestLocalCellNonZeroExit(t *testing.T) {
	c := NewLocal(t.TempDir())
	res, err := c.Run([]string{"sh", "-c", "exit 7"}, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 7 {
		t.Errorf("exit = %d, want 7", res.ExitCode)
	}
}

func TestLocalCellTimeout(t *testing.T) {
	c := NewLocal(t.TempDir())
	res, err := c.Run([]string{"sh", "-c", "sleep 30"}, nil, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut {
		t.Error("expected timeout")
	}
}

func TestLocalCellWorkdirIsolated(t *testing.T) {
	dir1 := filepath.Join(t.TempDir(), "a")
	dir2 := filepath.Join(t.TempDir(), "b")
	c1 := NewLocal(dir1)
	if _, err := c1.Run([]string{"sh", "-c", "echo x > marker.txt"}, nil, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "marker.txt")); err == nil {
		t.Error("cell leaked files to another directory")
	}
}