# selo playground

A deterministic, offline demo of Selo's full pipeline. **No network,
no LLM keys, no accounts** — the "agent" is a shell script
(`offender.sh`) that deliberately violates two rules, and every check
that catches it is the real engine, not a staged output.

## What you will see (all real, nothing scripted make-believe)

1. The task runs inside a throwaway git worktree — your checkout is
   never touched.
2. The change is rejected with `FAILED_SAFETY`, and the receipt names
   the reasons at byte level: `.env` modified (forbidden file), plus
   overclaims (`SECURE_BY_DESIGN`, `BATTLE_TESTED`) found in the diff.
3. A receipt is produced anyway — receipts exist for rejections, not
   just successes. Verdict lives in the receipt, never in the agent's
   word.
4. Third-party verification passes with `Provenance: PINNED`,
   `Result: VALID`. Anyone with any Selo install can re-check the same
   receipt — the fingerprint is printed for the sharing path.
5. Tamper test: flip one byte inside the receipt and `selo verify` fails
   with `Result: INVALID` (content hash + signature mismatch). The
   record is tamper-evident — that is the whole point.

## Run it

```
./run.sh        # ~10 seconds; keeps its sandbox open at the end
```

Requirements: `git`, an installed `selo` binary (see the README's
Quick Start), `openssl` (fingerprint math).

## What this proves — and refuses to claim

- **Proves:** containment (all changes happened in the worktree),
  audit (rules fired on the real diff), signing (an Ed25519,
  content-hash-bound, tamper-evident receipt) and pinned verification
  (`selo verify --pubkey <fingerprint>`).
- **Does not prove:** prevention — the violating change is contained
  and recorded *after* it happens; sandboxing — Docker strategy is
  refused by design; non-repudiation while signer and agent share a
  trust domain — here the sandbox key lives in a process as the same
  user. For real separation see ADR-008 (`--signer command` or
  keychain) and SECURITY.md ("Verifying a release", threat model).

The offender is not an agent and not a benchmark: it exists so the
demo cannot flatter the tool. A pipeline that cannot produce a
failure — or cannot replay one offline — shouldn't be trusted for the
failures you actually care about.

Sandbox layout (kept after the run): throwaway git repo +
`config/selo.yaml` (real runner → `./offender.sh`), sandbox signing
key generated fresh (`~/.selo/signing-key` untouched), queue + receipts
under `runs/` and `receipts/`. Delete the directory whenever you like.
