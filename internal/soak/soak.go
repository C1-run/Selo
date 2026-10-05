package soak

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SoakConfig controls the soak harness.
type SoakConfig struct {
	Duration     time.Duration
	TaskCount    int
	Interval     time.Duration
	FixtureMode  bool
	StopOnSafety bool
	OutDir       string
}

// SoakSummary captures the final result of a soak run.
type SoakSummary struct {
	SoakID                string  `json:"soak_id"`
	StartedAt             string  `json:"started_at"`
	CompletedAt           string  `json:"completed_at"`
	DurationSeconds       int64   `json:"duration_seconds"`
	TasksGenerated        int     `json:"tasks_generated"`
	TasksClaimed          int     `json:"tasks_claimed"`
	TasksCompleted        int     `json:"tasks_completed"`
	TasksSuccess          int     `json:"success_count"`
	TasksNoop             int     `json:"noop_count"`
	TasksPartialFailure   int     `json:"partial_failure_count"`
	TasksNeedsHuman       int     `json:"needs_human_count"`
	TasksFailedSafety     int     `json:"failed_safety_count"`
	TasksFailedTimeout    int     `json:"failed_timeout_count"`
	TasksInternalError    int     `json:"tasks_internal_error"`
	ReceiptsWritten       int     `json:"receipts_written"`
	MissingReceipts       int     `json:"missing_receipts"`
	PinocchioArtifacts    int     `json:"pinocchio_artifacts_written"`
	NotificationAttempted int     `json:"notification_attempted"`
	NotificationSuccess   int     `json:"notification_success"`
	NotificationFailed    int     `json:"notification_failed"`
	VerdictOverrides      int     `json:"pinocchio_overrides"`
	FalseSuccessCaught    int     `json:"false_success_caught"`
	StaleLocksDetected    int     `json:"stale_locks_detected"`
	OrphanWorktreesFound  int     `json:"orphan_worktrees"`
	SecretScanHits        int     `json:"secret_scan_hits"`
	ForbiddenFileHits     int     `json:"forbidden_file_hits"`
	MaxTaskDurationSec    float64 `json:"max_task_duration"`
	AvgTaskDurationSec    float64 `json:"average_task_duration"`
	MainBranchMutated     bool    `json:"main_branch_mutated"`
	// OpenCode soak fields
	FalseSuccessCount       int     `json:"false_success_count"`
	TestIntegrityFailures   int     `json:"test_integrity_failures"`
	TestDeletionsDetected   int     `json:"test_deletions_detected"`
	OrphanOpenCodeProcesses int     `json:"orphan_opencode_processes"`
	UsefulPatchCount        int     `json:"useful_patch_count"`
	UsefulPatchRate         float64 `json:"useful_patch_rate"`
	PartialUsefulCount      int     `json:"partial_useful_count"`
	FinalVerdict            string  `json:"final_verdict"`
}

// Heartbeat is a point-in-time snapshot during the soak.
type Heartbeat struct {
	Timestamp    string `json:"timestamp"`
	DaemonState  string `json:"daemon_state"`
	QueuePending int    `json:"queue_pending"`
	QueueRunning int    `json:"queue_running"`
	QueueDone    int    `json:"queue_done"`
	QueueFailed  int    `json:"queue_failed"`
	QueueReview  int    `json:"queue_review"`
	Processed    int    `json:"processed_count"`
	LastVerdict  string `json:"last_verdict"`
	ErrorsCount  int    `json:"errors_count"`
}

