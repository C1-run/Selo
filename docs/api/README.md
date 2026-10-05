# API Reference

Complete Go package documentation for C1 Forge.

## Package Index

| Package | Description |
|---------|-------------|
| `cmd/selo` | CLI entry point and commands |
| `internal/containment` | Unified isolation interface |
| `internal/opencode` | Native Go OpenCode adapter |
| `internal/pipeline` | Safety pipeline |
| `internal/receipt` | Cryptographic receipts |
| `internal/governor` | Policy enforcement |
| `internal/pinocchio` | Consistency checks |
| `internal/gatechain` | Compliance gates |
| `internal/runner` | C1 Loop runner |
| `internal/queue` | Task queue |
| `internal/workspace` | Git worktree management |

---

## `cmd/selo`

### Functions

#### `Execute()`

```go
func Execute()
```

Runs the root Cobra command. Entry point for the CLI.

#### `loadConfig(configPath, baseDir string) *Config`

```go
func loadConfig(configPath string, baseDir string) *Config
```

Loads and parses the YAML configuration file.

**Parameters:**
- `configPath` — Path to config file (relative to `baseDir`)
- `baseDir` — Base directory for resolving paths

**Returns:**
- `*Config` — Parsed configuration with defaults applied

#### `processOneTask(qm, rw, wtm, cfg) bool`

```go
func processOneTask(
    qm *queue.QueueManager,
    rw *receipt.ReceiptWriter,
    wtm *workspace.WorktreeManager,
    cfg *Config,
) bool
```

Processes a single task from the queue. This is the core task execution function.

**Returns:**
- `true` if a task was processed (success or failure)
- `false` if no tasks were pending

---

## `internal/containment`

### Types

#### `Containment` (interface)

```go
type Containment interface {
    Setup(repoPath, taskID string) (string, error)
    Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
    Teardown(repoPath, taskID string) error
    Workdir() string
}
```

Unified interface for isolated task execution.

#### `Config`

```go
type Config struct {
    Strategy  Strategy
    WorkDir   string
    Image     string
    Writable  []string
    Extra     []string
}
```

Configuration for creating a Containment instance.

#### `Result`

```go
type Result struct {
    ExitCode int
    TimedOut bool
    Output   string
}
```

Result of a command execution.

#### `Strategy`

```go
type Strategy string

const (
    StrategyWorktree Strategy = "worktree"
    StrategyDocker   Strategy = "docker"
    StrategyLocal    Strategy = "local"
)
```

### Functions

#### `New(cfg Config) (Containment, error)`

```go
func New(cfg Config) (Containment, error)
```

Creates a Containment instance based on the strategy.

**Example:**

```go
c, err := containment.New(containment.Config{
    Strategy: containment.StrategyWorktree,
    WorkDir:  "/tmp/c1-forge/worktrees",
})

path, err := c.Setup("/path/to/repo", "task-123")
result, err := c.Run([]string{"go", "test", "./..."}, nil, 5*time.Minute)
err = c.Teardown("/path/to/repo", "task-123")
```

#### `DockerAvailable() bool`

```go
func DockerAvailable() bool
```

Reports whether a Docker daemon is reachable.

---

## `internal/opencode`

### Types

#### `Adapter`

```go
type Adapter struct {
    opts AdapterOpts
}
```

Wraps the OpenCode binary for C1 Forge task execution.

#### `AdapterOpts`

```go
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
```

Configuration for the OpenCode adapter.

#### `RunResult`

```go
type RunResult struct {
    ExitCode   int
    Stdout     string
    Stderr     string
    TimedOut   bool
    SessionID  string
    DurationMs int64
}
```

Outcome of a one-shot OpenCode run.

#### `ServerOpts`

```go
type ServerOpts struct {
    BinaryPath  string
    Port        int
    WorkDir     string
    Model       string
    Agent       string
    Permissions bool
    Allowlist   []string
}
```

Configuration for the OpenCode serve process.

