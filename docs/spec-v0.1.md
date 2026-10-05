# Selo Specification v0.1

**Status**: Draft (v0.1 design)  
**Version**: 0.1.0  
**Update (2026-10)**: v0.2 and v0.3 added `selo verify`, `selo receipt` and `selo keys` (not in this draft); receipt paths are `receipts/` and `runs/run-<taskID>/` (not `.selo/receipts/`); only the git-worktree containment is implemented. See the README "What works" table for the current capability list.  
**Date**: 2026-09-03  
**License**: Apache-2.0  

---

## 1. Overview

Selo is a safety-first task runner for AI coding agents. It wraps any coding agent (OpenCode, Claude, GPT, etc.) with a multi-layer verification pipeline and produces tamper-proof cryptographic receipts for every task.

### 1.1 Design Principles

1. **Fail-Closed**: Unknown capabilities deny. Nil summaries produce stop. Empty lists pass.
2. **Immutable Audit Trail**: Every action produces a hash-chained event. Receipts are tamper-proof.
3. **Capability-Based Security**: Agents receive only explicitly granted permissions via frozen manifests.
4. **Composable Safety**: Six independent safety checks run in sequence. Any check can override the verdict.
5. **Deterministic Verification**: Pinocchio consistency checks are pure functions of the receipt data.

### 1.2 Architecture Summary

```
┌─────────────────────────────────────────────────────────────┐
│                      Selo Daemon                         │
├─────────────────────────────────────────────────────────────┤
│  Queue Manager          │  Safety Pipeline                   │
│  ┌─────────────┐        │  ┌─────────────────────────────┐  │
│  │ pending/    │───────▶│  │ 1. Governor (policy)        │  │
│  │ running/    │◀───────│  │ 2. Scans (secrets, claims)  │  │
│  │ done/       │        │  │ 3. Pinocchio (consistency)  │  │
│  │ failed/     │        │  │ 4. GateChain (compliance)   │  │
│  │ review/     │        │  │ 5. Receipt (cryptographic)  │  │
│  └─────────────┘        │  └─────────────────────────────┘  │
├─────────────────────────┴───────────────────────────────────┤
│  Containment Layer                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ Worktree │  │  Docker  │  │  Local   │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
├─────────────────────────────────────────────────────────────┤
│  Broker (Capability System)                                 │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ deny-by-default · manifest-bound · glob-matching    │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. CLI Specification

### 2.1 Binary

- **Name**: `selo`
- **Module**: `github.com/C1-run/selo`
- **Go Version**: 1.21+

### 2.2 Commands

#### `selo run [goal]`

Execute a single task through the safety pipeline. Always produces a receipt.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | cwd | Git repository path |
| `--commands` | string[] | [] | Test commands to run after agent |
| `--max-minutes` | int | 30 | Maximum minutes for agent execution |
| `--forbidden-files` | string[] | [] | Files agent must not modify |

**Args**: Exactly 1 (the task goal)

**Exit Codes**:
| Code | Meaning |
|------|---------|
| 0 | Success or Noop |
| 1 | Safety failure, limit exceeded, or internal error |
| 2 | Timeout |
| 3 | Needs human review |

**Output**: Receipt written to `receipts/c1f-{timestamp}.json` and `runs/run-{taskID}/receipt.json`

---

#### `selo daemon`

Start the Selo daemon to process queued tasks.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--one-shot` | bool | false | Process one task and exit |

**Behavior**:
- Polls `queue/pending/` every `poll_interval_sec` seconds
- Atomically claims tasks via rename to `queue/running/`
- Processes through safety pipeline
- Routes to `done/`, `failed/`, or `review/` based on verdict
- Creates lock file `.selo.lock` with PID
- Checks for stop file `.selo-stop` each iteration

---

#### `selo check`

