# Selo paid team pilot — delivery kit

**Status: deliverables first. NOT OPEN FOR PAYMENT.**

The landing page describes a *proposed* paid design-partner pilot. This kit
exists so that the offer can actually be delivered with the existing
open-source CLI — not as a promise of hosted features that do not exist.
No payment is accepted until every item below is in place and the written
terms are accepted by both sides.

## Offer boundary (must match the landing page)

- USD 500 **one time**, only after written scope, privacy, IP and acceptance
  terms are accepted. No instant checkout.
- 14 days, one repository, up to two agent-authored changes.
- C1 assists setup of the existing Selo CLI in the client's own workflow.
- One named client-side technical approver inspects the signed receipt as
  part of a real handoff.
- Excluded: hosted scheduling, Vault retention, alerts, policy crosswalks,
  certification, and any claim that Selo prevents unsafe changes.

## Files

| File | Use |
|---|---|
| `scope-template.md` | Written scope: repo, window, paths, checks, changes, named approver |
| `privacy-template.md` | Data handling: what stays local, what is shared, retention/deletion |
| `ip-template.md` | Ownership of client code, CLI license, and receipt status |
| `acceptance-template.md` | What the named approver verifies before acceptance |
| `rehearsal/run.sh` | Offline, reproducible delivery rehearsal with a real signed receipt |
| `rehearsal/receipt.json` | Output of that rehearsal — third-party verifiable, never hand-edited |
| `rehearsal/verification.log` | Binary version, checksum, fingerprint and verifier output |
| `rehearsal/APPROVER_REVIEW.md` | Honest guide to what the receipt proves and does not prove |
| `rehearsal/receipt-dump.txt` | Human-readable rendering of the same signed receipt |
| `check-gates.sh` | Executable readiness check — run it before any paid offer |
| `external-receipts.md` | Opt-in register for the external-receipt kill input (no telemetry) |
| `change-request-template.md` | Written CR record: old/new scope text, version + hash, named re-approval |
| `stage-record-template.md` | Universal step record: status, actor, UTC, evidence, decision, reason |
| `acceptance-check-template.md` | One AC id: pass condition, command, evaluator, receipt bindings |
| `offer-terms-alignment.md` | Handbook ↔ landing term reconciliation; close gaps before opening |
| `rehearsal/seven-step-walkthrough.md` | Step-by-step map of what exists vs what is only a template |

## Operating the seven steps

The Factory workflow is human-gated: an agent may execute an approved build,
but every stage exit is a named human decision. Fill a
`stage-record-template.md` for each step and attempt, use
`acceptance-check-template.md` for every buyer-approved AC id, and record any
buyer-driven modification in `change-request-template.md` with a new version,
a new SHA-256 and fresh written approval. Previous attempts — including
failures — are kept, never overwritten. See `rehearsal/seven-step-walkthrough.md`
for the honest state: only step-01 material and one signed rehearsal receipt
exist today; steps 02–07 are templates awaiting a real engagement.

## Gate — do not send a paid offer until all three hold

Run `./check-gates.sh` (set `SELO=...` to use a specific binary). It must
print `RESULT: READY`; any `NOT READY` line blocks the offer.

1. A delivery rehearsal has been reproduced with a **current release binary**
   and its receipt verifies `Result: VALID` / `Provenance: PINNED`
   (`rehearsal/run.sh`).
2. All four templates are filled in and accepted in writing by both sides
   (filled copies: `scope.md`, `privacy.md`, `ip.md`, `acceptance.md`; the
   `-template.md` files stay as blanks).
3. The client's named technical approver is identified in `scope.md`, and
   the checklist in `rehearsal/APPROVER_REVIEW.md` has been walked through.

If the agreed pilot cannot be delivered with the existing CLI, do not take
payment. The CLI stays free and Apache-2.0; this pilot sells assisted setup
and a recorded review, not a certificate, an answer, or a guarantee.
