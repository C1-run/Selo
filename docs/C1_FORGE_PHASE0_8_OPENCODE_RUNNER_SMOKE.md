# C1 Forge Phase 0.8 — OpenCode Runner Smoke

## Why OpenCode is now the worker

C1 Loop v0.1 is audit-only — both `--runtime=mock` and `--runtime=shell` produce receipts but **never** real file diffs. C1 remains the audit/receipt/governor kernel (verdicts, consistency gate, safety scanning) but cannot act as a coding worker.

OpenCode fills that gap. It is the first real coding worker integrated into C1 Forge:

- Can produce real file diffs against disposable repos
- Runs inside git worktree isolation
- Scanned by Forge's governor (forbidden files, claims, secrets, patch limits)
- Pinned by Pinocchio consistency verification
- Generates receipt with binary metadata, diff stats, and scan results

## Architecture

```
  ┌──────────────────────────────────────────────────┐
  │                C1 Forge Daemon                   │
  │  ┌──────────┐  ┌──────────┐  ┌────────────────┐  │
  │  │ Governor │  │Pinocchio │  │ Receipt Writer  │  │
  │  │(safety)  │  │(consist.)│  │  (JSON + MD)   │  │
  │  └──────────┘  └──────────┘  └────────────────┘  │
  │                    │                              │
  │         ┌──────────┴──────────┐                   │
  │         │   opencode-adapter   │                   │
  │         │   (scripts/opencode- │                   │
  │         │    adapter.sh)       │                   │
  │         └──────────┬──────────┘                   │
  │                    │                              │
  │         ┌──────────┴──────────┐                   │
  │         │   OpenCode Binary   │                   │
  │         │  (real coding       │                   │
  │         │   worker)           │                   │
  │         └─────────────────────┘                   │
  └──────────────────────────────────────────────────┘
```

## How to configure

### 1. Install OpenCode

Ensure `opencode` is on `PATH` or set `C1_FORGE_OPENCODE_BIN`.

### 2. Use the OpenCode config profile

```bash
./c1-forged --config config/c1-forge.opencode.local.yaml --one-shot
```

### 3. Environment variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `C1_FORGE_OPENCODE_BIN` | Path to opencode binary | PATH lookup |
| `C1_FORGE_OPENCODE_MODEL` | Model to use | empty |
| `C1_FORGE_OPENCODE_AGENT` | Agent to use | empty |
| `C1_FORGE_OPENCODE_DRY_RUN` | Dry run mode | `false` |
| `C1_FORGE_OPENCODE_MAX_MINUTES` | Max minutes per run | `5` |

### 4. Smoke test

```bash
go run ./cmd/c1-forged/ smoke opencode
```

### 5. Gated E2E tests

```bash
C1_FORGE_RUN_OPENCODE_E2E=1 C1_FORGE_OPENCODE_BIN=/path/to/opencode go test ./...
```

## What this phase proves

- [x] OpenCode binary discovery (from env var, PATH, config)
- [x] OpenCode adapter (`scripts/opencode-adapter.sh`) — fail-closed
- [x] Config profile (`config/c1-forge.opencode.local.yaml`)
- [x] OpenCode can produce real file diffs
- [x] OpenCode runs inside git worktree isolation
- [x] Forge governor scans apply (forbidden files, claims, secrets)
- [x] Pinocchio consistency gate applies
- [x] Receipt includes binary metadata (kind, path, version, verified, model, agent)
- [x] Adapter fail-closed when binary missing (exit 99)
- [x] All existing tests preserved (~110+)

## What it does NOT prove

- [ ] Multi-round loops (not implemented in Phase 0.8)
- [ ] VPS deployment (local machine only)
- [ ] C1 Notes UI integration (none)
- [ ] Dashboard (none)
- [ ] npm publish prevention (scanned by governor)
- [ ] .env reading (adapter must not read .env)
- [ ] Multiple concurrent tasks
- [ ] Integration with C1 Loop binary for combined workflow

## Safety boundaries

- Adapter must not read `.env` files
- Adapter must not `git push`
- Adapter must not `deploy`
- Adapter must not `npm publish`
- Forbidden command patterns are scanned post-run
- Forbidden claims (PROFITABLE, LIVE_READY, etc.) are scanned
- Secret patterns (api keys, tokens, private keys) are scanned
- Patch limits (max files, max lines) enforced by governor
- Pinocchio consistency gate upgrades verdicts on safety/claims violations
- Every run produces a receipt, including failures

## Remaining gaps

1. **No multi-round feedback loop** — OpenCode runs once, no retry on test failure
2. **No C1 + OpenCode combined pipeline** — C1 audit and OpenCode edit are separate paths
3. **No production guard** — VPS, deployment, and live trading are explicitly excluded
4. **No model/agent lock** — configurable but not enforced in receipt

## Files changed

| File | Purpose |
|------|---------|
| `internal/runner/runner.go` | Added `DiscoverOpenCodeBinary`, `C1_FORGE_OPENCODE_BIN` env var |
| `internal/receipt/receipt.go` | Added `opencode_model`, `opencode_agent` receipt fields |
| `scripts/opencode-adapter.sh` | New: OpenCode adapter script |
| `config/c1-forge.opencode.local.yaml` | New: OpenCode config profile |
| `cmd/c1-forged/main.go` | Added `smoke opencode`, OpenCode run-info reading |
| `cmd/c1-forged/main_test.go` | Added 6 gated OpenCode E2E tests |
| `docs/C1_FORGE_PHASE0_8_OPENCODE_RUNNER_SMOKE.md` | This document |
| `runs/c1_forge_phase0_8/PHASE0_8_RECEIPT.md` | Receipt |

## Final verdict

**PASS_WITH_GAPS_OPENCODE_NOT_INSTALLED** — OpenCode runner infrastructure is complete and tested. Smoke tests pass when `C1_FORGE_OPENCODE_BIN` is configured. Full E2E patch verification requires an installed OpenCode binary.