Run GateChain compliance check on a steps file.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--steps-file` | string | (required) | Path to steps.json |
| `--format` | string | "json" | Output format: `json` or `text` |

**Input**: JSON array of `ChainStep` objects

**Output**: JSON object with `action` (pass/review/stop), `blocking_tools`, `warnings`

---

#### `selo status`

Show daemon/queue status and recent receipts.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dir` | string | auto | Base directory |
| `--json` | bool | false | Output as JSON |

---

#### `selo init`

Initialize a Selo workspace.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | cwd | Repository path |

**Creates**:
- `.selo/queue/{pending,running,done,failed,review}`
- `.selo/runs/`
- `.selo/receipts/`
- `.selo/worktrees/`
- `config/selo.yaml`

---

#### `selo smoke [command]`

Run smoke tests against configured runner.

**Subcommands**: `c1-loop`, `actual-c1`, `c1-runtimes`, `opencode`

---

#### `selo soak`

Run soak test to stress the pipeline.

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--duration` | string | "24h" | Soak duration |
| `--task-count` | int | 0 | Max tasks (0=unlimited) |
| `--interval` | string | "1m" | Poll interval |
| `--fixture-mode` | bool | true | Use generated fixture tasks |
| `--stop-on-safety` | bool | true | Stop on safety failure |
| `--out` | string | "runs/selo_phase0_6" | Output directory |
| `--runner` | string | "" | Runner type: `opencode` |

---

#### `selo completion [shell]`

Generate shell completion scripts. Supported: `bash`, `zsh`, `fish`, `powershell`

---

## 3. Configuration Specification

### 3.1 Config File Location

Default: `config/selo.yaml`

Override: `--config` flag or `SELO_CONFIG` environment variable

### 3.2 Full Schema

```yaml
forge:
  # Daemon behavior
  poll_interval_sec: 5              # Queue polling interval (seconds)
  default_max_minutes: 30           # Default task timeout
  default_max_rounds: 3             # Default max agent rounds
  default_max_files: 10             # Default max files changed
  default_max_patch_lines: 200      # Default max patch lines
  
  # Signal files
  stop_file: ".selo-stop"       # Daemon stop signal
  lock_file: ".selo.lock"       # Daemon lock file
  
  # Notifications
  notify: "stdout"                  # Legacy: "stdout" | "ntfy"
  ntfy_topic: ""                    # Legacy: ntfy topic
  notify_config:
    mode: "stdout"                  # "stdout" | "ntfy" | "disabled"
    ntfy_url: ""                    # ntfy topic URL
    timeout_seconds: 5              # HTTP timeout
  
  # External scan tools
  scan_tools:
    secret_scan: ""                 # Path to secret scan binary
    forbidden_claims_scan: ""       # Path to claims scan binary
    pinocchio: ""                   # Path to pinocchio binary
  
  # Forbidden terms
  forbidden_claims:
    - "PROFITABLE"
    - "LIVE_READY"
    - "MONEY_ENGINE"
    - "CAPITAL_APPROVED"
    - "LIVE_CAPITAL_APPROVED"
    - "MONEY_MACHINE"
  
  # Runner configuration
  runner:
    mode: "real"                    # "real" | "mock"
    command: "c1-loop"              # Command to execute
    args: []                        # Arguments (supports {{var}} templating)
  
  # OpenCode adapter
  opencode:
    model: "anthropic/claude-sonnet-4-5"
    agent: "default"
    serve_timeout: 30               # Seconds
    dangerously_skip_permissions: false
    permission_allowlist: []        # Glob patterns for allowed files
```

### 3.3 Environment Variables

| Variable | Purpose |
|----------|---------|
| `SELO_OPENCODE_BIN` | Path to OpenCode binary |
| `SELO_OPENCODE_MODEL` | OpenCode model override |
| `SELO_C1_BIN` | Path to C1 binary |
| `SELO_DANGEROUSLY_SKIP_PERMISSIONS` | Skip permission checks (must be "1" or "true") |
| `SELO_C1_RUNTIME` | Requested runtime (default: "mock") |
| `SELO_SERVE_TIMEOUT` | Serve timeout override |
| `SELO_RUN_TIMEOUT_SEC` | Run timeout override |
| `SELO_TEST_INITIAL_VERDICT` | Force initial verdict for testing |

---

## 4. Safety Pipeline Specification

### 4.1 Pipeline Stages

Every task passes through these stages in order:

```
Task Input
    │
    ▼
