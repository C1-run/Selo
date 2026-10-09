# Architecture

C1 Forge architecture, system design, and component relationships.

## System Overview

C1 Forge is a safety-first task runner that wraps coding agents and produces cryptographic audit trails. It consists of:

1. **CLI** — User interface (Cobra)
2. **Daemon** — Background task processor
3. **Pipeline** — Safety verification (4 stages)
4. **Adapter** — Agent execution (OpenCode, mock, etc.)
5. **Receipt** — Cryptographic audit trail

## Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        User Input                           │
│  CLI: selo run "fix the bug"                          │
│  Plugin: c1forge_submit_task goal="fix the bug"            │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                     Task Queue                              │
│  .selo/queue/pending/   →  Task files (.md)            │
│  .selo/queue/running/   →  Active tasks                │
│  .selo/queue/done/      →  Completed tasks             │
│  .selo/queue/failed/    →  Failed tasks                │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  Worktree Creation                          │
│  git worktree add -b c1-forge/<task-id> <path> HEAD        │
│  Isolated directory for agent execution                     │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  Agent Execution                            │
│  OpenCode: Native Go adapter (REST API)                     │
│  Mock: fixture_agent.sh (testing)                           │
│  C1 Loop: Shell adapter (legacy)                            │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  Safety Pipeline                            │
│  ┌─────────┐  ┌─────────┐  ┌──────────┐  ┌───────────┐   │
│  │Governor │→ │ Scans   │→ │Pinocchio │→ │GateChain  │   │
│  └─────────┘  └─────────┘  └──────────┘  └───────────┘   │
│  - Policies   - Secrets    - Consistency   - Compliance    │
│  - Claims     - Files      - Tests         - Receipts     │
│  - Timeout    - Forbidden  - Phantom       - Verdict      │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                    Receipt                                   │
│  runs/<task-id>/receipt.json  — Machine-readable            │
│  runs/<task-id>/receipt.md    — Human-readable              │
│  runs/<task-id>/pinocchio.json — Consistency check          │
│  runs/<task-id>/test_integrity.json — Test inventory        │
└─────────────────────────────────────────────────────────────┘
```

## Component Details

### CLI (`cmd/selo/`)

**File Structure:**

| File | Purpose |
|------|---------|
| `root.go` | Root command, global flags (`--config`) |
| `cmd_daemon.go` | Daemon mode (`--one-shot`) |
| `cmd_run.go` | One-shot task execution |
| `cmd_status.go` | Queue/daemon status (`--json`) |
| `cmd_init.go` | Workspace initialization |
| `cmd_check.go` | GateChain compliance check |
| `cmd_smoke.go` | Smoke test dispatch |
| `cmd_soak.go` | Soak test dispatch |
| `cmd_completion.go` | Shell completions |
| `helpers.go` | Shared utilities |
| `smoke.go` | Smoke test functions |
| `soak_opencode.go` | Soak test helpers |

**Entry Point:**

```go
// main.go
func main() {
    Execute() // Cobra root command
}
```

**Global Flags:**

```bash
--config string   Path to config file (default "config/selo.yaml")
-h, --help       Help
```

### Daemon (`cmd/selo/cmd_daemon.go`)

The daemon continuously processes tasks from the queue:

```go
func runDaemon(cfg *Config, baseDir string, oneShot bool) {
    qm := queue.NewQueueManager(baseDir)
    rw := receipt.NewReceiptWriter(baseDir)
    wtm := workspace.NewWorktreeManager(filepath.Join(baseDir, "worktrees"))
    
    for {
        processed := processOneTask(qm, rw, wtm, cfg)
        if oneShot && !processed {
            break
        }
        time.Sleep(time.Duration(cfg.Forge.PollIntervalSec) * time.Second)
    }
}
```

**Task Processing:**

```go
func processOneTask(qm, rw, wtm, cfg) bool {
    // 1. Scan pending tasks
    // 2. Claim task (atomic rename)
    // 3. Parse task metadata
    // 4. Create lock file
    // 5. Resolve configuration
    // 6. Create worktree
    // 7. Run agent (OpenCode or C1 Loop)
    // 8. Capture diff
    // 9. Run safety pipeline
    // 10. Write receipt
    // 11. Cleanup worktree
}
```

### Pipeline (`internal/pipeline/`)

**Interface:**

```go
type Pipeline struct {
    Ctx PipelineContext
}

type PipelineContext struct {
    TaskID        string
    WorktreePath  string
    AllowedFiles  []string
    ForbiddenFiles []string
    ForbiddenClaims []string
    Commands      []string
    BaseDir       string
}

