# External receipts — opt-in register

Purpose: make the Pro-card kill criterion evaluable without telemetry.

> Rule (board §11, revised 2026-11-18): **30 days with zero payments AND
> fewer than 3 external receipts → the Pro card comes off.** Clicks are
> diagnostics only; self-test traffic is branched out separately.

## What counts as an external receipt

- Produced by an **external user** on their own repository, with their own
  signing key — not C1, not the maintainer, not a paid collaborator.
- The user shares it publicly **or explicitly agrees** to be listed here.
- It verifies with a current release: `Provenance: PINNED`, `Result: VALID`.
- **Mutually unrelated**: different people or teams. Two receipts from one
  person, one organisation, or one agent run are still one source.

## What does not count

- GitHub stars, release downloads, `install.sh` pulls, or page clicks.
- C1's own dogfood receipts, playground fixtures, mocks, or screenshots
  without a receipt file.
- Receipts produced by C1 while setting up a client's pilot — those are
  assisted, not independent external usage.
- A "verified" claim without a receipt the reader can check themselves.

## Register

Current count: **0** (as of 2026-11-19).

| Date | Receipt id | Signer fingerprint | Public link | Checked by | Why unrelated |
|---|---|---|---|---|---|
| _none yet_ | | | | | |

Add one row only after the checks above are satisfied. Keep the receipt's
own bytes; never hand-edit a signed receipt to make it qualify.

## Privacy

This register is **manual and opt-in**. No CLI telemetry is added, no silent
measurement of client runs, and nothing from a client's repository is listed
without their agreement. If consent is withdrawn, remove the row.
