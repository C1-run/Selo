# C1 Forge Phase 0.4 — Pinocchio Receipt Consistency Gate

## Objective
Replace the Pinocchio stub with a deterministic receipt consistency verifier that prevents C1 Forge from reporting `SUCCESS_WITH_RECEIPT` unless all evidence is mutually consistent.

This phase is **not** about making the agent smarter. It is about making Forge unable to lie to itself.

## What This Phase Proves

- `internal/pinocchio/` is a pure, deterministic verifier — no external LLM, no API calls, no model
- Every receipt field is checked against every other piece of evidence (diff, test output, safety hits, C1 receipt)
- `MapVerdict()` output is verified, and if contradictions exist, a stricter verdict overrides it
- `pinocchio.json` artifact is written for every task with the full analysis
- Forge receipts include `initial_verdict`, `final_verdict`, `verdict_overridden`, `pinocchio_verified`, `pinocchio_false_claims`, and `pinocchio_inconsistencies`
- Lying runner (c1-loop.sh LYING mode) is correctly caught — false SUCCESS never reaches the receipt
- All 55+ tests pass (existing 34 preserved + 12 Pinocchio unit + 6 E2E daemon + 3 new)

## What This Phase Does Not Prove

- Does NOT replace a real LLM-based C1 Loop agent
- Does NOT add multi-round agent feedback
- Does NOT detect semantic correctness of code changes (only structural consistency)
- Does NOT add OpenCode, VPS, web UI, or deployment
- Does NOT make the runner smarter — only stricter about its own reporting

## Verdict Enum Behavior

The verifier receives the output of `runner.MapVerdict()` as `initial_verdict`, then applies consistency rules:

### SUCCESS_WITH_RECEIPT checks
- `runner_exit_code == 0`
- `scans_passed == true`
- No safety hits / forbidden file edits / secret scan hits / forbidden claims
- `testOutput` does NOT contain "FAIL" if test commands exist
- `C1LoopReceiptPath` exists and is parseable if runner is `c1-compatible`
- C1 receipt exit code is compatible (0 = completed)
- `worktree_path`, `runner_command`, `runner_mode` are all present
- Diff evidence is compatible with `files_changed` / `patch_lines`

### NOOP_WITH_RECEIPT checks
- `runner_exit_code == 0`
- `files_changed == 0`, `patch_lines == 0`
- `scans_passed == true`
- No safety hits or forbidden claims

### PARTIAL_FAILURE / NEEDS_HUMAN checks
- Evidence of failure exists (FAIL in test output, non-zero exit, meaningful diff)
- If no evidence exists, inconsistency is flagged

### FAILED_SAFETY checks
- Safety hits or safety findings exist
- If verdict is safety but no hits found, inconsistency is flagged

### FAILED_TIMEOUT checks
- Runner timed out

## Override Rules

After `runner.MapVerdict()` returns a verdict, Pinocchio runs its checks. If Pinocchio's `final_verdict` has a higher severity than the initial verdict, the override is applied:

| Initial Verdict | Detected Issue | Final Verdict |
|---|---|---|
| SUCCESS_WITH_RECEIPT | tests_passed=false | NEEDS_HUMAN |
| SUCCESS_WITH_RECEIPT | secret scan failed | FAILED_SAFETY |
| NOOP_WITH_RECEIPT | files_changed>0 | NEEDS_HUMAN |
| SUCCESS_WITH_RECEIPT | C1 receipt missing | NEEDS_HUMAN |
| SUCCESS_WITH_RECEIPT | C1 receipt says fail | NEEDS_HUMAN |
| SUCCESS_WITH_RECEIPT | forbidden claim | FAILED_SAFETY |
| PARTIAL_FAILURE | no evidence of failure | NEEDS_HUMAN |
| FAILED_SAFETY | no safety hits | NEEDS_HUMAN |

