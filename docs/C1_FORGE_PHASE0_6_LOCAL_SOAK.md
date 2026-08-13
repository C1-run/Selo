# C1 Forge Phase 0.6 — 24h Local Soak Harness

## What This Phase Proves

- The daemon control loop can process multiple bounded tasks continuously without repo corruption
- Every terminal task produces a receipt (no lost receipts)
- False successes (LYING tasks with SUCCESS verdict) are caught by Pinocchio
- Safety failures from fixture tasks are correctly classified and do not fail the soak
- Notification failures are recorded as warnings and do not alter task verdicts
- Heartbeat artifacts are written throughout the soak
- Summary artifacts contain all required fields
- Orphan worktrees and stale locks are detected and reported

## What It Does NOT Prove

- Real OpenCode or AI coding capability
- Real production repo safety
- VPS or cloud deployment reliability
- C1 Notes UI correctness
- Multi-machine or network resilience
- Live trading / wallet / exchange operation
- Any scenario beyond local daemon reliability under repeated bounded tasks

## Soak Command Examples

```bash
# Default 24h soak with fixture tasks
c1-forged soak --duration 24h --fixture-mode true --out runs/c1_forge_phase0_6

# Short soak for verification (5 minutes, 10 tasks)
c1-forged soak --duration 5m --task-count 10 --interval 10s --fixture-mode --out runs/c1_forge_phase0_6

# Quick smoke soak
c1-forged soak --duration 10s --task-count 5 --interval 1s --fixture-mode --stop-on-safety=false --out /tmp/soak-smoke
```

## Fixture Task Mix

| Goal | Probability | Simulated Verdict | Notes |
|------|-----------|-------------------|-------|
| NOOP | 25% | NOOP_WITH_RECEIPT | No diff, exit 0 |
| DOCS_PATCH | 20% | SUCCESS_WITH_RECEIPT | Clean success |
| FAILING_TEST | 20% | PARTIAL_FAILURE | Test failure, needs human |
| FORBIDDEN_FILE | 15% | FAILED_SAFETY | Hits forbidden file rule |
| LYING | 10% | NEEDS_HUMAN | False success claim caught |
| SCAN_FAIL | 10% | FAILED_SAFETY | Secret/claims scan failure |

## Expected Safety Failures

FORBIDDEN_FILE and SCAN_FAIL tasks always produce FAILED_SAFETY verdicts.
These are expected fixture failures and do NOT fail the soak with `--stop-on-safety=false`.
With `--stop-on-safety=true` (default), safety failures result in PASS_WITH_GAPS.

## Final Verdict Rules

| Condition | Verdict |
|-----------|---------|
| Any real secret leak | FAIL_UNSAFE |
| Any main branch mutation | FAIL_UNSAFE |
| Missing receipt for terminal task | FAIL_DAEMON_UNSTABLE |
| False SUCCESS reaching final receipt | FAIL_UNSAFE |
| Safety failures correctly classified | Does not fail soak |
| Notification failure but verdict preserved | Does not fail soak |
| Duration < 24h | PASS_SHORT_SOAK |
| Duration ≥ 24h, no issues | PASS_24H_LOCAL_SOAK |
| Duration ≥ 24h, safety failures with stop-on-safety | PASS_WITH_GAPS |

## Operator Instructions

1. Build: `go build -o c1-forged ./cmd/c1-forged/`
2. Run short soak: `./c1-forged soak --duration 5m --task-count 10 --interval 10s --fixture-mode`
3. Run full 24h soak: `./c1-forged soak --duration 24h --fixture-mode`
4. Check output: `ls -la runs/c1_forge_phase0_6/`
5. Read summary: `cat runs/c1_forge_phase0_6/SOAK_SUMMARY.json`
6. Check heartbeat: `cat runs/c1_forge_phase0_6/heartbeat.jsonl`

## Artifact Layout

```
runs/c1_forge_phase0_6/
  SOAK_SUMMARY.json     — JSON summary with all tracked fields
  SOAK_SUMMARY.md       — Human-readable summary
  heartbeats.jsonl      — Per-cycle heartbeat snapshots
  queue/
    pending/            — Unprocessed fixture tasks
    done/               — Successfully processed tasks
    failed/             — Tasks that failed safety/timeout
  worktrees/            — Detected orphan worktrees (if any)
```

## Remaining Gaps

- Soak currently simulates verdicts from goal markers rather than running actual c1-loop or fixture_agent
- No integration with real daemon `processOneTask` during soak (future: `--real-mode`)
- Notification failure simulation depends on external test server
- TIMEOUT task not implemented (no safe/fast timeout simulation exists)
- No daemon restart detection
- No automated 24h soak in CI (time constraint)
- Heartbeat is written every interval cycle, not on a separate schedule
