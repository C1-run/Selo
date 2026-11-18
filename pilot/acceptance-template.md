# Pilot acceptance — written template (fill before payment)

| Field | Agreed value |
|---|---|
| Named client-side technical approver | _fill_ |
| Receipt(s) under review | _fill_ (receipt id from the run) |
| Verifier binary + version used by approver | _fill_ |
| Decision | _accepted / rejected_ |
| Reasons (if rejected) | _fill_ |
| Date / signature | _fill_ |

## Approver checklist (all boxes must be ticked by the human approver)

- [ ] Confirm the verifier binary version and SHA-256 in
      `rehearsal/verification.log`; use a **current release** (older verifiers
      may reject newer receipt formats — that is a version mismatch, not
      evidence of tampering).
- [ ] Compute the signer fingerprint from the receipt's `public_key`
      (SHA-256 of the decoded 32 bytes) and match it to the log.
- [ ] Run `selo verify --pubkey <fp> receipt.json` and read
      `Provenance: PINNED` and `Result: VALID` yourself. Do not accept a
      screenshot of someone else's run.
- [ ] Check the verdict and findings against the agreed scope: forbidden
      paths and forbidden claim words in `scope-template.md`. A
      `FAILED_SAFETY` verdict means the check **caught** a violation; it is
      not a pass.
- [ ] Read the `diff_summary`; confirm it matches the change you were told
      about and contains nothing you are not willing to share.
- [ ] Understand `checks_version`: receipts from 0.7.0+ state the required-
      commands gate semantics explicitly. Do not read `scans_passed` as
      "tests passed".
- [ ] Confirm the written scope, privacy, IP and acceptance terms are the
      versions actually agreed.
- [ ] Confirm no claim of certification, hosted service, or prevention is
      being made anywhere in the handoff.

## Failure path

If any box cannot be ticked truthfully, **do not accept and do not pay**.
C1 may fix the setup and produce another signed receipt; every run is kept
as a record, including failed ones. There is no "retry until green" story:
a rejection is reported as a rejection. Payment follows written acceptance
only, and the CLI remains free and Apache-2.0 regardless of the outcome.