#### `Client`

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}
```

REST API client for OpenCode.

### Functions

#### `NewAdapter(opts AdapterOpts) *Adapter`

```go
func NewAdapter(opts AdapterOpts) *Adapter
```

Creates a new OpenCode adapter.

#### `(a *Adapter) Run(ctx context.Context, goal string) *RunResult`

```go
func (a *Adapter) Run(ctx context.Context, goal string) *RunResult
```

Executes a task using the OpenCode binary.

#### `NewClient(baseURL string) *Client`

```go
func NewClient(baseURL string) *Client
```

Creates a new REST API client.

#### `(c *Client) CreateSession(ctx context.Context) (*Session, error)`

```go
func (c *Client) CreateSession(ctx context.Context) (*Session, error)
```

Creates a new OpenCode session.

#### `(c *Client) Prompt(ctx context.Context, sessionID string, req PromptRequest) (*PromptResponse, error)`

```go
func (c *Client) Prompt(ctx context.Context, sessionID string, req PromptRequest) (*PromptResponse, error)
```

Sends a prompt to a session.

#### `Start(ctx context.Context, opts ServerOpts) (*Server, error)`

```go
func Start(ctx context.Context, opts ServerOpts) (*Server, error)
```

Starts the OpenCode serve process.

#### `ValidateFilePath(path string, allowlist []string) error`

```go
func ValidateFilePath(path string, allowlist []string) error
```

Checks if a file path matches the allowlist patterns.

---

## `internal/pipeline`

### Types

#### `Pipeline`

```go
type Pipeline struct {
    Ctx PipelineContext
}
```

Safety pipeline for task verification.

#### `PipelineContext`

```go
type PipelineContext struct {
    TaskID          string
    WorktreePath    string
    AllowedFiles    []string
    ForbiddenFiles  []string
    ForbiddenClaims []string
    Commands        []string
    BaseDir         string
}
```

Context for pipeline execution.

#### `CheckResult`

```go
type CheckResult struct {
    Passed  bool
    Message string
    Details interface{}
}
```

Result of a pipeline check.

#### `PipelineResult`

```go
type PipelineResult struct {
    Governor  *CheckResult
    Scans     *CheckResult
    Pinocchio *CheckResult
    GateChain *CheckResult
}
```

Complete pipeline result.

### Functions

#### `NewPipeline(ctx PipelineContext) *Pipeline`

```go
func NewPipeline(ctx PipelineContext) *Pipeline
```

Creates a new pipeline instance.

#### `(p *Pipeline) Run(ctx context.Context) (*PipelineResult, error)`

```go
func (p *Pipeline) Run(ctx context.Context) (*PipelineResult, error)
```

Runs all pipeline stages sequentially.

---

## `internal/receipt`

### Types

#### `Receipt`

```go
type Receipt struct {
    ReceiptID             string            `json:"receipt_id"`
    TaskID                string            `json:"task_id"`
    Verdict               string            `json:"verdict"`
    StartedAt             time.Time         `json:"started_at"`
    FinishedAt            time.Time         `json:"finished_at"`
    DurationSec           float64           `json:"duration_sec"`
    RoundsUsed            int               `json:"rounds_used"`
    ExitCode              int               `json:"exit_code"`
    TimedOut              bool              `json:"timed_out"`
    FilesChanged          int               `json:"files_changed"`
    PatchLines            int               `json:"patch_lines"`
    HasDiff               bool              `json:"has_diff"`
    TestOutput            string            `json:"test_output"`
    RunnerMode            string            `json:"runner_mode"`
    RunnerCommand         string            `json:"runner_command"`
    RunnerBinaryKind      string            `json:"runner_binary_kind"`
    RunnerBinaryPath      string            `json:"runner_binary_path"`
    RunnerBinaryVersion   string            `json:"runner_binary_version"`
    RunnerBinaryVerified  bool              `json:"runner_binary_verified"`
    C1RuntimeRequested    string            `json:"c1_runtime_requested"`
    C1RuntimeUsed         string            `json:"c1_runtime_used"`
    C1RuntimeIsMock       bool              `json:"c1_runtime_is_mock"`
    WorktreePath          string            `json:"worktree_path"`
    BaseCommit            string            `json:"base_commit"`
    ScansPassed           bool              `json:"scans_passed"`
    PinocchioVerified     bool              `json:"pinocchio_verified"`
    PinocchioResultPath   string            `json:"pinocchio_result_path"`
    InitialVerdict        string            `json:"initial_verdict"`
    FinalVerdict          string            `json:"final_verdict"`
    VerdictOverridden     bool              `json:"verdict_overridden"`
    TestIntegrityPassed   bool              `json:"test_integrity_passed"`
    TestIntegrityResultPath string          `json:"test_integrity_result_path"`
    TestInventoryBeforeCount int            `json:"test_inventory_before_count"`
    TestInventoryAfterCount  int            `json:"test_inventory_after_count"`
    OpenCodeTimedOut      bool              `json:"opencode_timed_out"`
    NotificationMode      string            `json:"notification_mode"`
    NotificationSuccess   bool              `json:"notification_success"`
    GateChainAction       string            `json:"gatechain_action"`
    GateChainStopRequired bool              `json:"gatechain_stop_required"`
    GateChainReviewRequired bool            `json:"gatechain_review_required"`
    GateChainFinalStatus  string            `json:"gatechain_final_status"`
    GateChainActionReason string            `json:"gatechain_action_reason"`
}
```

Cryptographic audit trail for a task.

#### `ReceiptWriter`

```go
type ReceiptWriter struct {
    BaseDir string
}
```

Writes receipts to disk.

#### `TaskMeta`

```go
type TaskMeta struct {
    ID             string   `yaml:"id"`
    Goal           string   `yaml:"goal"`
    Repo           string   `yaml:"repo"`
    MaxMinutes     int      `yaml:"max_minutes"`
    MaxRounds      int      `yaml:"max_rounds"`
    MaxFiles       int      `yaml:"max_files"`
    MaxPatchLines  int      `yaml:"max_patch_lines"`
    Commands       []string `yaml:"commands"`
    ForbiddenFiles []string `yaml:"forbidden_files"`
}
```

Metadata from a task file.

### Functions

#### `NewReceiptWriter(baseDir string) *ReceiptWriter`

```go
func NewReceiptWriter(baseDir string) *ReceiptWriter
```

Creates a new receipt writer.

#### `(rw *ReceiptWriter) WriteReceipt(receipt *Receipt) error`

```go
func (rw *ReceiptWriter) WriteReceipt(receipt *Receipt) error
```

Writes a receipt to disk (JSON + Markdown).

#### `ParseTaskMeta(taskPath string) (*TaskMeta, error)`

```go
func ParseTaskMeta(taskPath string) (*TaskMeta, error)
```

Parses task metadata from a YAML file.

---

## `internal/queue`

### Types

#### `QueueManager`

```go
type QueueManager struct {
    BaseDir string
}
```

Manages the task queue.

### Functions

#### `NewQueueManager(baseDir string) *QueueManager`

```go
func NewQueueManager(baseDir string) *QueueManager
```

Creates a new queue manager.

#### `(qm *QueueManager) ScanPending() ([]string, error)`

```go
func (qm *QueueManager) ScanPending() ([]string, error)
```

Returns paths to pending tasks.

#### `(qm *QueueManager) ClaimTask(taskPath string) (string, error)`

```go
func (qm *QueueManager) ClaimTask(taskPath string) (string, error)
```

Atomically claims a task by moving it to running/.

#### `(qm *QueueManager) MoveTask(from, to string) error`

```go
func (qm *QueueManager) MoveTask(from, to string) error
```

Moves a task between queue directories.

---

## `internal/workspace`

### Types

#### `WorktreeManager`

```go
type WorktreeManager struct {
    WorkDir string
}
```

Creates and removes git worktrees.

### Functions

#### `NewWorktreeManager(workDir string) *WorktreeManager`

```go
func NewWorktreeManager(workDir string) *WorktreeManager
```

Creates a new worktree manager.

#### `(wm *WorktreeManager) CreateWorktree(repoPath, taskID string) (string, error)`

```go
func (wm *WorktreeManager) CreateWorktree(repoPath, taskID string) (string, error)
```

Creates a git worktree for a task.

#### `(wm *WorktreeManager) RemoveWorktree(repoPath, taskID string) error`

```go
func (wm *WorktreeManager) RemoveWorktree(repoPath, taskID string) error
```

Removes a git worktree.

#### `(wm *WorktreeManager) CleanupAll(repoPath string) error`

```go
func (wm *WorktreeManager) CleanupAll(repoPath string) error
```

Removes all c1-forge worktrees.

---

## Error Types

### Common Errors

| Error | Package | Description |
|-------|---------|-------------|
| `ErrTaskNotFound` | `queue` | Task not in queue |
| `ErrTaskAlreadyClaimed` | `queue` | Task already running |
| `ErrWorktreeExists` | `workspace` | Worktree already exists |
| `ErrBinaryNotFound` | `opencode` | OpenCode binary not found |
| `ErrSessionNotFound` | `opencode` | Session doesn't exist |
| `ErrPipelineFailed` | `pipeline` | Safety check failed |
| `ErrGateChainBlocked` | `gatechain` | Task blocked by policy |

### Error Handling Pattern

```go
result, err := adapter.Run(ctx, goal)
if err != nil {
    log.Printf("adapter error: %v", err)
    // Handle error
}

