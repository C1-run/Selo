# C1 Forge Phase 0.1 — Real Runner Wiring

## Overview

Phase 0.1 replaces the Phase 0 placeholder runner (`echo "C1 Loop: ..."`) with a configurable real runner path while preserving all safety gates. The daemon now supports two runner modes:

- **mock**: Placeholder echo command (backward compatible, default)
- **real**: Executes a configured external command (e.g., `opencode`, `c1`) with template substitution

## Changes

### 1. Runner Modes (`internal/runner/runner.go`)

```go
type RunnerMode string
const ModeReal RunnerMode = "real"
const ModeMock RunnerMode = "mock"

type RunnerConfig struct {
    Command string
    Args    []string
}
```

- `C1LoopRunner.Run()` now dispatches to `runReal()` or `runMock()` based on `Mode`
- `runReal()` substitutes `{{task_file}}`, `{{worktree}}`, `{{task_id}}`, `{{max_minutes}}` in command args
- `NewMockRunner()` creates a backward-compatible mock runner

### 2. Verdict Mapping (`runner.MapVerdict`)

Canonical verdict mapping:

| Condition | Verdict |
|---|---|
| Safety hit with "forbidden" or "secret" | `FAILED_SAFETY` |
| Safety hit with "exceeded" or "too many" | `FAILED_LIMIT_EXCEEDED` |
| Non-critical safety hits | `NEEDS_HUMAN` |
| Timed out | `FAILED_TIMEOUT` |
| Limit violation | `FAILED_LIMIT_EXCEEDED` |
| Exit non-zero (+ diff or test output) | `PARTIAL_FAILURE` |
| Exit 0 + meaningful diff (≥50 chars) | `SUCCESS_WITH_RECEIPT` |
| Exit 0 + no/empty diff | `NOOP_WITH_RECEIPT` |

### 3. New Receipt Fields (`internal/receipt/receipt.go`)

```go
type ForgeReceipt struct {
    // ... existing fields ...
    RunnerMode     string   `json:"runner_mode"`
    RunnerCommand  string   `json:"runner_command"`
    RunnerExitCode int      `json:"runner_exit_code"`
    WorktreePath   string   `json:"worktree_path"`
    BaseCommit     string   `json:"base_commit"`
    TestsPassed    int      `json:"tests_passed"`
    ScansPassed    bool     `json:"scans_passed"`
}
```

### 4. Config (`config/c1-forge.example.yaml`)

```yaml
forge:
  runner:
    mode: "mock"
    # command: "opencode"
    # args:
    #   - "run"
    #   - "--task-file"
    #   - "{{task_file}}"
    #   - "--workdir"
    #   - "{{worktree}}"
```

### 5. Fixture Repo (`/tmp/c1-forge-fixture`)

A disposable git repo with:
- `src/main.go` — simple Go program
- `src/main_test.go` — passing tests (`TestAdd`, `TestAddNegative`)
- Used by real-run integration tests

### 6. Real-Run Tests (`cmd/c1-forged/main_test.go`)

| Test | Description |
|---|---|
| `TestRealRunNoopTask` | Full daemon flow: claim, worktree, run, receipt |
| `TestRealRunDocsPatchTask` | Task with `touch README.md` command |
| `TestRealRunTestCommand` | Task running `go test ./...` on fixture |
| `TestVerdictMappingSuccess` | 7 sub-tests covering all verdict paths |

## Test Results

```
ok  github.com/anomalyco/c1-forge/cmd/c1-forged       0.939s
ok  github.com/anomalyco/c1-forge/internal/governor    1.420s
ok  github.com/anomalyco/c1-forge/internal/queue        2.262s
ok  github.com/anomalyco/c1-forge/internal/runner        3.202s
```

Total: 21+ tests passing (17 unit + 4 integration).

## Hard Constraints Verified

- [x] One disposable repo only
- [x] One task at a time
- [x] No VPS yet
- [x] No C1 Notes UI yet
- [x] No auto git push
- [x] No deploy
- [x] No npm publish
- [x] No .env reads
- [x] No external directory access
- [x] No production repos
- [x] Every task produces receipt

## Gaps

1. **Real runner command not tested end-to-end**: The "real" mode executes any configured command, but no external C1 Loop tool is installed yet on this machine. Integration tests run in "mock" mode.
2. **Pinocchio still a stub**: No change from Phase 0.
3. **ntfy still a stub**: No change from Phase 0.
4. **No multi-round C1 Loop**: The daemon runs one round per claim. Multi-round loop logic awaits a real runner command that supports it.
