# Development Guide

Guide for developers contributing to C1 Forge.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Build and test |
| Git | 2.30+ | Version control |
| Docker | 20.10+ | Optional: container containment |
| OpenCode | 1.15+ | Optional: real AI execution |

## Setup

### 1. Clone Repository

```bash
git clone https://github.com/selo-dev/selo.git
cd selo
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Build

```bash
go build -o selo ./cmd/selo/
```

### 4. Run Tests

```bash
go test ./...
```

### 5. Run Vet

```bash
go vet ./...
```

---

## Project Structure

```
c1-forge/
├── cmd/
│   ├── selo/           # Main CLI
│   │   ├── main.go          # Entry point
│   │   ├── root.go          # Root command
│   │   ├── cmd_*.go         # Subcommands
│   │   ├── helpers.go       # Utilities
│   │   ├── smoke.go         # Smoke tests
│   │   └── soak_opencode.go # Soak tests
│   ├── c1-vps/              # VPS agent
│   └── v0-*/                # Legacy commands
├── internal/
│   ├── containment/         # Unified isolation
│   ├── opencode/            # OpenCode adapter
│   ├── pipeline/            # Safety pipeline
│   ├── receipt/             # Audit receipts
│   ├── governor/            # Policy enforcement
│   ├── pinocchio/           # Consistency checks
│   ├── gatechain/           # Compliance gates
│   ├── runner/              # C1 Loop runner
│   ├── queue/               # Task queue
│   ├── workspace/           # Git worktrees
│   └── ...
├── .opencode/               # OpenCode plugin
├── docs/                    # Documentation
├── config/                  # Example configs
├── scripts/                 # Helper scripts
└── go.mod                   # Go module
```

---

## Code Style

### Go Style

Follow standard Go conventions:

```go
// Good
func ProcessTask(task *Task) error {
    if task == nil {
        return errors.New("task is nil")
    }
    
    result, err := runPipeline(task)
    if err != nil {
        return fmt.Errorf("run pipeline: %w", err)
    }
    
    return writeReceipt(result)
}

// Bad
func processTask(t *Task) (e error) {
    if t == nil {
        e = errors.New("nil")
        return
    }
    // ...
}
```

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Packages | lowercase, single word | `queue`, `receipt` |
| Types | PascalCase | `QueueManager`, `ReceiptWriter` |
| Functions | PascalCase | `NewAdapter`, `ProcessTask` |
| Variables | camelCase | `taskPath`, `receiptID` |
| Constants | PascalCase | `VerdictPass`, `ActionStop` |
| Files | snake_case | `cmd_daemon.go`, `receipt_test.go` |

### Error Handling

```go
// Always check errors
result, err := doSomething()
if err != nil {
    return fmt.Errorf("do something: %w", err)
}

// Use descriptive error messages
return fmt.Errorf("create worktree for task %s: %w", taskID, err)
```

### Comments

```go
// ProcessTask processes a single task through the safety pipeline.
// It creates a worktree, runs the agent, and generates a receipt.
//
// Parameters:
//   - qm: Queue manager for task lifecycle
//   - rw: Receipt writer for audit trail
//   - wtm: Worktree manager for isolation
//   - cfg: Configuration
//
// Returns:
//   - true if a task was processed
//   - false if no tasks were pending
func processOneTask(qm, rw, wtm, cfg) bool {
    // ...
}
```

---

## Testing

### Test Structure

```
internal/
├── pipeline/
│   ├── pipeline.go
│   └── pipeline_test.go
├── receipt/
│   ├── receipt.go
│   └── receipt_test.go
└── ...
```

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/pipeline/...

# Verbose
go test -v ./internal/pipeline/...

# With race detector
go test -race ./...

# With timeout
go test -timeout 300s ./...
```

### Writing Tests

