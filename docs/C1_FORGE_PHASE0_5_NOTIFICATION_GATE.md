# C1 Forge Phase 0.5 — ntfy/stdout Notification Gate

## Objective
Replace the ntfy stub with a minimal real notification adapter while preserving stdout fallback.

## Supported Modes

| Mode | Behavior |
|---|---|
| `stdout` (default) | Print notification to stdout with severity prefix |
| `ntfy` | HTTP POST to `ntfy_url` with title, priority, body |
| `disabled` | Silently discard |

## Notification Interface

```go
type Notifier interface {
    Notify(ctx context.Context, n Notification) error
}
```

Implementations:
- `StdoutNotifier` — thread-safe, writes to `io.Writer`
- `NtfyNotifier` — HTTP POST with configurable timeout
- `DisabledNotifier` — no-op

## Notification Timing

1. Receipt written (JSON + MD)
2. Review.md written (if applicable)
3. Task moved to done/failed/review
4. Notification sent
5. Receipt JSON updated with notification outcome

This ensures all artifacts exist before the human is notified.

## Notification Payload (ntfy)

```
POST <ntfy_url>
Title: C1 Forge <final_verdict>
Priority: 5 (safety) / 4 (needs_human) / 3 (default)
Content-Type: text/plain; charset=utf-8

C1 Forge task complete
task: <task_id>
repo: <repo>
verdict: <final_verdict>
overridden: <true/false>
tests: <pass/fail>
scans: <pass/fail>
files: <n>
receipt: <path>
review: <path>
```

## Severity Mapping (stdout)

| Verdict | Prefix |
|---|---|
| SUCCESS / NOOP | `✓ C1 Forge` |
| PARTIAL / NEEDS_HUMAN | `! C1 Forge` |
| FAILED_SAFETY | `!! C1 Forge SAFETY` |
| FAILED_TIMEOUT / LIMIT / INTERNAL_ERROR | `X C1 Forge` |

## Config

```yaml
notify_config:
  mode: stdout | ntfy | disabled
  ntfy_url: "https://ntfy.sh/<topic>"
  timeout_seconds: 5
```

Legacy `notify:` and `ntfy_topic:` fields are auto-migrated.

## Failure Behavior

- Notification failure does NOT change task verdict
- Failure is recorded in receipt JSON (`notification_error`)
- Notification is best-effort

## Receipt Fields

New fields in `receipt.json` and `receipt.md`:
- `notification_mode` — the mode used
- `notification_success` — whether the notification was delivered
- `notification_error` — error message if delivery failed

Review.md includes a Notification section.

## Files Changed

| File | Change |
|---|---|
| `internal/notify/notify.go` | NEW: Notifier interface, StdoutNotifier, NtfyNotifier, DisabledNotifier, Config, NewNotifierFromConfig |
| `internal/notify/notify_test.go` | NEW: 10 unit tests |
| `internal/receipt/receipt.go` | ADD: notification_mode, notification_success, notification_error to ForgeReceipt; notification section in formatReceiptMD and WriteReviewMD; REMOVED: old Notify() stub |
| `cmd/c1-forged/main.go` | ADD: notify import, NotifyConfig field, sendNotification(), updateReceiptNotification(), notification flow in processOneTask; notify_config parsing in loadConfig; legacy migration |
| `cmd/c1-forged/main_test.go` | ADD: 3 E2E tests |

## Test Results

```
All ~80+ tests pass
+ 10 notify unit tests
+ 3 daemon E2E tests
+ 0 regressions
```

| Package | Tests | Status |
|---|---|---|
| `internal/notify` | 10 | PASS |
| `internal/pinocchio` | 12 | PASS |
| `internal/governor` | 5 | PASS |
| `internal/queue` | 3 | PASS |
| `internal/runner` | 14 | PASS |
| `cmd/c1-forged` | 25+ | PASS |

## Safety Boundaries

- ntfy_url is configurable by operator (cannot be set in task file)
- No secrets/tokens in receipt.md or review.md
- Forbidden command classifier unchanged
- Forbidden claims unchanged
- Notification failure cannot alter verdict

## Remaining Gaps

- No C1 Notes integration
- No web dashboard
- No VPS deployment
- No OpenCode integration
- No multi-round loop
- ntfy auth headers not supported (no .env/token config)
