# C1 Forge Phase 0.7.1 — Actual C1 Binary Smoke

## What This Phase Proves

- A real local C1 Loop binary (`c1_bin`) was discovered, classified (`real_c1`), versioned, and executed through C1 Forge
- The adapter correctly translates Forge's interface to C1's actual CLI
- A no-op task ran successfully through the daemon with the real C1 binary
- Forge receipt correctly records `runner_binary_kind=real_c1`, version, path, and verified status
- C1 receipt (`c1-receipt.json`) was produced and linked by the Forge receipt
- Pinocchio verified the run (no false claims, no inconsistencies)
- All binary metadata fields are populated in the receipt

## Real C1 Binary Path

```
Binary: /Users/desmondkam/loop/c1_bin
Kind:    real_c1
Version: c1 version 0.1.0
```

## Adapter Behavior

The adapter at `scripts/c1-real-adapter.sh` translates:

```
Forge:  c1-real-adapter.sh --task-file <path> --workdir <dir>
C1:     c1 loop "<goal>" --runtime=mock --max-rounds=<n>
```

It also initializes `.c1/config.yaml` if missing, runs `c1 loop`, and copies the C1 receipt to the worktree root as `c1-receipt.json`.

## Smoke Command Output

```
c1-forged smoke actual-c1
  C1 binary discovery → Found: /Users/desmondkam/loop/c1_bin
  Kind: real_c1
  Version: c1 version 0.1.0
  Verified: true
  Result: 5 passed, 0 failed
```

## Forge Daemon Run (Actual C1 No-Op Task)

```
C1_FORGE_C1_BIN=/Users/desmondkam/loop/c1_bin ./c1-forged --config config/c1-forge.actual-c1.local.yaml --one-shot

Result: task c1-actual-final → NOOP_WITH_RECEIPT
```

### Receipt Key Fields

| Field | Value |
|-------|-------|
| `runner_mode` | real |
| `runner_binary_kind` | real_c1 |
| `runner_binary_path` | /Users/desmondkam/loop/c1_bin |
| `runner_binary_version` | c1 version 0.1.0 |
| `runner_binary_verified` | true |
| `c1_loop_receipt_path` | worktrees/wt-c1-actual-final/c1-receipt.json |
| `pinocchio_verified` | true |
| `verdict` | NOOP_WITH_RECEIPT |

### Receipt Linkage

C1 receipt exists at the path referenced by `c1_loop_receipt_path` and contains:
```json
{"run_id": "...", "verdict": "CONDITIONAL", "task": "C1 Actual Binary Final Test", ...}
```

## What Passed

- Actual C1 binary discovery and classification
- Adapter translation of Forge → C1 interface
- C1 init + loop execution in disposable repo
- C1 receipt copy to worktree root
- Binary metadata written to Forge receipt (`kind=real_c1`, version, path, verified)
- Pinocchio verification passed
- Gated E2E test passes with real C1 binary
- Smoke command returns all 5/5 passed
- Short soak still passes

## What Failed / Gaps

- C1 `loop` currently uses `--runtime=mock` (no real agent execution)
- C1 verdict is `CONDITIONAL` (no verification configured, not real work)
- Docs/audit/feature tasks not tested (C1 mock runtime only observes, does not modify)
- No multi-round loop tested
- C1 binary is a prebuilt x86_64 binary (version 0.1.0) with mock-only runtime
- Binary is `c1_bin`, not `c1` on PATH (discovered via `C1_FORGE_C1_BIN` env var)
- No automated 24h soak was run with real C1 (not needed for this phase)

## Remaining Gaps After This Phase

- Real C1 verification mode must be configured for proper verdicts
- C1 0.1.0 does not produce meaningful diffs (mock runtime only)
- Forge Pinocchio correctly handles `NOOP` but cannot validate real C1 output
- Real C1 Loop with actual agent integration not yet tested
- Adapter currently hardcodes `--runtime=mock` — configurable if needed
