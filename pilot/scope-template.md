# Pilot scope — written template (fill before any payment)

| Field | Agreed value |
|---|---|
| Client / legal name | _fill_ |
| Repository (URL or local path) | _fill_ |
| Pilot window (max 14 days) | _fill_ |
| Agent tooling in use | _fill_ |
| Agent-authored changes covered (max 2) | _fill_ |
| Allowed paths | _fill_ |
| Forbidden paths (e.g. `.env`, `*.key`) | _fill_ |
| Forbidden claim words to check | _fill_ |
| Command / test checks to enable | _fill_ |
| Named client-side technical approver | _fill_ |
| Delivery contact on C1 side | team@c1.run |
| Fee | USD 500, one time, payable only after this scope is accepted |
| Sign-off (both sides, date) | _fill_ |

## Included

- Assisted setup of the existing Selo CLI inside the client's own workflow.
- A signed receipt per covered run; the named approver inspects the receipt
  as part of the client's handoff.
- Written handoff of the receipts and verifier instructions.

## Explicitly excluded

- Hosted scheduler, always-on watch, or background daemon.
- Hosted receipt retention (Vault), alert delivery, policy crosswalks.
- Security certification, compliance attestation, or any guarantee that
  unsafe changes are prevented. Selo audits **after** the agent finishes:
  it can contain, reject and record a run under configured checks, but the
  change did happen.

## Amendments

Any change to the repository, window, paths, checks, approver or fee must be
agreed in writing before the work starts. No oral amendment.
