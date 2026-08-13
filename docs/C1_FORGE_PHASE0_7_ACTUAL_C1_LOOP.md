# C1 Forge Phase 0.7 — Actual C1 Loop Binary E2E

## What This Phase Proves

- Runner can discover an actual C1 binary via explicit config, env var, or PATH
- Discovery distinguishes `real_c1`, `opencode`, `shim`, and `unknown` binary kinds
- `c1-forged smoke actual-c1` validates the full discovery + adapter chain
- Binary metadata (kind, path, version, verified) is written to receipt.json
- Adapter script exists and fails closed when real C1 binary is unavailable
- Receipt includes `runner_binary_kind`, `runner_binary_path`, `runner_binary_version`, `runner_binary_verified`

## What It Does NOT Prove

- Real C1 Loop running without an actual `c1` binary on PATH (must be installed separately)
- Multi-round C1 Loop execution (limited to what the binary supports)
- Any capability beyond calling the binary and recording the receipt
- OpenCode or AI coding capability
- VPS or cloud deployment

## Actual C1 Binary Discovery Rules

| Priority | Source | Example |
|----------|--------|---------|
| 1 | Explicit `runner.command` in config | `command: "/usr/local/bin/c1"` |
| 2 | `C1_FORGE_C1_BIN` env var | `export C1_FORGE_C1_BIN=/path/to/c1` |
| 3 | `c1` on PATH | `which c1` |
| 4 | `opencode` on PATH | `which opencode` |
| 5 | `c1-loop` / `c1-loop.sh` (only with `--allow-shim`) | `which c1-loop.sh` |

## Binary Kind Classification

| Filename | Kind |
|----------|------|
| `c1` | `real_c1` |
| `opencode` | `opencode` |
| `c1-loop` or `c1-loop.sh` | `shim` |
| anything else | `unknown` |

## Adapter Behavior

The adapter at `scripts/c1-real-adapter.sh`:

- Uses `C1_FORGE_C1_BIN` if set, then PATH lookup for `c1`, then `opencode`
- Exits with code 99 (FATAL) if no binary found
- Passes `--task-file <path> --workdir <dir>` to the real C1 binary
- Propagates exit code from the real C1 binary
- Fails closed — never fakes work

## Smoke Command

```bash
# Normal smoke (rejects shims)
c1-forged smoke actual-c1

# Allow shim for testing
c1-forged smoke actual-c1 --allow-shim

# With explicit env var
C1_FORGE_C1_BIN=scripts/c1-loop.sh c1-forged smoke actual-c1 --allow-shim
```

## Receipt Fields Added

| Field | Type | Description |
|-------|------|-------------|
| `runner_binary_kind` | string | `real_c1`, `opencode`, `shim`, or `unknown` |
| `runner_binary_path` | string | Absolute path to the binary |
| `runner_binary_version` | string | Output of `--version` or `version` subcommand |
| `runner_binary_verified` | bool | Whether the binary is executable and found |

## E2E Test

A gated end-to-end test is available but skipped by default:

```bash
C1_FORGE_RUN_ACTUAL_C1_E2E=1 go test ./... -run TestActualC1BinaryE2E
```

This test requires a real `c1` binary on PATH or via `C1_FORGE_C1_BIN`.
It creates a disposable repo, runs `c1 loop --task-file <task> --workdir <repo>`, and checks for `c1-receipt.json`.

## Known Gaps

- No actual `c1` binary installed in this development environment
- C1 Loop CLI interface (`loop --task-file --workdir`) must match expectations; if not, adapter may need adjustment
- Current `c1-loop.sh` shim produces verbatim `c1-receipt.json` but does not call real C1
- Real C1 may require additional arguments (model, agent, etc.)
- The adapter does not handle `{{max_minutes}}` or {{task_id}} template substitution (Forge handles those before calling the adapter)