┌─────────────────┐
│ 1. Governor     │  Policy enforcement, timeout, limits
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. Scans        │  6 safety checks in sequence
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. Pinocchio    │  Consistency verification
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. GateChain    │  Compliance decision (SEMANTIC_SPEC)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. Receipt      │  Cryptographic receipt generation
└────────┬────────┘
         │
         ▼
    Task Output
```

### 4.2 Check Interface

```go
type Check interface {
    Name() string
    Run(ctx CheckContext) CheckResult
}

type CheckResult struct {
    Hits      []string  // Human-readable hit messages
    Violation bool      // Whether this check failed
}

type CheckContext struct {
    Diff            string   // git diff HEAD output
    TestOutput      string   // Test command output
    WorktreePath    string   // Path to git worktree
    Commands        []string // Test commands
    AllowedFiles    []string // Glob patterns for allowed files
    ForbiddenFiles  []string // Glob patterns for forbidden files
    ForbiddenClaims []string // Prohibited terms
    Governor        *governor.Governor
    AllowTestMods   bool     // Allow test file modifications
    TestInvBefore   *testintegrity.TestInventory
    RunnerMode      string   // "real" | "mock"
    Verdict         string   // Current verdict
}
```

### 4.3 Default Checks

| Check | Purpose | Failure Action |
|-------|---------|----------------|
| `forbidden_file_edit` | Detects disallowed file modifications | Sets NEEDS_HUMAN |
| `forbidden_claims` | Scans for prohibited terms | Sets FAILED_SAFETY |
| `secret_scan` | Detects private keys, API keys, tokens | Sets FAILED_SAFETY |
| `patch_limit` | Verifies file/line count limits | Sets FAILED_LIMIT_EXCEEDED |
| `round_limit` | Verifies round count limits | Sets FAILED_LIMIT_EXCEEDED |
| `test_integrity` | Detects test file tampering | Sets NEEDS_HUMAN |

### 4.4 Verdict Constants

```go
const (
    VerdictSuccess        = "SUCCESS_WITH_RECEIPT"
    VerdictNoop           = "NOOP_WITH_RECEIPT"
    VerdictPartial        = "PARTIAL_FAILURE"
    VerdictNeedsHuman     = "NEEDS_HUMAN"
    VerdictTimedOut       = "FAILED_TIMEOUT"
    VerdictSafety         = "FAILED_SAFETY"
    VerdictLimitExceeded  = "FAILED_LIMIT_EXCEEDED"
    VerdictStaleLock      = "FAILED_STALE_LOCK"
    VerdictInternalError  = "FAILED_INTERNAL_ERROR"
)
```

### 4.5 Verdict Mapping Rules

```
IF timeout                          -> FAILED_TIMEOUT
IF safety_hits AND limit_violation  -> FAILED_LIMIT_EXCEEDED
IF safety_hits                      -> FAILED_SAFETY
IF exit_code != 0 OR tests_fail    -> PARTIAL_FAILURE
IF no_diff                          -> NOOP_WITH_RECEIPT
IF diff                             -> SUCCESS_WITH_RECEIPT
```

---

## 5. GateChain Specification

### 5.1 SEMANTIC_SPEC Compliance

GateChain implements a 3-rule decision priority:

```
Rule 1: IF stop_required=true  -> ActionStop
Rule 2: IF review_required=true AND stop=false -> ActionReview
Rule 3: OTHERWISE -> ActionPass
```

### 5.2 ChainStep Schema

```json
{
    "id": "string",
    "label": "string",
    "status": "pass|warn|block|skip",
    "coverage": "covered|not_checked|partial|blocked",
    "risk": "low|medium|high|blocked",
    "reason_code": "string",
    "evidence_ref": "string",
    "raw_text_included": false,
    "secrets_included": false,
    "adapter": "string"
}
```

### 5.3 Forbidden Fields

The following fields are forbidden in ChainStep objects:
- `raw_text` (use `evidence_ref` instead)
- `secrets` (never include secrets)
- `pnl`, `sharpe`, `drawdown` (financial data)
- `portfolio_value`, `position_size` (trading data)

### 5.4 AggregateVerdict Schema

```json
{
    "final_status": "pass|review|stop",
    "stop_required": false,
    "review_required": false,
    "blocking_tools": [],
    "warnings": [],
    "action": "pass|review|stop",
    "action_reason": "string"
}
```

---

## 6. Containment Specification

### 6.1 Interface

```go
type Containment interface {
    Setup(repoPath, taskID string) (string, error)
    Run(cmd []string, env []string, timeout time.Duration) (*Result, error)
    Teardown(repoPath, taskID string) error
    Workdir() string
}
```

### 6.2 Strategies

#### Worktree (Default)

- Creates git worktree at `worktrees/wt-{taskID}`
- Branch: `selo/{taskID}`
- Sanitized environment (PATH, HOME, TMPDIR only)
- Teardown: `git worktree remove --force` + `os.RemoveAll`

#### Docker

```bash
docker run --rm \
    --network none \
    --security-opt no-new-privileges \
    --cap-drop ALL \
    --read-only \
    -v /work:ro \
    {image} {command}
