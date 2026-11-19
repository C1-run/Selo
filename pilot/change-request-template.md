# Change request — written record template

**Rule:** no oral change is work. A change only exists once this record is
filled, the scope version is incremented, a new SHA-256 is computed and the
buyer approves that exact version/hash. The original records stay unchanged.

| Field | Value |
|---|---|
| Change request id | CR-____ |
| Requested at (UTC) / requested by (name + role) | ____ |
| Engagement id / current scope version | ____ / v____ |
| Proposed new scope version | v____ |
| What changes (quote the old and new text) | ____ |
| Why (new buyer need, evidence, or defect) | ____ |
| Affected acceptance-check IDs | ____ |
| Impact: scope / price / schedule / risk (name each, or "none") | ____ |
| Evidence reference (email, call note, ticket) | ____ |
| Buyer approval: name + role + date + source | ____ |
| New scope body SHA-256 (computed after body is final) | ____ |
| Re-run `scope-hash` / `check-scope` on the new version? | YES / NO |
| Re-verification needed after Build? (new receipt) | YES / NO |
| Operator acceptance + date | ____ |
| Outcome | APPROVED / REJECTED / WITHDRAWN |

## Gate before any work continues

- [ ] The exact old and new wording are recorded; no "small tweak" is silent.
- [ ] Every affected AC id is listed and its evaluator re-named if needed.
- [ ] Buyer named approval for **this version + hash** is attached.
- [ ] Blocking unknowns from the change are resolved or explicitly excluded.
- [ ] If price, timing or risk changes, the commercial terms are amended too.
- [ ] Previous scope version, failed candidates and receipts remain on file.

If any box is unticked: **stop**. Do not let an agent or vendor work from
the chat summary, the old hash, or an unsigned approval.
