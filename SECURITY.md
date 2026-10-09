# Security Policy

Selo is a safety-first task runner, so we hold its own safety claims to the
same standard we ask of the agents it governs. This document describes what
to report, how to report it, and what is already known and documented.

## Reporting a vulnerability

**Do not open a public issue for a vulnerability.** A public report exposes
every user before a fix is available.

Report privately using GitHub's **"Report a vulnerability"** advisory feature
on this repository (Security tab → Report a vulnerability). That keeps the
report private to the maintainers and gives us a place to coordinate a fix and
a disclosure timeline with you.

Report vulnerabilities by email to **team@c1.run** (or via a GitHub private
security advisory). Please do not include
any vulnerability details in it.

We will acknowledge reports as capacity allows; this is an alpha maintained
by a small team, and we would rather tell you we are slow than promise a
response time we cannot keep.

## In scope

Selo makes specific, testable safety claims. A report is in scope when it shows
one of them is false:

- **Receipt signing and anchoring.** Selo signs each receipt with Ed25519 and
  anchors it to a git branch. In scope: signature forgery, signature bypass,
  canonicalization that lets two different receipts share a signature,
  verification that returns success for a tampered receipt, or anchor commits
  that do not actually bind the receipt hash they claim to.
- **Receipt integrity.** Tampering with a receipt or its metadata that survives
  verification, or a gap that lets a run complete without a receipt.
- **Containment escape.** Escaping the isolated git worktree, or reaching the
  host or another task's worktree from inside a run.
- **Secret-scan bypass.** A changed file that carries a secret the scan is
  supposed to catch but does not, where the secret is written in a form the
  documented patterns should match.
- **Command injection through task files.** Task files are parsed and the
  runner command is executed with `{{task_file}}` and `{{worktree}}`
  substitution. In scope: a task file that causes arbitrary command execution,
  argument injection, or substitution that lets task-controlled text escape its
  argument.
- **Path traversal.** Traversal outside the intended workspace through the
  `repo:` task field (the only path-bearing field a task file can set; it is
  parsed in `receipt.ParseTaskMeta` and used to pick the repository that the
  worktree is created from), or through path handling derived from task input.
- **Safety-check bypass.** A change that violates a configured policy
  (forbidden claims, forbidden files, patch or file limits) and still receives a
  passing verdict.

## Threat model: what a receipt does and does not prove

A receipt is evidence about the *run*, signed by whoever holds the signing key.
It proves that a process holding that key observed a given verdict over a given
diff. It does **not**, by itself, prove that the agent could not have produced
the receipt.

For the guarantee to hold, the signing key must live **outside the trust domain
of the agent being audited**:

- `selo run` removes `SELO_SIGNING_KEY` and `SELO_ALLOW_EPHEMERAL_KEY` from the
  environment of the agent subprocess (`internal/runner`), so the agent cannot
  read the key from its own environment.
- That is necessary but **not sufficient**: an agent running as the *same OS
  user* on the same host can still read `~/.selo/signing-key` from disk. If you
  need a receipt an audited agent cannot forge, run the agent under a different
  user, in a container, or in a CI job that cannot read the key, or sign from a
  keychain/KMS the agent has no access to.
- `SELO_SIGNER` selects the backend: `file` (default), `keychain` (the seed lives
  in the OS keychain, not on disk — `selo keys store --keychain`), or `command`
  (an external program signs and Selo never holds the key — `SELO_SIGNER_COMMAND`
  + `SELO_SIGNER_PUBKEY`). `keychain` raises the bar but the item is still
  readable by the same user; only `command` with a non-extractable key (HSM, KMS,
  ssh-agent) actually moves the signer out of the agent's trust domain. Every
  receipt records which backend signed it, in the signed `key_source` field.
- When the agent runs in the same trust domain as the signer, treat the receipt
  as an integrity and audit record of *what Selo observed*, not as
  non-repudiable proof of agent behavior. Do not use the words
  "non-repudiable" or "tamper-proof" for such a receipt.

The phased plan for closing this gap (env strip → keychain/KMS → separate
signer user or CI signing) is tracked in
[ADR-008](docs/decisions/ADR-008-signer-trust-domain.md).

## Known limitations (not vulnerabilities)

These are documented behaviors, not bugs. Please do not report them as
vulnerabilities. If you think one of them is described incorrectly, that is a
documentation issue and an ordinary issue is fine.

1. **Ephemeral signing key (opt-in).** Selo resolves a persistent key from
   `SELO_SIGNING_KEY`, or automatically from `~/.selo/signing-key` (written by
   `selo keys generate`). If neither exists it refuses to sign (fail-closed). An
   ephemeral per-process Ed25519 key is only used when you pass `selo run --dev`
   or set `SELO_ALLOW_EPHEMERAL_KEY=1`. Such receipts are not attributable across
   runs and are stamped `key_mode: "ephemeral"`; `selo verify` reports them as
   `UNPINNED_EPHEMERAL`. This is a known limitation of dev mode, not a signing
   flaw.
2. **`selo verify` proves provenance only when you pin a key.** Without
   `--pubkey`, `selo verify` confirms only internal self-consistency (content
   hash + signature) and explicitly reports `Provenance: UNPINNED_*`. A receipt
   re-signed by an attacker with their own key still verifies as internally
   consistent. Pass `--pubkey <fingerprint|file>` to prove the signer. Treat
   any unpinned `VALID` as "not proven who signed this," not "trustworthy."
3. **Only `worktree` containment is implemented.** The `worktree` strategy is
   the only one that runs. `docker` and `local` are rejected at startup by
   configuration validation rather than silently downgraded to a weaker
   strategy. Docker is not a supported containment path in the current alpha.
4. **Safety checks are post-execution audit, not real-time interception.** A
   violating change is contained in the worktree and rejected with a receipt;
   nothing blocks a file write as it happens. The verdict is on the diff after
   the agent runs.
5. **The secret scan is a small set of regular expressions**, not a
   general-purpose scanner. It is not expected to catch every secret format.
   Generic SAST and CVE scanning are roadmap, not v0.1.
6. **The signing key shares the agent's trust domain by default.** When the
   agent runs as the same OS user as Selo on the same host, it can read
   `~/.selo/signing-key` from disk and could forge a receipt that passes
   `selo verify --pubkey`. Selo strips the key from the agent's environment
   (`internal/runner`) but cannot stop a same-user disk read. Non-repudiation
   requires running the agent in a separate trust domain — see "Threat model"
   above. This is a deployment limitation, not a signing flaw. Mitigations:
   `SELO_SIGNER=keychain` (seed off disk) or `SELO_SIGNER=command` (Selo holds
   no key at all).

## Supported versions

`0.5.x` is the current supported line. It is an alpha: expect breaking changes,
incomplete checks, and behavior that moves between minor releases. Reports
against anything older, or against a fork, are out of scope.