```

- No docker.sock access
- Read-only filesystem
- Network disabled
- Capabilities dropped

#### Local

- Process-level isolation in sandbox directory
- Sanitized environment
- No container overhead

### 6.3 Contracts

#### RunContract

```json
{
    "goal": "string",
    "deadline_utc": "ISO8601",
    "max_patch_count": 10,
    "patch_count": 0,
    "patches_after_deadline": []
}
```

#### ScopeContract

```json
{
    "allowed_paths": ["src/**"],
    "forbidden_paths": ["secrets/**"],
    "changed_paths": ["src/main.go"]
}
```

**Pattern Matching**: Uses `filepath.Match` (standard Go glob)

---

## 7. Capability Broker Specification

### 7.1 Design

- **Default**: Deny all requests
- **Grant**: Only via frozen manifest
- **Binding**: Manifest SHA-256 hash binds permissions

### 7.2 Request Kinds

| Kind | Target Pattern | Manifest Field |
|------|----------------|----------------|
| `filesystem.read` | File path | `capability.filesystem.read[]` |
| `filesystem.write` | File path | `capability.filesystem.write[]` |
| `exec` | Command | `capability.exec.allow[]` |
| `git.commit` | boolean | `capability.git.commit` |
| `git.push` | boolean | `capability.git.push` |

### 7.3 Pattern Matching Rules

- Standard Go glob via `path.Match`
- Directory prefix: `"src/"` covers `"src/a/b.ts"`
- Recursive: `"src/**"` covers all files under `src/`
- Exact match: `"README.md"` matches only that file

### 7.4 Unix Socket Protocol

```
Request:  {"run_id":"...","kind":"...","target":"...","args":{...}}
Response: {"allowed":true,"reason":"...","policy":"..."}
```

---

## 8. Manifest Specification

### 8.1 RunManifest Schema

```json
{
    "version": "1.0",
    "run_id": "string",
    "task": {
        "goal": "string",
        "repo": "string",
        "base_commit": "string"
    },
    "repo": {
        "path": "string",
        "base_commit": "string"
    },
    "agent": {
        "command": "string",
        "args": [],
        "image": "string"
    },
    "capability": {
        "filesystem": {
            "read": ["src/**"],
            "write": ["src/**", "tests/**"]
        },
        "exec": {
            "allow": ["go test ./..."]
        },
        "git": {
            "commit": true,
            "push": false
        }
    },
    "network": {
        "mode": "none",
        "model": ""
    },
    "verify": {
        "test_command": "go test ./..."
    },
    "limits": {
        "max_minutes": 30,
        "max_lines": 200,
        "max_files": 10,
        "work_mem": "512MB"
    }
}
```

### 8.2 Freeze Operation

```go
func Freeze(m *RunManifest) *Frozen {
    // 1. Canonicalize JSON (sorted keys, no whitespace)
    // 2. Compute SHA-256 hash
    // 3. Return Frozen{Manifest: m, Hash: hash}
}
```

**Invariant**: `HashOf(Freeze(m).Manifest) == Freeze(m).Hash`

---

## 9. Receipt Specification

### 9.1 ForgeReceipt Schema (60+ fields)

```json
{
    "receipt_id": "c1f-{unix_nano}",
    "task_id": "run-{uuid}",
    "goal": "string",
    "repo": "string",
    "base_commit": "string",
    "verdict": "SUCCESS_WITH_RECEIPT",
    
    "timing": {
        "started_at": "ISO8601",
        "completed_at": "ISO8601",
        "duration_ms": 1234
    },
    
    "agent": {
        "mode": "real",
        "command": "c1-loop",
        "exit_code": 0,
        "timed_out": false
    },
    
    "diff": {
        "files_changed": 3,
        "insertions": 50,
        "deletions": 10,
        "patch_lines": 60
    },
    
    "tests": {
        "passed": true,
        "output": "string"
    },
    
    "safety": {
        "hits": [],
        "secret_scan": "PASS",
        "forbidden_claims": "PASS",
        "forbidden_files": "PASS",
        "patch_limit": "PASS",
        "round_limit": "PASS",
        "test_integrity": "PASS"
    },
    
    "governor": {
        "max_rounds": 3,
        "max_files": 10,
        "max_patch_lines": 200,
        "rounds_used": 1,
        "files_changed": 3,
        "patch_lines": 60
    },
    
    "pinocchio": {
        "verified": true,
        "false_claims": [],
        "inconsistencies": [],
        "recommended_verdict": "SUCCESS_WITH_RECEIPT"
    },
    
    "gatechain": {
        "action": "pass",
        "blocking_tools": [],
        "warnings": [],
        "steps": []
    },
    
    "containment": {
        "strategy": "worktree",
        "workdir": "worktrees/wt-{taskID}"
    },
    
    "notification": {
        "sent": true,
        "mode": "stdout",
        "error": ""
    }
}
```

### 9.2 Receipt File Locations

| Location | Format | Purpose |
|----------|--------|---------|
| `receipts/c1f-{timestamp}.json` | JSON | Permanent archive |
| `receipts/c1f-{timestamp}.md` | Markdown | Human-readable |
| `runs/run-{taskID}/receipt.json` | JSON | Task-specific |
| `runs/run-{taskID}/review.md` | Markdown | Human review |
| `runs/run-{taskID}/diff.patch` | Text | Git diff |
| `runs/run-{taskID}/test_output.txt` | Text | Test output |
| `runs/run-{taskID}/pinocchio.json` | JSON | Consistency check |
| `runs/run-{taskID}/test_integrity.json` | JSON | Test integrity |
| `runs/run-{taskID}/steps.json` | JSON | GateChain steps |

---

## 10. Audit Trail Specification

### 10.1 Event Schema

```json
{
    "seq": 1,
    "ts": "ISO8601",
    "type": "flow|decision|verdict",
    "run_id": "string",
    "kind": "string",
    "target": "string",
    "policy": "string",
    "detail": "string",
    "previous_hash": "string",
    "event_hash": "string"
}
```

### 10.2 Hash Chain

```
event_hash = SHA256(seq + ts + type + run_id + kind + target + policy + detail + previous_hash)
```

**Verification**: Replay all events, recompute hashes, verify chain integrity.

### 10.3 Event Types

| Type | Purpose |
|------|---------|
| `flow` | Pipeline stage transitions |
| `decision` | Capability broker decisions |
| `verdict` | Final task verdict |

---

## 11. Plugin Specification

### 11.1 Plugin Registration

```json
// opencode.json
{
    "plugin": ["./.opencode/plugin.ts"]
}
```

### 11.2 Plugin ID

```
selo
```

### 11.3 Tools

| Tool | Description |
|------|-------------|
| `selo_daemon_status` | Check daemon status, queue statistics |
| `selo_submit_task` | Submit task for safe execution |
| `selo_task_status` | Check task receipt and verdict |
| `selo_safety_scan` | View recent safety scan results |
| `selo_compliance_check` | Run GateChain compliance check |

### 11.4 Tool Schemas

#### `selo_submit_task`

```json
{
    "goal": "string (required)",
    "repo": "string (optional)",
    "commands": ["string"] (optional),
    "maxMinutes": "number (optional)",
    "forbiddenFiles": ["string"] (optional)
}
```

#### `selo_task_status`

```json
{
    "taskId": "string (required)"
}
```

#### `selo_compliance_check`

```json
{
    "stepsFile": "string (optional, default: samples/steps-pass.json)"
}
```

---

## 12. Queue Specification

### 12.1 Directory Structure

```
.selo/queue/
├── pending/          # New tasks waiting to be claimed
├── running/          # Currently processing tasks
├── done/             # Successfully completed tasks
├── failed/           # Tasks that failed safety checks
└── review/           # Tasks requiring human review
```

### 12.2 Task File Format

```yaml
# task.md
id: run-{uuid}
goal: "Add feature X"
repo: /path/to/repo
commands:
  - "go test ./..."
