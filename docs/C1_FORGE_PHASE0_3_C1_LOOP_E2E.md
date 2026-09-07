# C1 Forge Phase 0.3 — C1 Loop Binary E2E

## Objective
Verify that `c1-forged` can discover, smoke-test, and run an external C1 Loop binary end-to-end, producing receipts with `c1_loop_receipt_path`.

## Deliverables

### 1. `scripts/c1-loop.sh` — C1 Loop Binary Shim
- Minimal shell script implementing the C1 Loop interface: `c1 loop --task-file <path> --workdir <path>`
- Goal-aware behaviors:
  - `NOOP` — no changes, exits 0
  - `DOCS_PATCH` — creates `README.md`, `git add`s it
  - `FAILING_TEST` — breaks `src/main_test.go`, `git add -A`
  - `FORBIDDEN_FILE` — creates `src/secret.rs`, `git add`s it
- Writes `c1-receipt.json` in the worktree with status, goal, timestamps
- Task commands (e.g., `go test ./...`) executed after goal action

### 2. Binary Discovery (`runner.DiscoverC1Binary`)
- Accepts explicit path; if empty, searches `PATH` for `c1`, `opencode`, `c1-loop.sh`
- Returns absolute path to executable or empty string
- `"scripts/c1-loop.sh"` is in the C1-forge base dir so the explicit path resolves

### 3. Smoke Command (`c1-forged smoke c1-loop`)
- Verifies: binary discovery, disposable repo, config wiring, receipt writer
- Exits 0 on all checks passing

### 4. New Receipt Fields
- `c1_loop_receipt_path` — absolute path to `c1-receipt.json` in worktree (if found)
- `forge_receipt_path` — reserved for future use

### 5. E2E Tests (5 tests, all passing)
| Test | Verdict | Notes |
|---|---|---|
| `TestC1LoopBinaryDiscovery` | PASS | Absolute path, nonexistent, empty |
| `TestC1LoopNoopTask` | PASS | Verdict `NOOP_WITH_RECEIPT` |
| `TestC1LoopDocsPatchTask` | PASS | Verdict `SUCCESS_WITH_RECEIPT`, diff captured |
| `TestC1LoopFailingTestTask` | PASS | Verdict `PARTIAL_FAILURE`, test output captured |
| `TestC1LoopForbiddenFileAttempt` | PASS | Verdict `FAILED_SAFETY`, `c1_loop_receipt_path` present |

## Files Changed
- `scripts/c1-loop.sh` — new file (C1 Loop shim)
- `internal/runner/runner.go` — added `DiscoverC1Binary`, `isExecutable`
- `internal/receipt/receipt.go` — added `C1LoopReceiptPath`, `ForgeReceiptPath` fields + markdown rendering
- `cmd/c1-forged/main.go` — added `c1_loop_receipt_path` population, `smoke` subcommand
- `cmd/c1-forged/main_test.go` — added 5 E2E tests + `c1LoopShimPath` helper + `cleanupStaleBranches`
- `config/c1-forge.example.yaml` — updated runner examples

## Test Count
- Phase 0-0.2: 29 tests
- Phase 0.3: 5 tests
- **Total: 34 tests, all passing**