if result.ExitCode != 0 {
    log.Printf("task failed with exit code %d", result.ExitCode)
    // Handle failure
}
```

---

## Constants

### Verdicts

```go
const (
    VerdictPass          = "PASS"
    VerdictReview        = "NEEDS_HUMAN"
    VerdictStop          = "STOP"
    VerdictNoopWithReceipt = "NOOP_WITH_RECEIPT"
)
```

### Pipeline Actions

```go
const (
    ActionPass   = "pass"
    ActionReview = "review"
    ActionStop   = "stop"
)
```

---

## Examples

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "github.com/desmondkam/openselo/internal/opencode"
)

func main() {
    adapter := opencode.NewAdapter(opencode.AdapterOpts{
        BinaryPath: "/usr/local/bin/opencode",
        WorkDir:    "/tmp/my-project",
        Model:      "anthropic/claude-sonnet-4-5",
    })
    
    result := adapter.Run(context.Background(), "fix the login bug")
    
    fmt.Printf("Exit code: %d\n", result.ExitCode)
    fmt.Printf("Duration: %dms\n", result.DurationMs)
}
```

### Containment

```go
package main

import (
    "fmt"
    "github.com/desmondkam/openselo/internal/containment"
)

func main() {
    c, err := containment.New(containment.Config{
        Strategy: containment.StrategyWorktree,
        WorkDir:  "/tmp/c1-forge/worktrees",
    })
    if err != nil {
        panic(err)
    }
    
    path, err := c.Setup("/path/to/repo", "task-123")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Worktree: %s\n", path)
    
    result, err := c.Run([]string{"go", "test", "./..."}, nil, 5*time.Minute)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Exit code: %d\n", result.ExitCode)
    
    c.Teardown("/path/to/repo", "task-123")
}
```

### Pipeline

```go
package main

import (
    "context"
    "github.com/desmondkam/openselo/internal/pipeline"
)

func main() {
    p := pipeline.NewPipeline(pipeline.PipelineContext{
        TaskID:       "task-123",
        WorktreePath: "/tmp/c1-forge/worktrees/wt-task-123",
        Commands:     []string{"go test ./..."},
    })
    
    result, err := p.Run(context.Background())
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Governor: %v\n", result.Governor.Passed)
    fmt.Printf("Scans: %v\n", result.Scans.Passed)
    fmt.Printf("Pinocchio: %v\n", result.Pinocchio.Passed)
    fmt.Printf("GateChain: %v\n", result.GateChain.Passed)
}
```
