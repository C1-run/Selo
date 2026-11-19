# Offer terms — handbook ↔ landing alignment

The Operator Handbook (`c1/.agents/skills/c1-factory/references/c1-factory-guide.md`,
"Operator reference: first paid Selo pilot") and the landing page must say the
same thing. Any difference below must be closed **before** the NOT OPEN marker
comes off. This is a reconciliation sheet, not a contract.

| Term | Handbook (source of truth) | Landing today (NOT OPEN) | Action before opening |
|---|---|---|---|
| Price / timing | USD 500 **prepaid**; one agency repo; 14 days; up to two customer-run agent changes | "$500 one time · proposed only"; 14 days / one repo / two changes | Add "prepaid before work starts" to written terms; page wording is consistent once scope is accepted |
| Who runs the agent | The agency runs and supervises its **own** coding agent; C1 assists Selo setup + evidence review and **does not write feature code** | "Assisted setup of the existing CLI in your own workflow" | State explicitly in scope: C1 writes no feature code; client's agent is client-run |
| Client reviewer | Named client-side technical approver reruns the agreed commands and verifies the receipt | "One named client-side technical approver reviews the evidence" | Already aligned; name them in scope |
| No eligible change | If none runs in 14 days: **full refund + one free re-run after re-scope** | Not stated on page | Put in written terms; add a one-line page summary |
| C1 effort cap | Keep C1 delivery work **within six hours** (setup, signer, diff review, calls, support) | Not stated | Put in written terms (internal cap; may be summarised on page) |
| Pre-work paperwork | Zoom → one-page recap (repo, changes, exact commands, reviewer, price, no-change outcome) → email confirmation → written code-owner authorization for agent/Selo/C1 access | "Written scope, privacy, IP, acceptance terms before payment" | Attach the recap + authorization as scope exhibits |
| Signer identity | Default receipt signed by **C1/the operator**, labelled "tamper-evident operator evidence"; client key optional | Page says receipts are Ed25519-signed; no default-signer statement | State in acceptance terms; never imply client-controlled identity by default |
| What is not included | No hosted scheduler, Vault retention, alerts, policy crosswalk, certification; no prevention claim | Same exclusions in card fineprint + FAQ | Aligned |
| Payment trigger | Payment in advance, after written scope accepted (not after delivery) | "Payment only follows an accepted written scope; no instant checkout" | Make the sequence explicit: scope signed → payment → work |
| Gate status | Handbook describes the intended workflow; step 01 only, tool form parked | "NOT OPEN — 2 OF 3 GATES PENDING" | Keep until written terms accepted + named approver exists; run `check-gates.sh` |

## Honesty check

Do not publish a term in this sheet that cannot be honoured with the existing
CLI and human process. If a term needs hosted infrastructure or a feature
that does not exist, it does not go on the page — it is recorded as a gap.