Severity order (ascending): `NOOP_WITH_RECEIPT` < `SUCCESS_WITH_RECEIPT` < `PARTIAL_FAILURE` < `NEEDS_HUMAN` < `FAILED_TIMEOUT` < `FAILED_LIMIT_EXCEEDED` < `FAILED_SAFETY` < `FAILED_INTERNAL_ERROR`

## Examples of False Success Caught

1. **Failed tests with SUCCESS claim**: Runner exits 0, makes changes, claims success. Test output shows FAIL. Pinocchio catches via `checkSuccess` false claim: "SUCCESS claimed but test output indicates failure" → downgrades to NEEDS_HUMAN.

2. **C1 receipt mismatch**: Runner exits 0, Forge says SUCCESS. But c1-receipt.json has `exit_code=1`. Pinocchio flags inconsistency and recommends NEEDS_HUMAN.

3. **Safety violation with SUCCESS claim**: Forge says SUCCESS, but forbidden file is detected. Pinocchio's safety findings override to FAILED_SAFETY.

4. **NOOP with changes**: Forge says NOOP, but diff has 3 files changed. Pinocchio false claim: "NOOP claimed but files_changed = 3" → NEEDS_HUMAN.

5. **Partial failure without evidence**: Forge says PARTIAL_FAILURE but exit code is 0, no diff, no test output. Pinocchio flags inconsistency.

## Negative Control

`c1-loop.sh` has a `LYING` mode that:
- Breaks a test file
- Runs `go test ./...` (which fails)
- Exits 0 (lying about failure)
- Writes `c1-receipt.json` claiming `status=completed, exit_code=0`

The E2E test `TestDaemonPinocchioLyingRunner` verifies that the final verdict is NOT `SUCCESS_WITH_RECEIPT`. The chain is: `MapVerdict` sees exit 0 + diff + FAIL test output → `PARTIAL_FAILURE`. Pinocchio confirms that `PARTIAL_FAILURE` is justified given the evidence.

## Safety Boundaries

The verifier catches:
- Push / deploy / npm publish commands (via `classifySafetyFindings` keyword matching)
- `.env` access attempts
- Forbidden claim terms (PROFITABLE, LIVE_READY, MONEY_ENGINE, etc.)
- Forbidden file edits
- Secret/API key patterns
- External directory access patterns (guarded by `CheckForbiddenFileEdit`)

## Files Changed

- `internal/pinocchio/pinocchio.go` — new: `VerifyForgeReceipt`, `PinocchioResult`, `C1Receipt`, `WritePinocchioArtifact`, `classifySafetyFindings`, `testOutputPassed`, `countDiffStats`, `parseC1Receipt`
- `internal/pinocchio/pinocchio_test.go` — new: 12 unit tests covering all verdict paths
- `internal/receipt/receipt.go` — updated: `ForgeReceipt` with 8 new Pinocchio fields; `WriteReviewMD` with Pinocchio section; `formatReceiptMD` with Pinocchio section
- `cmd/c1-forged/main.go` — updated: Pinocchio integration in `processOneTask` (12c-12d); imports; `pinocchioVerdictSeverity()`; `formatOverrideReason()`
- `cmd/c1-forged/main_test.go` — updated: 6 new E2E tests (`TestDaemonPinocchioSuccess`, `TestDaemonPinocchioDowngradesFalseSuccess`, `TestDaemonPinocchioFailsSafetyOnForbiddenFile`, `TestDaemonWritesPinocchioArtifact`, `TestReceiptIncludesPinocchioFields`, `TestDaemonPinocchioLyingRunner`)
- `scripts/c1-loop.sh` — updated: new `LYING` goal mode for negative control

## Remaining Gaps

- `classifySafetyFindings` uses simple keyword matching — could produce false positives
- C1 receipt parsing is minimal (no verdict field, only exit_code and status)
- No multi-round loop yet — Pinocchio only runs once per task
- No ntfy integration
- No web UI or dashboard
- Pinocchio does not validate semantic correctness of diffs — only structural consistency