max_minutes: 30
forbidden_files:
  - "secrets/**"
```

### 12.3 Claim Protocol

1. Scan `pending/` for `task.md` files
2. Attempt atomic rename: `pending/{taskID} -> running/{taskID}`
3. If rename fails (concurrent claim), skip
4. Create lock file: `.selo/locks/{taskID}.lock`

### 12.4 Routing Rules

| Verdict | Target Directory |
|---------|------------------|
| SUCCESS_WITH_RECEIPT | `done/` |
| NOOP_WITH_RECEIPT | `done/` |
| NEEDS_HUMAN | `review/` |
| PARTIAL_FAILURE | `review/` |
| FAILED_SAFETY | `failed/` |
| FAILED_TIMEOUT | `failed/` |
| FAILED_LIMIT_EXCEEDED | `failed/` |
| FAILED_INTERNAL_ERROR | `failed/` |

---

## 13. Test Coverage

### 13.1 Test Files

| Package | Test File | Key Tests |
|---------|-----------|-----------|
| `cmd/selo` | `main_test.go` | Main package tests |
| `internal/pipeline` | `pipeline_test.go` | 8 tests: RunAll, AggregatesHits, ViolationSetsOverride, PatchLimit, RoundLimit, TestIntegrity, DefaultChecks, FormatHits |
| `internal/containment` | `containment_test.go` | Worktree, Local, Docker |
| `internal/containment` | `contract_test.go` | RunContract (5 cases), ScopeContract (5 cases) |
| `internal/gatechain` | `consumer_test.go` | ValidateStep, Summarize, SEMANTIC_SPEC R1/R2/R3, ConsumeDecision, VerifyDecision |
| `internal/gatechain` | `receipt_adapter_test.go` | Receipt adaptation |
| `internal/broker` | `broker_test.go` | DenyByDefault, FSWrite, Exec, Git, RunID, Socket |
| `internal/manifest` | `manifest_test.go` | Freeze, Load, Hash |
| `internal/receipt` | `receipt_test.go` | Generation, Parsing |
| `internal/governor` | `governor_test.go` | Limit enforcement |
| `internal/pinocchio` | `pinocchio_test.go` | Consistency verification |
| `internal/runner` | `runner_test.go` | Runner modes, binary discovery |
| `internal/opencode` | `adapter_test.go` | Adapter validation |
| `internal/queue` | `queue_test.go` | Queue operations |
| `internal/notify` | `notify_test.go` | Notification modes |
| `internal/testintegrity` | `testintegrity_test.go` | Inventory analysis |
| `internal/evidence` | `evidence_test.go` | Hash chain |
| `internal/auditlog` | `auditlog_test.go` | Audit events |
| `internal/soak` | `soak_test.go` | Soak harness |
| `internal/cell` | `cell_test.go` | Cell interface |
| `internal/p45` | `p45_test.go` | P45 contracts |

### 13.2 Run Tests

```bash
go test ./...
```

---

## 14. Security Considerations

### 14.1 Threat Model

| Threat | Mitigation |
|--------|------------|
| Agent modifies forbidden files | ForbiddenFileEditCheck + ScopeContract |
| Agent injects secrets | SecretScanCheck + Capability Broker |
| Agent makes false claims | ForbiddenClaimsCheck + Pinocchio |
| Agent exceeds resource limits | Governor + PatchLimitCheck + RoundLimitCheck |
| Agent tampers with tests | TestIntegrityCheck |
| Concurrent task claims | Atomic rename + lock files |
| Receipt tampering | SHA-256 hash chain |

### 14.2 Permission Model

- **Default**: All file access denied
- **Grant**: Only via frozen manifest with SHA-256 binding
- **Enforcement**: Capability Broker checks every filesystem/exec/git operation
- **Audit**: All decisions logged to evidence ledger

### 14.3 Containment Security

| Strategy | Isolation Level |
|----------|-----------------|
| Worktree | Git-level isolation, sanitized env |
| Docker | Container isolation, network=none, read-only, cap-drop ALL |
| Local | Process-level, sanitized env |

---

## 15. Dependencies

### 15.1 Direct Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing |

### 15.2 Indirect Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/spf13/cobra` | v1.10.2 | CLI framework |
| `github.com/spf13/pflag` | v1.0.9 | Flag parsing |
| `github.com/inconshreveable/mousetrap` | v1.1.0 | Windows terminal detection |

