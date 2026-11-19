# Seven-step walkthrough — documentation rehearsal, NOT an engagement

**Status: EXAMPLE — NOT EVIDENCE.** No real buyer, no payment, no agent build,
no deployment has gone through steps 02–07. This document maps each step to
its template and names what genuinely exists today, so a future real run can
be filled in without inventing history. Do not cite this as proof of delivery.

## Step-by-step map

| Step | Template to fill in a real run | What genuinely exists today | Real engagement status |
|---|---|---|---|
| 01 Touchstone | `stage-record-template.md` | Self-hosted intake code; 6 Touchstone + 4 scope records in c1 KV, all internal `@c1.run` probes (10-29 / 11-02); form parked 11-14; one **manual** four-question verdict for siga: **REFINE** (`c1/docs/siga-touchstone-verdict-2026-11-15.md`) | No external buyer submission |
| 02 Validate | `stage-record-template.md` | Feasibility checklist in handbook + `check-validate` checker | Never run |
| 03 Scope | `stage-record-template.md`, `change-request-template.md` | Versioned brief template + `scope-hash` / `check-scope`; change-request is one line in the record | No buyer-approved version/hash |
| 04 Build | `stage-record-template.md`, `acceptance-check-template.md` | Build record template + `check-build` (checks records only; does not run builds) | Never run |
| 05 Verify | `acceptance-check-template.md` + real receipt | **Real evidence**: receipt `c1f-1795016097433669000` (selo v0.7.2, checksum `7ea848e4…`, fp `e9512c4f…`), `receipt.json`, `verification.log`, `receipt-dump.txt`; verify → `Provenance: PINNED` / `Result: VALID` | Receipt proves integrity of a **scripted-offender rehearsal**, not a client delivery |
| 06 Ship | `stage-record-template.md` | `check-ship` / `check-shipped` preflight; manual release rule | Never run |
| 07 Measure | `stage-record-template.md` | `check-measure`; 14-day default checkpoint; missing-evidence rule | Never run |

## How step 05 would bind a real receipt

1. Fill `acceptance-check-template.md` for every AC id; record the exact
   command and the named evaluator.
2. Bind the receipt to: approved brief version + SHA-256, the AC ids run,
   the candidate commit, and each evidence digest.
3. Run `selo verify --pubkey <fp> receipt.json` yourself; paste the exact
   output and retain the original receipt bytes and their SHA-256.
4. Operator writes the verdict and accepts (or blocks) for Ship. A FAIL or
   INCONCLUSIVE is recorded, never silently retried into a pass.

## Why this document exists

The seven-step process is a written operating discipline with record
checkers, not an automated product. This walkthrough makes the gap visible:
the only genuine artefacts are the step-01 prescreen material, one manual
verdict, and a signed receipt for an offline rehearsal. Everything else is
a template waiting for a real engagement with a named buyer and a payment.
