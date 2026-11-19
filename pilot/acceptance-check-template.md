# Acceptance check — one per AC id

| Field | Value |
|---|---|
| AC id (stable, e.g. AC-03) | ____ |
| Check statement (what must be true) | ____ |
| Observable pass condition | ____ |
| Command / objective evidence method | ____ |
| Environment where it runs | ____ |
| Evaluator (named person + role) | ____ |
| Buyer confirmation source (who, when) | ____ |
| Run result | PASS / FAIL / INCONCLUSIVE / NOT RUN |
| Evidence reference (log, screenshot, digest) | ____ |
| Evidence SHA-256 | ____ |
| Redaction check (secrets removed before sharing) | YES / NO |

## Receipt bindings to record in step 05

- Approved brief version: v____ ; brief SHA-256: ____
- AC ids actually run: ____
- Candidate revision (commit): ____
- Evidence digests: ____
- Receipt id: ____ ; receipt SHA-256: ____
- Signer identity / fingerprint: ____
- `selo verify` command and its exact output: ____

**Read this twice:** a Selo receipt proves the integrity of a record and the
checks that were reported. It does **not** prove the code is correct, that
every risk was found, or that the signer is independent. In the default
pilot the receipt is signed by C1/the operator and is labelled
**tamper-evident operator evidence**; a client-controlled key is optional
when the buyer can supply one in time. The operator gives the verdict —
the tool does not.
