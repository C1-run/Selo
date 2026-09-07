# C1 Forge Phase 0 Specification

## Overview

C1 Forge is a local file-queue daemon (`c1-forged`) that processes bounded coding tasks through a C1 Loop and writes receipts. It operates on a single repository, one task at a time, on a local Mac.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  c1-forged                        │
│  ┌──────────┐  ┌───────────┐  ┌───────────────┐ │
│  │ Queue     │  │ Governor  │  │ Runner         │ │
│  │ Manager   │  │           │  │ (C1 Loop)      │ │
│  └──────────┘  └───────────┘  └───────────────┘ │
│  ┌──────────┐  ┌───────────┐  ┌───────────────┐ │
│  │ Workspace │  │ Receipt   │  │ Hooks          │ │
│  │ Manager   │  │ Writer    │  │ (Secret, etc)  │ │
│  └──────────┘  └───────────┘  └───────────────┘ │
└─────────────────────────────────────────────────┘
```

## Directory Layout

```
~/C1-forge/
├── cmd/c1-forged/main.go        # Daemon entry point
├── internal/
│   ├── queue/queue.go           # File queue with atomic rename
│   ├── workspace/workspace.go   # Git worktree manager
│   ├── runner/runner.go         # C1 Loop subprocess runner
│   ├── governor/governor.go     # Timeout/constraint enforcement
│   └── receipt/receipt.go       # Receipt + review writer
├── config/c1-forge.example.yaml # Example configuration
├── queue/
│   ├── pending/                 # Incoming tasks (task.md)
│   ├── running/                 # Currently claimed (with lock)
│   ├── review/                  # Needs human review
│   ├── done/                    # Completed successfully
│   └── failed/                  # Failed tasks
├── runs/                        # Per-task run artifacts
├── receipts/                    # All receipts (json + md)
├── worktrees/                   # Git worktrees
├── docs/C1_FORGE_PHASE0_SPEC.md # This file
├── go.mod
└── .c1-forge.lock               # Daemon lock
```

## Task Format (task.md)

```yaml
id: "task-001"
title: "Example Task"
repo: "/path/to/repository"
goal: "Implement feature X"
acceptance_criteria:
  - "Criterion 1"
  - "Criterion 2"
allowed_files:
  - "src/"
forbidden_files:
  - "src/secret.rs"
commands:
  - "cargo test"
  - "npm run lint"
max_minutes: 30
max_rounds: 3
max_files: 10
max_patch_lines: 200
forbidden_claims:
  - "PROFITABLE"
  - "LIVE_READY"
deliverables:
  - "receipt.md"
```

## Queue Flow

```
pending ──[atomic rename]──> running ──> review
                                 ├──> done
                                 └──> failed
```

## Lock Mechanism

- Daemon lock: `.c1-forge.lock` (PID + timestamp)
- Task lock: `queue/running/<task-id>.lock` (PID + timestamp + task ID)
- Stale lock detection: Check if PID is still alive via signal(0)

## Governor

- `max_minutes`: Hard timeout per task (default: 30)
- `max_rounds`: Max C1 Loop iterations (default: 3)
- `max_files`: Max files modified (default: 10)
- `max_patch_lines`: Max diff lines (default: 200)
- Stop file: Touch `.c1-forge-stop` for graceful shutdown

## Hook Chain

1. **Secret scan** - Regex patterns for API keys, private keys, tokens
2. **Forbidden claims scan** - Substring match against forbidden terms
3. **Forbidden file edit check** - Verify only allowed files modified
4. **Patch limit check** - Enforce max files and patch lines

## Verdicts

| Verdict | Meaning |
|---|---|
| `SUCCESS_WITH_RECEIPT` | Task completed successfully |
| `PARTIAL_FAILURE` | Non-zero exit code |
| `NEEDS_HUMAN` | Safety hits found (non-critical) |
| `FAILED_TIMEOUT` | max_minutes exceeded |
| `FAILED_SAFETY` | Forbidden file or secret detected |
| `FAILED_LIMIT_EXCEEDED` | Patch limit exceeded |
| `FAILED_STALE_LOCK` | Previous lock detected and cleaned |
| `FAILED_INTERNAL_ERROR` | Infrastructure failure |

## Receipt Format

Each receipt has JSON and Markdown variants:
- `receipts/<receipt-id>.json` - Machine-readable
- `receipts/<receipt-id>.md` - Human-readable
- `runs/run-<task-id>/receipt.*` - Per-task copy
- `runs/run-<task-id>/diff.patch` - Git diff
- `runs/run-<task-id>/test_output.txt` - Test output

## Acceptance Tests

1. Daemon claims a pending task ✓
2. Stale lock is detected ✓
3. Timeout produces FAILED_TIMEOUT receipt ✓
4. Forbidden file edit produces FAILED_SAFETY ✓
5. Failed tests produce PARTIAL_FAILURE or NEEDS_HUMAN ✓
6. Successful no-op demo task produces SUCCESS_WITH_RECEIPT ✓
7. No auto push/deploy command can run ✓

## Hard Constraints

- One repo only
- One task at a time
- Local Mac first
- No VPS yet
- No containers yet
- No web UI
- No auto git push
- No deploy
- No npm publish
- No .env reads
- No external directory access
- No live trading / wallet / exchange code
- Every task must produce a receipt, including failures
