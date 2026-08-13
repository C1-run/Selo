# C1 Forge Phase 0.2 — External Runner Smoke

## Overview

Phase 0.2 proves that `c1-forged` can run a real external command path safely against a disposable repo and produce honest receipts. All 6 E2E tests use `fixture_agent.sh` as the external runner with `runner.mode=real`.

## What Was Built

### 1. `scripts/fixture_agent.sh`

A shell script that simulates a C1 Loop/OpenCode external runner. It accepts `task_file` and `worktree` as arguments, reads the task's `goal` field, and produces the requested outcome:

| Mode | Behavior | Expected Verdict |
|---|---|---|
| `NOOP` | Do nothing, exit 0 | `NOOP_WITH_RECEIPT` |
| `DOCS_PATCH` | Create README.md, run `go test`, exit 0 | `SUCCESS_WITH_RECEIPT` |
| `FAILING_TEST` | Break `src/main_test.go`, run `go test`, exit 0 | `PARTIAL_FAILURE` |
| `FORBIDDEN_FILE` | Create `src/secret.rs`, exit 0 | `FAILED_SAFETY` |

### 2. E2E Tests (`cmd/c1-forged/main_test.go`)

6 new E2E tests that configure `runner.mode=real` with `runner.command = fixture_agent.sh`:

| Test | Verifies |
|---|---|
| `TestExternalRunnerNoop` | `NOOP_WITH_RECEIPT`, `runner_mode=real`, `base_commit` present, `scans_passed=true` |
| `TestExternalRunnerDocsPatch` | `SUCCESS_WITH_RECEIPT`, `files_changed>=1`, `patch_lines>=1` |
| `TestExternalRunnerFailingTest` | `PARTIAL_FAILURE` when test output contains `FAIL` |
| `TestExternalRunnerForbiddenFileAttempt` | `FAILED_SAFETY`, `scans_passed=false`, safety hits contain "forbidden" |
| `TestExternalRunnerReceiptFields` | All 13 required receipt fields present |
| `TestExternalRunnerNoopReceipt` | `NOOP_WITH_RECEIPT`, `runner_mode=real`, `base_commit`, `worktree_path` |

### 3. Verdict Mapping Update (`runner.MapVerdict`)

Added test output checking: if exit code is 0 but test output contains `FAIL` or `failed`, the verdict becomes `PARTIAL_FAILURE` (was previously incorrectly mapping to `SUCCESS`).

### 4. Diff Capture Fix (`CaptureDiff`)

Changed from `git diff` (unstaged only) to `git diff HEAD` (staged + unstaged) to catch changes that external runners `git add` before exiting.

## Receipt Fields Verified in E2E

All receipts from real-mode executions contain:

| Field | Present |
|---|---|
| `runner_mode` | `"real"` |
| `runner_command` | Full command with substituted args |
| `runner_exit_code` | 0 |
| `worktree_path` | Absolute path |
| `base_commit` | SHA hash |
| `files_changed` | Count |
| `patch_lines` | Count |
| `tests_passed` | Count (when commands run) |
| `scans_passed` | `true` or `false` |
| `verdict` | Correct per scenario |

## E2E Flow

```
task.md (with goal marker)
  → c1-forged claims task (atomic rename to running/)
  → git worktree created from fixture repo
  → runs fixture_agent.sh {{task_file}} {{worktree}}
    → agent reads goal, performs action, modifies files, git adds
    → agent runs go test (if commands in task)
    → agent exits 0
  → daemon captures diff (git diff HEAD), test output, safety scans
  → MapVerdict determines final verdict
  → receipt.json + receipt.md written
  → task moved to done/failed/review
  → worktree cleaned up
```

## Test Results

```
ok  github.com/anomalyco/c1-forge/cmd/c1-forged      23.6s    (12 tests)
ok  github.com/anomalyco/c1-forge/internal/governor    9.5s    (5 tests)
ok  github.com/anomalyco/c1-forge/internal/queue        5.6s    (3 tests)
ok  github.com/anomalyco/c1-forge/internal/runner       11.5s    (9 tests)
```

**Total: 29 tests** (17 unit + 6 Phase 0.1 integration + 6 new E2E).

## Verdicts Verified

| Scenario | Expected | Got |
|---|---|---|
| No-op external runner | `NOOP_WITH_RECEIPT` | ✓ |
| Docs patch + tests pass | `SUCCESS_WITH_RECEIPT` | ✓ |
| Tests fail with diff | `PARTIAL_FAILURE` | ✓ |
| Forbidden file edit | `FAILED_SAFETY` | ✓ |

## Hard Constraints

- [x] Disposable fixture repo only
- [x] One task at a time
- [x] No VPS
- [x] No C1 Notes UI
- [x] No auto git push
- [x] No deploy
- [x] No npm publish
- [x] No .env reads
- [x] No external directory access
- [x] No production repos
- [x] Every task produces receipt (including failures)