type CheckResult struct {
    Passed  bool
    Message string
    Details interface{}
}
```

**Pipeline Stages:**

```go
func (p *Pipeline) Run(ctx context.Context) (*PipelineResult, error) {
    // Stage 1: Governor
    governorResult := p.runGovernor(ctx)
    
    // Stage 2: Scans
    scanResult := p.runScans(ctx)
    
    // Stage 3: Pinocchio
    pinocchioResult := p.runPinocchio(ctx)
    
    // Stage 4: GateChain
    gatechainResult := p.runGateChain(ctx)
    
    return &PipelineResult{
        Governor:  governorResult,
        Scans:     scanResult,
        Pinocchio: pinocchioResult,
        GateChain: gatechainResult,
    }, nil
}
```

### Adapter (`internal/opencode/`)

**Interface:**

```go
type Adapter struct {
    opts AdapterOpts
}

type AdapterOpts struct {
    BinaryPath      string
    WorkDir         string
    TaskFilePath    string
    Model           string
    Agent           string
    MaxMinutes      int
    ServeTimeoutSec int
    RunTimeoutSec   int
    Permissions     bool
    Allowlist       []string
}

func (a *Adapter) Run(ctx context.Context, goal string) *RunResult
```

**Execution Flow:**

```go
func (a *Adapter) Run(ctx context.Context, goal string) *RunResult {
    // 1. Discover OpenCode binary
    binPath := a.resolveBinary()
    
    // 2. Start serve process
    server, _ := Start(ctx, ServerOpts{...})
    
    // 3. Create REST client
    client := NewClient(server.URL())
    
    // 4. Create session
    session, _ := client.CreateSession(ctx)
    
    // 5. Send prompt
    response, _ := client.Prompt(ctx, session.ID, PromptRequest{Prompt: goal})
    
    // 6. Capture output
    // 7. Return result
}
```

### Receipt (`internal/receipt/`)

**Structure:**

```go
type Receipt struct {
    ReceiptID        string            `json:"receipt_id"`
    TaskID           string            `json:"task_id"`
    Verdict          string            `json:"verdict"`
    StartedAt        time.Time         `json:"started_at"`
    FinishedAt       time.Time         `json:"finished_at"`
    DurationSec      float64           `json:"duration_sec"`
    RoundsUsed       int               `json:"rounds_used"`
    ExitCode         int               `json:"exit_code"`
    TimedOut         bool              `json:"timed_out"`
    FilesChanged     int               `json:"files_changed"`
    PatchLines       int               `json:"patch_lines"`
    HasDiff          bool              `json:"has_diff"`
    TestOutput       string            `json:"test_output"`
    RunnerMode       string            `json:"runner_mode"`
    RunnerCommand    string            `json:"runner_command"`
    RunnerBinaryKind string            `json:"runner_binary_kind"`
    RunnerBinaryPath string            `json:"runner_binary_path"`
    RunnerBinaryVersion string         `json:"runner_binary_version"`
    RunnerBinaryVerified bool          `json:"runner_binary_verified"`
    C1RuntimeRequested string          `json:"c1_runtime_requested"`
    C1RuntimeUsed    string            `json:"c1_runtime_used"`
    C1RuntimeIsMock  bool              `json:"c1_runtime_is_mock"`
    WorktreePath     string            `json:"worktree_path"`
    BaseCommit       string            `json:"base_commit"`
    ScansPassed      bool              `json:"scans_passed"`
    PinocchioVerified bool             `json:"pinocchio_verified"`
    PinocchioResultPath string         `json:"pinocchio_result_path"`
    InitialVerdict   string            `json:"initial_verdict"`
    FinalVerdict     string            `json:"final_verdict"`
    VerdictOverridden bool             `json:"verdict_overridden"`
    TestIntegrityPassed bool           `json:"test_integrity_passed"`
    TestIntegrityResultPath string     `json:"test_integrity_result_path"`
    TestInventoryBeforeCount int       `json:"test_inventory_before_count"`
    TestInventoryAfterCount int        `json:"test_inventory_after_count"`
    OpenCodeTimedOut bool              `json:"opencode_timed_out"`
    NotificationMode string            `json:"notification_mode"`
    NotificationSuccess bool           `json:"notification_success"`
    GateChainAction  string            `json:"gatechain_action"`
    GateChainStopRequired bool         `json:"gatechain_stop_required"`
    GateChainReviewRequired bool       `json:"gatechain_review_required"`
    GateChainFinalStatus string        `json:"gatechain_final_status"`
    GateChainActionReason string       `json:"gatechain_action_reason"`
}
```

### Queue (`internal/queue/`)

**Directory Structure:**

```
.selo/queue/
├── pending/           # New tasks
│   └── run-abc123.md
├── running/           # Active tasks
│   └── run-abc123.md
├── done/              # Completed tasks
│   └── task-run-abc123.md
├── failed/            # Failed tasks
│   └── task-run-abc123.md
└── review/            # Needs human review
    └── task-run-abc123.md
