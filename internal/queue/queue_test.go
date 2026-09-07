package queue

import (
	"os"
	"testing"
)

func TestIsStaleNonExistentPID(t *testing.T) {
	lf := &LockFile{PID: 99999999, Path: "/tmp/test.lock"}
	if !lf.IsStale() {
		t.Error("expected stale lock for non-existent PID")
	}
}

func TestIsStaleCurrentPID(t *testing.T) {
	lf := &LockFile{PID: os.Getpid(), Path: "/tmp/test2.lock"}
	if lf.IsStale() {
		t.Error("current process should not be stale")
	}
}

func TestWriteAndReadLock(t *testing.T) {
	path := "/tmp/test_write_read.lock"
	defer os.Remove(path)

	lf, err := WriteLock(path, "test-task")
	if err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	if lf.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), lf.PID)
	}
	if lf.TaskID != "test-task" {
		t.Errorf("expected task test-task, got %s", lf.TaskID)
	}

	read, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if read.PID != os.Getpid() {
		t.Errorf("read PID mismatch: %d", read.PID)
	}
	if read.TaskID != "test-task" {
		t.Errorf("read task mismatch: %s", read.TaskID)
	}
}
