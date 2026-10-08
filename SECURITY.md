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

## Known limitations (not vulnerabilities)

These are documented behaviors, not bugs. Please do not report them as
vulnerabilities. If you think one of them is described incorrectly, that is a
documentation issue and an ordinary issue is fine.

1. **Ephemeral signing key (opt-in).** Selo refuses to sign without a
   configured `SELO_SIGNING_KEY` (fail-closed). An ephemeral per-process Ed25519
   key is only used when you pass `selo run --dev` or set
   `SELO_ALLOW_EPHEMERAL_KEY=1`. Such receipts are not attributable across runs
   and are stamped `key_mode: "ephemeral"`; `selo verify` reports them as
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

## Supported versions

`0.5.x` is the current supported line. It is an alpha: expect breaking changes,
incomplete checks, and behavior that moves between minor releases. Reports
against anything older, or against a fork, are out of scope.