```

**Task File Format:**

```markdown
---
id: run-abc123
goal: Fix the login bug
repo: /path/to/repo
max_minutes: 30
max_rounds: 3
commands:
  - go test ./...
forbidden_files:
  - secrets.yaml
---

Fix the login bug that causes timeout errors.
```

### Containment (`internal/containment/`)

**Interface:**

```go
type Containment interface {
    Setup(repoPath, taskID string) (string, error)
    Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
    Teardown(repoPath, taskID string) error
    Workdir() string
}
```

**Implementations:**

| Type | File | Isolation |
|------|------|-----------|
| `WorktreeContainment` | `containment.go` | Git worktree |
| `DockerContainment` | `containment.go` | Container |
| `LocalContainment` | `containment.go` | Process |

**Factory:**

```go
func New(cfg Config) (Containment, error) {
    switch cfg.Strategy {
    case StrategyWorktree:
        return &WorktreeContainment{workDir: cfg.WorkDir}, nil
    case StrategyDocker:
        return &DockerContainment{...}, nil
    case StrategyLocal:
        return &LocalContainment{workDir: cfg.WorkDir}, nil
    }
}
```

## Concurrency Model

### Queue Locking

Tasks are locked using filesystem locks:

```go
type Lock struct {
    Path   string
    TaskID string
    File   *os.File
}

func WriteLock(path, taskID string) (*Lock, error) {
    // Atomic create + write
}

func (l *Lock) Remove() error {
    // Release lock
}
```

### Worktree Isolation

Each task gets its own worktree:

```
.selo/worktrees/
├── wt-run-abc123/    # Task 1
├── wt-run-def456/    # Task 2
└── wt-run-ghi789/    # Task 3
```

### Daemon Polling

```go
for {
    processed := processOneTask(...)
    if oneShot && !processed {
        break
    }
    time.Sleep(pollInterval)
}
```

## Error Handling

### Error Categories

| Category | Example | Action |
|----------|---------|--------|
| Transient | Network timeout | Retry |
| Permanent | Invalid config | Fail fast |
| Safety | Secret detected | Block task |
| Resource | Disk full | Alert user |

### Error Propagation

```go
func processOneTask(...) bool {
    tasks, err := qm.ScanPending()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        return false // Retry on next poll
    }
    
    runningPath, err := qm.ClaimTask(taskPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        qm.MoveTask(taskPath, qm.FailedDir())
        return true // Task failed
    }
    // ...
}
```

## Performance Characteristics

| Operation | Complexity | Notes |
|-----------|------------|-------|
| Task scan | O(n) | n = pending tasks |
| Worktree create | O(1) | Git worktree add |
| Safety pipeline | O(m) | m = files changed |
| Receipt write | O(1) | JSON + Markdown |

## Security Model

### Trust Boundaries

1. **User → CLI**: Trusted input
2. **CLI → Daemon**: Local IPC
3. **Daemon → Agent**: Isolated execution
4. **Agent → Files**: Contained (worktree/container)
5. **Pipeline → Verdict**: Signed, tamper-evident receipts

### Permission Model

```yaml
forge:
  opencode:
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/project/src/*.go"
      - "/project/tests/*"
```

### Secret Detection

The scans stage detects:
- API keys (AWS, GCP, Azure, etc.)
- Hardcoded credentials
- Private keys
- Connection strings

## Extension Points

### Custom Containment

Implement the `Containment` interface:

```go
type MyContainment struct {
    // ...
}

func (c *MyContainment) Setup(repoPath, taskID string) (string, error) { ... }
func (c *MyContainment) Run(cmd []string, env []string, timeout time.Duration) (*Result, error) { ... }
func (c *MyContainment) Teardown(repoPath, taskID string) error { ... }
func (c *MyContainment) Workdir() string { ... }
```

### Custom Pipeline Stage

Add a new stage to the pipeline:

```go
func (p *Pipeline) runCustomCheck(ctx context.Context) *CheckResult {
    // Implement check logic
    return &CheckResult{
        Passed:  true,
        Message: "Custom check passed",
    }
}
```

### Custom Receipt Field

Extend the receipt struct:

```go
type Receipt struct {
    // ... existing fields
    CustomField string `json:"custom_field,omitempty"`
}
```

## References

- [API Reference](../api/README.md) — Package documentation
- [Configuration](../configuration/README.md) — Config options
- [Development](../development/README.md) — Contributing guide
