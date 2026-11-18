# Delivery rehearsal — approver review guide

**Status: rehearsal reproduced 2026-11-18 with a current release binary.**
This satisfies gate 1 in `../README.md`. Gates 2 and 3 (written terms
accepted, named client approver) are still open — this document is not a
sign-off and no payment is due.

## Actual evidence (reproduce, do not trust the summary)

| Item | Value |
|---|---|
| Receipt id | `c1f-1795016097433669000` |
| Binary | selo **v0.7.2**, sha256 `7ea848e418aa6edfbbd04e033a0a79b7fd0655f5ebbbfc6700b8d60d1ce6dd7c` |
| Signer fingerprint | `e9512c4fc525598a7d8187cef80455d33a7dc9600920a9adaf9396ab0579cbaf` (= SHA-256 of the decoded 32-byte `public_key`) |
| Verdict | `FAILED_SAFETY` — the checks caught a deliberate violation |
| Findings | forbidden path `.env`; claim words `SECURE_BY_DESIGN` / `BATTLE_TESTED` at `src/summary.md:3` |
| Checks semantics | `checks_version: 1` — required-commands gate semantics (0.7.0+) |
| Integrity | `Hash: OK`, `Signature: OK`, `Provenance: PINNED`, `Result: VALID` |
| Command | `selo verify --pubkey <fp> pilot/rehearsal/receipt.json` |

The verifier's full output is in `verification.log`; the human-readable
receipt is in `receipt-dump.txt`. Run `run.sh` yourself to regenerate the
whole thing from scratch — the receipt is engine-signed, never hand-edited.

## What this rehearsal actually proves

1. The existing CLI can be set up in a throwaway repository with a local
   signing key, no network, no API keys, no hosted service.
2. A deliberately offending change is **contained, rejected and recorded**:
   the receipt states the verdict and findings, not the agent's own story.
3. A third party with the pinned fingerprint can verify the receipt
   independently and get `Provenance: PINNED` / `Result: VALID`.
4. The record is tamper-evident — flipping a byte makes verification fail
   (see the tamper beat in `examples/playground/run.sh`).

## What it explicitly does NOT prove

- **Not an LLM agent.** `runner_command` is `./offender.sh`, a deterministic
  shell script; `c1_runtime_is_mock: true` means the C1 orchestration layer
  was mocked. `runner_mode: real` and `runner_binary_kind: opencode`
  describe the *runner binary whose provenance was verified*
  (`runner_binary_path`), **not** the command that was executed.
- **Not independent identity assurance.** The sandbox signing key shares a
  trust domain with the runner. It proves receipt integrity, not who signed
  it in a legal sense; stronger signer separation (ADR-008) is outside the pilot.
- **Not prevention.** The audit runs **after** the agent finishes. The
  violating change did happen inside the contained worktree; Selo recorded
  and rejected it, it did not make the change never occur.
- **Not a security certificate, certification, or compliance attestation.**
  No hosted scheduler, Vault retention, alerting, or policy crosswalk is
  part of this pilot.
- **Not the full pilot.** Only one change was exercised; the offer covers up
  to two. No client approver has reviewed this receipt yet, and no written
  scope has been accepted.

## Approver walkthrough (tick boxes in `../acceptance-template.md`)

1. Check the binary version and sha256 above against `verification.log`.
2. Recompute the fingerprint from `receipt.json`'s `public_key` (decode
   base64, SHA-256) — it must equal the value above.
3. Run the verify command yourself with a current release binary and read
   `Provenance: PINNED` and `Result: VALID` in your own terminal.
4. Read the findings and `diff_summary`; confirm they match the agreed
   scope's forbidden paths and claim words.
5. Record acceptance or rejection in writing. A rejection means **no
   payment**; C1 fixes the setup and issues a new signed run. Failed runs
   stay on the record.
