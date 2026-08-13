# C1 Forge Phase 0.4.1 — Forced Verdict Override Negative Control

## Why This Patch Exists

Phase 0.4 proved Pinocchio's consistency checks work at the unit level. But the daemon E2E test (`TestDaemonPinocchioLyingRunner`) only proved that `MapVerdict` catches false success — not that Pinocchio itself can override when a false `SUCCESS_WITH_RECEIPT` reaches the consistency gate.

## What the Previous LYING Mode Proved

- `MapVerdict()` correctly detects test output containing "FAIL" and returns `PARTIAL_FAILURE`
- When `MapVerdict` produces `PARTIAL_FAILURE`, Pinocchio verifies the evidence is consistent
- The total system prevents false SUCCESS from reaching the final receipt

## What It Did Not Prove

- **Pinocchio's override path was never triggered** in daemon E2E — because `MapVerdict` always downgrades before Pinocchio sees the verdict
- `verdict_overridden=true` was never observed in a real daemon run
- The code path at `main.go:315-322` (env override → Pinocchio → override) was untested at the daemon level

## What This Forced Override Test Proves

A direct negative control using `C1_FORGE_TEST_INITIAL_VERDICT` environment variable to bypass `MapVerdict` and inject a false `SUCCESS_WITH_RECEIPT`:

| Field | Value | Meaning |
|---|---|---|
| `initial_verdict` | `SUCCESS_WITH_RECEIPT` | Forced by test env (simulates MapVerdict bug) |
| `final_verdict` | `NEEDS_HUMAN` | Pinocchio downgraded due to test failure |
| `verdict_overridden` | `true` | Override path activated |
| `pinocchio_verified` | `false` | Inconsistency detected |
| `pinocchio_false_claims` | `["SUCCESS claimed but test output indicates failure"]` | Specific lie identified |

The override is deterministic: `VerifyForgeReceipt` finds a false claim, sets `RecommendedVerdict=NEEDS_HUMAN`, and since `NEEDS_HUMAN` has higher severity (3) than `SUCCESS_WITH_RECEIPT` (1), the override is applied.

## Examples of Initial SUCCESS Being Downgraded

| Scenario | Initial | Final | Reason |
|---|---|---|---|
| Tests fail but SUCCESS claimed | SUCCESS | NEEDS_HUMAN | False claim: test output shows FAIL |
| Scan fails but SUCCESS claimed | SUCCESS | FAILED_SAFETY | Safety finding: secret/forbidden detected |
| C1 receipt shows failure | SUCCESS | NEEDS_HUMAN | Inconsistency: C1 receipt exit_code != 0 |
| Diff exists but NOOP claimed | NOOP | NEEDS_HUMAN | False claim: files_changed > 0 |
| No evidence of failure but PARTIAL claimed | PARTIAL | NEEDS_HUMAN | Inconsistency: exit 0, no diff, no test failure |

## Remaining Gaps

- The test hook (`C1_FORGE_TEST_INITIAL_VERDICT`) is test-only and not used in production
- Pinocchio cannot validate semantic correctness of code — only structural consistency of the receipt
- No multi-round loop integration (Pinocchio runs once per task)
- No ntfy/web UI/OpenCode/VPS
