package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LockFile represents a pid-based lock file for a running task.
type LockFile struct {
	Path      string
	PID       int
	Timestamp time.Time
	TaskID    string
}

// WriteLock creates a lock file with PID and timestamp.
func WriteLock(path, taskID string) (*LockFile, error) {
	os.MkdirAll(filepath.Dir(path), 0755)
	content := fmt.Sprintf("%d\n%d\n%s\n", os.Getpid(), time.Now().Unix(), taskID)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write lock: %w", err)
	}
	return &LockFile{
		Path:      path,
		PID:       os.Getpid(),
		Timestamp: time.Now(),
		TaskID:    taskID,
	}, nil
}

// ReadLock reads and parses an existing lock file.
func ReadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock: %w", err)
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 3)
	if len(lines) < 3 {
		return nil, fmt.Errorf("malformed lock file: %s", path)
	}
	pid, _ := strconv.Atoi(lines[0])
	ts, _ := strconv.ParseInt(lines[1], 10, 64)
	return &LockFile{
		Path:      path,
		PID:       pid,
		Timestamp: time.Unix(ts, 0),
		TaskID:    lines[2],
	}, nil
}

// IsStale checks if the lock's PID is no longer alive.
func (l *LockFile) IsStale() bool {
	// Use syscall.Kill with signal 0 to check if process exists.
	// This works reliably on both Linux and macOS.
	if err := syscall.Kill(l.PID, 0); err != nil {
		return true
	}
	return false
}

// Remove deletes the lock file.
func (l *LockFile) Remove() error {
	return os.Remove(l.Path)
}

// QueueManager manages the file-based task queue using atomic rename.
type QueueManager struct {
	BaseDir string
}

// NewQueueManager creates a new queue manager rooted at the given directory.
func NewQueueManager(baseDir string) *QueueManager {
	return &QueueManager{BaseDir: baseDir}
}

func (qm *QueueManager) PendingDir() string  { return filepath.Join(qm.BaseDir, "queue", "pending") }
func (qm *QueueManager) RunningDir() string  { return filepath.Join(qm.BaseDir, "queue", "running") }
func (qm *QueueManager) ReviewDir() string   { return filepath.Join(qm.BaseDir, "queue", "review") }
func (qm *QueueManager) DoneDir() string     { return filepath.Join(qm.BaseDir, "queue", "done") }
func (qm *QueueManager) FailedDir() string   { return filepath.Join(qm.BaseDir, "queue", "failed") }
func (qm *QueueManager) LockDir() string     { return filepath.Join(qm.BaseDir, "queue", "running") }
func (qm *QueueManager) RunsDir() string     { return filepath.Join(qm.BaseDir, "runs") }
func (qm *QueueManager) ReceiptsDir() string { return filepath.Join(qm.BaseDir, "receipts") }

// ScanPending returns all task.md files in the pending queue, sorted by name.
func (qm *QueueManager) ScanPending() ([]string, error) {
	dir := qm.PendingDir()
	os.MkdirAll(dir, 0755)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan pending: %w", err)
	}
	var tasks []string
	for _, e := range entries {
		if !e.IsDir() && (e.Name() == "task.md" || strings.HasSuffix(e.Name(), ".md")) {
			tasks = append(tasks, filepath.Join(dir, e.Name()))
		}
	}
	return tasks, nil
}

// ClaimTask atomically renames a task file from pending to running.
// Returns the new path in the running directory.
func (qm *QueueManager) ClaimTask(taskPath string) (string, error) {
	runningDir := qm.RunningDir()
	os.MkdirAll(runningDir, 0755)
	_, taskFile := filepath.Split(taskPath)
	dest := filepath.Join(runningDir, taskFile)
	if err := os.Rename(taskPath, dest); err != nil {
		return "", fmt.Errorf("claim task (rename %s -> %s): %w", taskPath, dest, err)
	}
	return dest, nil
}

// MoveTask moves a task file from running to the target directory.
func (qm *QueueManager) MoveTask(taskPath, targetDir string) (string, error) {
	os.MkdirAll(targetDir, 0755)
	_, taskFile := filepath.Split(taskPath)
	dest := filepath.Join(targetDir, taskFile)
	if err := os.Rename(taskPath, dest); err != nil {
		return "", fmt.Errorf("move task: %w", err)
	}
	return dest, nil
}

// DaemonLockPath returns the path for the daemon's own lock file.
func (qm *QueueManager) DaemonLockPath() string {
	return filepath.Join(qm.BaseDir, ".selo.lock")
}

// StopFilePath returns the path for the stop signal file.
func (qm *QueueManager) StopFilePath() string {
	return filepath.Join(qm.BaseDir, ".selo-stop")
}
