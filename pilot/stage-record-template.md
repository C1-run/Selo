# Stage record — universal template (steps 01–07)

One record per stage per attempt. Copy this file; never overwrite a previous
attempt. Failures and blocked decisions are evidence too.

## Common header (required for every step)

| Field | Value |
|---|---|
| Engagement id | ____ |
| Step | 01 / 02 / 03 / 04 / 05 / 06 / 07 |
| Attempt number | 1 / 2 / … |
| Status | `complete` / `in_progress` / `awaiting_approval` / `blocked` / `rejected` |
| Record version / UTC timestamp | v____ / ____ |
| Actor + role (named human) | ____ |
| Inputs (artifact refs) | ____ |
| Artifacts produced (paths / hashes) | ____ |
| Decision | ____ |
| Reason (why this decision) | ____ |
| Evidence references | ____ |
| Next owner + due date | ____ |

**Status discipline:** `complete` is never inferred from a generated file, a
CLI exit code, a prescreen score, or an unsigned proposal. `awaiting_approval`
means a named human owes a decision. `blocked` / `rejected` must name the
reason and owner. Retries are new records with new evidence, not edits that
make a stage look green.

## Per-step required output (from the Operator Handbook)

| Step | Required output recorded here |
|---|---|
| 01 Touchstone | Prescreen + evidence-backed proceed / refine / stop decision; **no coding authorization** |
| 02 Validate | Feasibility note, dependencies, runnable ACs, stop conditions; buyer + operator agree to scope path |
| 03 Scope | Versioned brief, AC ids, SHA-256, named owners, buyer approval of exact version; blocking unknowns resolved |
| 04 Build | Base + candidate revision, changed files, tool/version, consent reference, redacted logs, operator acceptance |
| 05 Verify | Every AC result (pass / fail / inconclusive / not-run), evidence digests, Selo receipt bindings + digest, operator verdict |
| 06 Ship | Human release authorization, rollback owner/steps, shipped revision, release id/URL, UTC time, buyer notice |
| 07 Measure | Baseline / target / window, observed evidence, buyer feedback, operator hours, repeat / revise / stop / inconclusive |

## Handoff rule

Advance only when the prior record's status is `complete` with a named actor
and the stage gate is evidenced. If a checker is run, record the exact
command and its output — the checker tests record completeness, not the
truth of the underlying claim, and never makes the decision for the human.