```go
package pipeline

import (
    "testing"
)

func TestPipeline_Run(t *testing.T) {
    // Arrange
    ctx := PipelineContext{
        TaskID:       "test-123",
        WorktreePath: t.TempDir(),
        Commands:     []string{"go test ./..."},
    }
    p := NewPipeline(ctx)
    
    // Act
    result, err := p.Run(context.Background())
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !result.Governor.Passed {
        t.Error("governor check failed")
    }
}

func TestValidateFilePath(t *testing.T) {
    tests := []struct {
        name      string
        path      string
        allowlist []string
        wantErr   bool
    }{
        {
            name:      "empty allowlist permits all",
            path:      "/any/path.go",
            allowlist: nil,
            wantErr:   false,
        },
        {
            name:      "exact match",
            path:      "/project/src/main.go",
            allowlist: []string{"/project/src/main.go"},
            wantErr:   false,
        },
        {
            name:      "no match",
            path:      "/project/secret.key",
            allowlist: []string{"/project/src/*.go"},
            wantErr:   true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateFilePath(tt.path, tt.allowlist)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Test Conventions

1. **Table-driven tests** for multiple cases
2. **Subtests** with `t.Run()`
3. **Test helpers** with `t.Helper()`
4. **Temp directories** with `t.TempDir()`
5. **Skip conditions** with `t.Skip()`

### Integration Tests

```go
func TestIntegration_FullPipeline(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Setup
    tmpDir := t.TempDir()
    repoDir := filepath.Join(tmpDir, "repo")
    initTestRepo(t, repoDir)
    
    // Create config
    cfg := &Config{...}
    
    // Run full pipeline
    qm := queue.NewQueueManager(tmpDir)
    rw := receipt.NewReceiptWriter(tmpDir)
    wtm := workspace.NewWorktreeManager(filepath.Join(tmpDir, "worktrees"))
    
    // Submit and process task
    taskPath, _ := qm.SubmitTask("test-task", "fix the bug")
    processed := processOneTask(qm, rw, wtm, cfg)
    
    if !processed {
        t.Error("no task processed")
    }
}
```

---

## Pull Request Process

### 1. Create Branch

```bash
git checkout -b feature/my-feature
```

### 2. Make Changes

Follow code style and add tests.

### 3. Run Checks

```bash
go test ./...
go vet ./...
```

### 4. Commit

```bash
git add .
git commit -m "Add my feature

- Add new functionality
- Add tests for new code
- Update documentation"
```

### 5. Push

```bash
git push origin feature/my-feature
```

### 6. Create PR

- Title: Clear description
- Description: What changed and why
- Tests: Confirm tests pass
- Review: Request review

---

## Issue Templates

### Bug Report

```markdown
## Bug Description

A clear description of the bug.

## Steps to Reproduce

1. Run `selo run "..."`
2. See error

## Expected Behavior

What should happen.

## Actual Behavior

What actually happens.

## Environment

- OS: macOS 14.0
- Go: 1.21
- C1 Forge: latest

## Logs

```
[paste logs here]
```
```

### Feature Request

```markdown
## Feature Description

A clear description of the feature.

## Use Case

Why is this feature needed?

## Proposed Solution

How should it work?

## Alternatives Considered

Other approaches considered.
```

---

## Release Process

### Versioning

Follow semantic versioning:

- **Major**: Breaking changes
- **Minor**: New features
- **Patch**: Bug fixes

### Release Steps

1. Update version in code
2. Update CHANGELOG.md
3. Create release branch
4. Run full test suite
5. Create PR and merge
6. Tag release: `git tag v1.0.0`
7. Push tag: `git push origin v1.0.0`
8. Create GitHub Release

---

## Debugging

### Enable Debug Logging

```bash
SELO_DEBUG=1 selo run "fix the bug"
```

### Common Issues

| Issue | Solution |
|-------|----------|
| `go.mod` not found | Run from repository root |
| Test timeout | Increase `-timeout` flag |
| Permission denied | Check file permissions |
| Worktree exists | Remove old worktrees: `git worktree prune` |

### Useful Commands

```bash
# List worktrees
git worktree list

# Remove stale worktrees
git worktree prune

# Check queue state
selo status

# View receipt
cat runs/run-*/receipt.json
```

---

## References

- [Architecture](../architecture/README.md) — System design
- [API Reference](../api/README.md) — Package documentation
- [Configuration](../configuration/README.md) — Config options
- [Deployment](../deployment/README.md) — Production setup