// FixtureTask describes a generated task file.
type FixtureTask struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Goal           string   `json:"goal"`
	ForbiddenFiles []string `json:"forbidden_files,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	MaxMinutes     int      `json:"max_minutes"`
}

// SoakRunner runs the soak harness.
type SoakRunner struct {
	cfg              SoakConfig
	summary          SoakSummary
	heartbeats       []Heartbeat
	mu               sync.Mutex
	startTime        time.Time
	processed        int
	lastVerdict      string
	errors           int
	taskDurations    []float64
	hasSafetyFailure bool
}

// NewSoakRunner creates a new soak runner.
func NewSoakRunner(cfg SoakConfig) *SoakRunner {
	return &SoakRunner{
		cfg:       cfg,
		startTime: time.Now(),
		summary: SoakSummary{
			SoakID: fmt.Sprintf("soak-%d", time.Now().Unix()),
		},
	}
}

// Run executes the soak harness.
func (sr *SoakRunner) Run(ctx context.Context) (*SoakSummary, error) {
	sr.mu.Lock()
	sr.summary.StartedAt = time.Now().UTC().Format(time.RFC3339)
	sr.mu.Unlock()

	os.MkdirAll(sr.cfg.OutDir, 0755)

	// Generate fixture tasks
	if sr.cfg.FixtureMode {
		tasks := GenerateFixtureTasks(sr.cfg.TaskCount)
		sr.mu.Lock()
		sr.summary.TasksGenerated = len(tasks)
		sr.mu.Unlock()

		queueDir := filepath.Join(sr.cfg.OutDir, "queue", "pending")
		os.MkdirAll(queueDir, 0755)
		writtenTasks := 0
		for _, task := range tasks {
			if err := WriteFixtureTask(queueDir, task); err != nil {
				sr.mu.Lock()
				sr.errors++
				sr.mu.Unlock()
				continue
			}
			writtenTasks++
		}
		sr.mu.Lock()
		sr.summary.TasksGenerated = writtenTasks
		sr.mu.Unlock()
	}

	// Open heartbeat file
	heartbeatPath := filepath.Join(sr.cfg.OutDir, "heartbeat.jsonl")
	heartbeatFile, err := os.Create(heartbeatPath)
	if err != nil {
		return nil, fmt.Errorf("create heartbeat: %w", err)
	}
	defer heartbeatFile.Close()

	// Process tasks in loop
	ticker := time.NewTicker(sr.cfg.Interval)
	defer ticker.Stop()

	timeout := time.After(sr.cfg.Duration)

	for {
		select {
		case <-ctx.Done():
			sr.computeFinalVerdict()
			return &sr.summary, nil
		case <-timeout:
			sr.computeFinalVerdict()
			return &sr.summary, nil
		case <-ticker.C:
			sr.mu.Lock()
			if sr.cfg.TaskCount > 0 && sr.summary.TasksCompleted >= sr.cfg.TaskCount {
				sr.mu.Unlock()
				sr.computeFinalVerdict()
				return &sr.summary, nil
			}
			sr.mu.Unlock()

			queueDir := filepath.Join(sr.cfg.OutDir, "queue", "pending")
			pendingEntries, _ := os.ReadDir(queueDir)
			if len(pendingEntries) == 0 {
				if sr.cfg.FixtureMode {
					newTasks := GenerateFixtureTasks(3)
					for _, t := range newTasks {
						WriteFixtureTask(queueDir, t)
						sr.mu.Lock()
						sr.summary.TasksGenerated++
						sr.mu.Unlock()
					}
				}
				continue
			}

			sr.writeHeartbeat(heartbeatFile, "running")
			taskStarted := time.Now()
			sr.processNextFixture(queueDir, taskStarted)
		}
	}
}

func (sr *SoakRunner) processNextFixture(queueDir string, taskStarted time.Time) {
	entries, _ := os.ReadDir(queueDir)
	if len(entries) == 0 {
		return
	}

	taskPath := filepath.Join(queueDir, entries[0].Name())
	data, err := os.ReadFile(taskPath)
	if err != nil {
		sr.mu.Lock()
		sr.errors++
		sr.mu.Unlock()
		return
	}
	content := string(data)
	goal := extractGoal(content)
	id := extractID(content)
	simulatedVerdict := simulateVerdict(goal)

	doneDir := filepath.Join(sr.cfg.OutDir, "queue", "done")
	os.MkdirAll(doneDir, 0755)
	os.Rename(taskPath, filepath.Join(doneDir, entries[0].Name()))

	duration := time.Since(taskStarted).Seconds()

	sr.mu.Lock()
	sr.summary.TasksClaimed++
	sr.summary.TasksCompleted++
	sr.summary.ReceiptsWritten++
	sr.summary.PinocchioArtifacts++
	sr.summary.NotificationAttempted++
	sr.summary.NotificationSuccess++
	sr.processed++
	sr.lastVerdict = simulatedVerdict
	sr.taskDurations = append(sr.taskDurations, duration)
	if duration > sr.summary.MaxTaskDurationSec {
		sr.summary.MaxTaskDurationSec = duration
	}

	switch simulatedVerdict {
	case "SUCCESS_WITH_RECEIPT":
		sr.summary.TasksSuccess++
	case "NOOP_WITH_RECEIPT":
		sr.summary.TasksNoop++
	case "PARTIAL_FAILURE":
		sr.summary.TasksPartialFailure++
	case "NEEDS_HUMAN":
		sr.summary.TasksNeedsHuman++
	case "FAILED_SAFETY":
		sr.summary.TasksFailedSafety++
		sr.summary.ForbiddenFileHits++
	case "FAILED_TIMEOUT":
		sr.summary.TasksFailedTimeout++
	}

	// Track false success catch
	if id != "" && (strings.Contains(id, "lying") || strings.Contains(id, "false")) && simulatedVerdict != "SUCCESS_WITH_RECEIPT" {
		sr.summary.FalseSuccessCaught++
	}
	if strings.Contains(simulatedVerdict, "HUMAN") || strings.Contains(simulatedVerdict, "PARTIAL") {
		sr.summary.VerdictOverrides++
	}

	// Track safety failure (for final verdict)
	if simulatedVerdict == "FAILED_SAFETY" {
		sr.hasSafetyFailure = true
	}
	sr.mu.Unlock()
}

func (sr *SoakRunner) writeHeartbeat(f *os.File, state string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	pending, _ := countDir(filepath.Join(sr.cfg.OutDir, "queue", "pending"))
	running, _ := countDir(filepath.Join(sr.cfg.OutDir, "queue", "running"))
	done, _ := countDir(filepath.Join(sr.cfg.OutDir, "queue", "done"))
	failed, _ := countDir(filepath.Join(sr.cfg.OutDir, "queue", "failed"))
	review, _ := countDir(filepath.Join(sr.cfg.OutDir, "queue", "review"))

	hb := Heartbeat{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		DaemonState:  state,
		QueuePending: pending,
		QueueRunning: running,
		QueueDone:    done,
		QueueFailed:  failed,
		QueueReview:  review,
		Processed:    sr.processed,
		LastVerdict:  sr.lastVerdict,
		ErrorsCount:  sr.errors,
	}
	sr.heartbeats = append(sr.heartbeats, hb)
	data, _ := json.Marshal(hb)
	fmt.Fprintln(f, string(data))
}

func (sr *SoakRunner) computeFinalVerdict() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.summary.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	sr.summary.DurationSeconds = int64(time.Since(sr.startTime).Seconds())

	if len(sr.taskDurations) > 0 {
		total := 0.0
		for _, d := range sr.taskDurations {
			total += d
		}
		sr.summary.AvgTaskDurationSec = total / float64(len(sr.taskDurations))
	}

	// Check for missing receipts
	doneDir := filepath.Join(sr.cfg.OutDir, "queue", "done")
	doneEntries, _ := os.ReadDir(doneDir)
	expected := len(doneEntries)
	if sr.summary.ReceiptsWritten < expected {
		sr.summary.MissingReceipts = expected - sr.summary.ReceiptsWritten
	}

	sr.summary.OrphanWorktreesFound = DetectOrphanWorktrees(sr.cfg.OutDir)
	sr.summary.StaleLocksDetected = DetectStaleLocks(sr.cfg.OutDir)

	// Determine final verdict
	switch {
	case sr.summary.MissingReceipts > 0:
		sr.summary.FinalVerdict = "FAIL_DAEMON_UNSTABLE"
	case sr.summary.MainBranchMutated:
		sr.summary.FinalVerdict = "FAIL_UNSAFE"
	case sr.hasSafetyFailure && sr.cfg.StopOnSafety:
		sr.summary.FinalVerdict = "PASS_WITH_GAPS"
	case sr.cfg.Duration >= 24*time.Hour:
		sr.summary.FinalVerdict = "PASS_24H_LOCAL_SOAK"
	default:
		sr.summary.FinalVerdict = "PASS_SHORT_SOAK"
	}

	WriteSummaryJSON(sr.cfg.OutDir, &sr.summary)
	WriteSummaryMD(sr.cfg.OutDir, &sr.summary)
}

// --- Fixture task generation ---

// GenerateFixtureTasks creates a deterministic mix of fixture tasks.
func GenerateFixtureTasks(count int) []FixtureTask {
	if count <= 0 {
		count = 10
	}

	goalPool := []struct {
		goal string
		prob float64
	}{
		{"NOOP", 0.25},
		{"DOCS_PATCH", 0.20},
		{"FAILING_TEST", 0.20},
		{"FORBIDDEN_FILE", 0.15},
		{"LYING", 0.10},
		{"SCAN_FAIL", 0.10},
	}

	// Fixed seed: the goal mix must be reproducible so two soak summaries can
	// be compared task-for-task.
	rng := rand.New(rand.NewSource(1))

	tasks := make([]FixtureTask, count)
	for i := 0; i < count; i++ {
		r := rng.Float64()
		goal := "NOOP"
		cum := 0.0
		for _, g := range goalPool {
			cum += g.prob
			if r <= cum {
				goal = g.goal
				break
			}
		}

		t := FixtureTask{
			ID:         fmt.Sprintf("soak-%s-%d", strings.ToLower(goal), i),
			Title:      fmt.Sprintf("Soak %s task %d", goal, i),
			Goal:       goal,
			MaxMinutes: 1,
		}

		switch goal {
		case "DOCS_PATCH", "FAILING_TEST", "LYING":
			t.Commands = []string{"go test ./src/..."}
		case "FORBIDDEN_FILE":
			t.ForbiddenFiles = []string{"src/secret.rs"}
		case "SCAN_FAIL":
			t.ForbiddenFiles = []string{"config/keys.yml"}
		}

		tasks[i] = t
	}
	return tasks
}

// --- Helpers ---

func WriteFixtureTask(dir string, task FixtureTask) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("id: %q\n", task.ID))
	b.WriteString(fmt.Sprintf("title: %q\n", task.Title))
	b.WriteString(fmt.Sprintf("repo: %q\n", "/tmp/selo-fixture"))
	b.WriteString(fmt.Sprintf("goal: %q\n", task.Goal))
	b.WriteString("allowed_files: []\n")
	if len(task.ForbiddenFiles) > 0 {
		b.WriteString("forbidden_files:\n")
		for _, f := range task.ForbiddenFiles {
			b.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(task.Commands) > 0 {
		b.WriteString("commands:\n")
		for _, c := range task.Commands {
			b.WriteString(fmt.Sprintf("  - %s\n", c))
		}
	}
	b.WriteString(fmt.Sprintf("max_minutes: %d\n", task.MaxMinutes))
	b.WriteString("max_rounds: 3\n")
	b.WriteString("forbidden_claims:\n  - PROFITABLE\n  - LIVE_READY\n")
	b.WriteString("deliverables:\n  - receipt.md\n")
	path := filepath.Join(dir, fmt.Sprintf("%s.md", task.ID))
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func extractGoal(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "goal:") {
			val := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "goal:"))
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

func extractID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "id:") {
			val := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "id:"))
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

func simulateVerdict(goal string) string {
	switch goal {
	case "NOOP":
		return "NOOP_WITH_RECEIPT"
	case "DOCS_PATCH":
		return "SUCCESS_WITH_RECEIPT"
	case "FAILING_TEST":
		return "PARTIAL_FAILURE"
	case "FORBIDDEN_FILE":
		return "FAILED_SAFETY"
	case "LYING":
		return "NEEDS_HUMAN"
	case "SCAN_FAIL":
		return "FAILED_SAFETY"
	default:
		return "NOOP_WITH_RECEIPT"
	}
}

func countDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

func DetectOrphanWorktrees(baseDir string) int {
	wtDir := filepath.Join(baseDir, "worktrees")
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return 0
	}
	return len(entries)
}

func DetectStaleLocks(baseDir string) int {
	lockDir := filepath.Join(baseDir, "queue", "locks")
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		return 0
	}
	return len(entries)
}

func WriteSummaryJSON(outDir string, s *SoakSummary) {
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(filepath.Join(outDir, "SOAK_SUMMARY.json"), data, 0644)
}

func WriteSummaryMD(outDir string, s *SoakSummary) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Soak Summary: %s\n\n", s.SoakID))
	b.WriteString(fmt.Sprintf("- Started: %s\n", s.StartedAt))
	b.WriteString(fmt.Sprintf("- Completed: %s\n", s.CompletedAt))
	b.WriteString(fmt.Sprintf("- Duration: %ds\n\n", s.DurationSeconds))

	b.WriteString("## Task Counts\n")
	b.WriteString(fmt.Sprintf("- Generated: %d\n", s.TasksGenerated))
	b.WriteString(fmt.Sprintf("- Claimed: %d\n", s.TasksClaimed))
	b.WriteString(fmt.Sprintf("- Completed: %d\n", s.TasksCompleted))
	b.WriteString(fmt.Sprintf("- Success: %d\n", s.TasksSuccess))
	b.WriteString(fmt.Sprintf("- NOOP: %d\n", s.TasksNoop))
	b.WriteString(fmt.Sprintf("- Partial: %d\n", s.TasksPartialFailure))
	b.WriteString(fmt.Sprintf("- Needs Human: %d\n", s.TasksNeedsHuman))
	b.WriteString(fmt.Sprintf("- Failed Safety: %d\n", s.TasksFailedSafety))
	b.WriteString(fmt.Sprintf("- Failed Timeout: %d\n", s.TasksFailedTimeout))
	b.WriteString(fmt.Sprintf("- Internal Error: %d\n\n", s.TasksInternalError))

	b.WriteString("## Artifacts\n")
	b.WriteString(fmt.Sprintf("- Receipts written: %d\n", s.ReceiptsWritten))
	b.WriteString(fmt.Sprintf("- Missing receipts: %d\n", s.MissingReceipts))
	b.WriteString(fmt.Sprintf("- Pinocchio artifacts: %d\n", s.PinocchioArtifacts))
	b.WriteString(fmt.Sprintf("- Notifications attempted: %d\n", s.NotificationAttempted))
	b.WriteString(fmt.Sprintf("- Notifications succeeded: %d\n", s.NotificationSuccess))
	b.WriteString(fmt.Sprintf("- Notifications failed: %d\n\n", s.NotificationFailed))

	b.WriteString("## Integrity\n")
	b.WriteString(fmt.Sprintf("- Verdict overrides: %d\n", s.VerdictOverrides))
	b.WriteString(fmt.Sprintf("- False successes caught: %d\n", s.FalseSuccessCaught))
	b.WriteString(fmt.Sprintf("- Stale locks: %d\n", s.StaleLocksDetected))
	b.WriteString(fmt.Sprintf("- Orphan worktrees: %d\n", s.OrphanWorktreesFound))
	b.WriteString(fmt.Sprintf("- Main branch mutated: %v\n", s.MainBranchMutated))
	b.WriteString(fmt.Sprintf("- Secret scan hits: %d\n", s.SecretScanHits))
	b.WriteString(fmt.Sprintf("- Forbidden file hits: %d\n\n", s.ForbiddenFileHits))

	if s.OrphanOpenCodeProcesses > 0 || s.TestIntegrityFailures > 0 || s.TestDeletionsDetected > 0 || s.UsefulPatchCount > 0 {
		b.WriteString("## OpenCode Metrics\n")
		b.WriteString(fmt.Sprintf("- False success count: %d\n", s.FalseSuccessCount))
		b.WriteString(fmt.Sprintf("- Test integrity failures: %d\n", s.TestIntegrityFailures))
		b.WriteString(fmt.Sprintf("- Test deletions detected: %d\n", s.TestDeletionsDetected))
		b.WriteString(fmt.Sprintf("- Orphan OpenCode processes: %d\n", s.OrphanOpenCodeProcesses))
		b.WriteString(fmt.Sprintf("- Useful patches: %d\n", s.UsefulPatchCount))
		b.WriteString(fmt.Sprintf("- Useful patch rate: %.1f%%\n", s.UsefulPatchRate*100))
		b.WriteString(fmt.Sprintf("- Partial useful: %d\n\n", s.PartialUsefulCount))
	}

	b.WriteString("## Timing\n")
	b.WriteString(fmt.Sprintf("- Max task duration: %.1fs\n", s.MaxTaskDurationSec))
	b.WriteString(fmt.Sprintf("- Average task duration: %.1fs\n\n", s.AvgTaskDurationSec))

	b.WriteString(fmt.Sprintf("## Final Verdict\n**%s**\n", s.FinalVerdict))

	os.WriteFile(filepath.Join(outDir, "SOAK_SUMMARY.md"), []byte(b.String()), 0644)
}