---

## 16. Glossary

| Term | Definition |
|------|------------|
| **ChainStep** | A single gate in the GateChain compliance pipeline |
| **Containment** | Isolated execution environment for agent tasks |
| **ForgeReceipt** | Cryptographic receipt with 60+ audit fields |
| **GateChain** | Compliance decision engine implementing SEMANTIC_SPEC |
| **Governor** | Policy enforcement engine (limits, timeouts) |
| **Pinocchio** | Consistency verification engine |
| **P45** | Contract system (goal, scope, run contracts) |
| **RunManifest** | Immutable, SHA-256-bound permission document |
| **SEMANTIC_SPEC** | GateChain decision rule specification |

---

## Appendix A: File Locations

| Path | Purpose |
|------|---------|
| `config/selo.yaml` | Configuration file |
| `.selo/queue/` | Task queue directories |
| `.selo/runs/` | Task run directories |
| `.selo/receipts/` | Permanent receipt archive |
| `.selo/worktrees/` | Git worktrees |
| `.selo/locks/` | Task lock files |
| `runs/run-{taskID}/` | Per-task artifacts |
| `receipts/c1f-{timestamp}.json` | Permanent receipts |

---

## Appendix B: Error Codes

| Code | Constant | Meaning |
|------|----------|---------|
| 0 | - | Success or Noop |
| 1 | - | Safety/limit/internal error |
| 2 | - | Timeout |
| 3 | - | Needs human review |

---

## Appendix C: Version History

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | 2026-09-03 | Initial specification |

---

*End of Specification v0.1*
