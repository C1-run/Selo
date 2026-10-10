<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/logo-word-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/logo-word-light.svg">
    <img src="docs/brand/logo-word-light.svg" width="300" alt="Selo — Don't trust the agent. Verify the receipt.">
  </picture>
</p>

**Don't trust the agent. Verify the receipt.**

Your coding agent will eventually touch a file you told it not to. Selo catches it, rejects the
task, and hands you a signed receipt.

[![CI](https://github.com/C1-run/Selo/actions/workflows/ci.yml/badge.svg)](https://github.com/C1-run/Selo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/C1-run/Selo)](https://github.com/C1-run/Selo/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/C1-run/selo.svg)](https://pkg.go.dev/github.com/C1-run/selo)
[![Go Report Card](https://goreportcard.com/badge/github.com/C1-run/selo)](https://goreportcard.com/report/github.com/C1-run/selo)

[Changelog](CHANGELOG.md) · [What works](#what-works-in-v05) · [Why not Selo](#why-not-selo) · [Contributing](CONTRIBUTING.md) · [selo.c1.run](https://selo.c1.run)

![Selo catches a forbidden-file edit: the task is rejected with FAILED_SAFETY, and the receipt verifies](docs/demo.gif)

Selo runs any AI coding agent (OpenCode, Claude, GPT, …) in a throwaway worktree, audits **the real
diff** against your rules after the agent finishes, and emits an Ed25519-signed receipt for every
outcome — pass or fail. So the question stops being *"did the agent say it's done?"* and becomes
*"can I check?"*

Three things happened in that demo: the change was **contained** (your checkout was never
touched), it was **caught** (a forbidden-file rule fired on the actual diff), and the rejection is
**verifiable** — `selo verify` re-checks the content hash and signature, and so can anyone else.

## Why Selo

- **Contained.** Every task runs in a throwaway git worktree — the agent never touches your checkout.
- **Audited.** Forbidden files, scope violations (allowlist globs), secrets, forbidden claims, patch/round limits, test tampering — checked on the real diff after the agent runs.
- **Signed.** Every outcome gets an Ed25519-signed receipt over canonical JSON, with a content hash.
- **Verifiable.** `selo verify --pubkey <fingerprint> <receipt.json>` lets anyone — not just you — prove the receipt was signed by the trusted key and not doctored. Without `--pubkey` it still checks internal consistency, but cannot prove who signed.
- **Surfaced.** Receipt cards render into GitHub job summaries and PR comments, where merge decisions happen.
- **Embeddable.** `selo mcp serve` exposes the real checks to any MCP client.

## Quick Start

```bash
# install — the script verifies the binary's checksum and cosign signature
# before installing, and fails closed if either check fails
# (or: go install github.com/C1-run/selo/cmd/selo@latest)
curl -fsSL https://raw.githubusercontent.com/C1-run/Selo/main/install.sh | bash

# one-time: make signatures attributable across runs
selo keys generate   # writes ~/.selo/signing-key, which Selo then loads automatically — no env var needed

# initialize a workspace and run a task
selo init
selo run "fix the login bug"

# read the verdict — always from the receipt, never from the agent
selo receipt show run-<id> --format text
selo verify runs/run-<id>/receipt.json --pubkey <your-fingerprint>   # proves the signer

# your fingerprint (sha256 of the public key) — share it out of band so others can verify:
selo keys pub   # prints the public key; fingerprint = sha256 of that key
```

## What works in v0.5

| Capability | Status in v0.5 |
|---|---|
| Post-run safety audit (forbidden file edits, forbidden claims, secret scan, patch/round limits, test integrity) | Enforced on the diff and worktree after the agent runs. Violations reject the task (exit 1) and produce a signed receipt. |
| `opencode.permission_allowlist` | Enforced as a post-run scope check: with a non-empty allowlist, any changed file that does not match it (repo-relative glob patterns; directory prefixes like `src/` or `src/**`) rejects the task. It does not sandbox the agent process itself. |
| `selo verify` | Re-checks a receipt's content hash and Ed25519 signature; `--anchor` also verifies the git anchor. With `--pubkey <fingerprint\|file>` it proves the signer (provenance); without it, reports `Provenance: UNPINNED_*` — internally consistent but not proven who signed. |
| `selo receipt` | `receipt list` and `receipt show` render a receipt as a decision card — always including its integrity state — with `--format github` ready for a CI job summary; `receipt show` also takes `--pubkey`. |
| `selo receipt export` | Re-signs a receipt as an **in-toto Statement v1** wrapped in a **DSSE** envelope (`--format in-toto`), so third parties can verify with standard tooling (cosign, slsa-verifier) without installing Selo. The signature covers the DSSE PAE over the exact statement bytes. `selo verify` accepts an envelope too. The native `receipt.json` is unchanged. |
| Signed receipts | Ed25519 over canonical receipt JSON. `selo keys generate` writes a signing key (mode 0600), which Selo then **loads automatically** (`~/.selo/signing-key`) — no env var needed. Signing is fail-closed: with no key at all, `selo run` exits non-zero unless you pass `--dev` (ephemeral per-process key, stamped `key_mode: ephemeral`). Every receipt records `key_mode`, and binds the audited change (`diff_hash`), the effective policy (`policy_hash`), and the build (`selo_version`) into the signed bytes. |
| Pluggable signer | `SELO_SIGNER` picks where the key lives: `file` (default), `keychain` (seed in the OS keychain — `selo keys store --keychain`), or `command` (an external program signs; Selo holds no key at all — `SELO_SIGNER_COMMAND`). Only `command` with a non-extractable key moves the signer out of the agent's trust domain (ADR-008). Every receipt records the signed `key_source`, and a third party **enforces** that separation with `selo verify --require-keysource=command` — a receipt signed by the same user's `file` key is then rejected (ADR-008). |
| Trusted timestamp | `selo run --tsa <url>` obtains an **RFC3161 timestamp** over the receipt hash from an external authority (ADR-005), so the signing time cannot be back-dated. `selo receipt timestamp <id> --tsa <url>` retrofits one onto an existing receipt. `selo verify --tsa-ca <pem>` anchors the token to a trusted TSA (chain + timestamping EKU); `--require-tsa` gates on it. Fail-closed: an unreachable TSA fails the run unless `--tsa-soft` records the absence explicitly. |
| Transparency log | `selo run --rekor <url>` logs the receipt hash in a **Sigstore Rekor** log (ADR-006), so a receipt cannot be equivocated or quietly withdrawn. Only the hash is published — Rekor stores the envelope/payload *hashes*, never the receipt, task name, or file paths. `selo verify --rekor-pubkey <pem>` recomputes the RFC 6962 inclusion proof to the checkpoint root and verifies the log's checkpoint signature; `--require-rekor` gates on it. |
| Verified releases | Selo's own releases are signed and independently verifiable (ADR-002, ADR-003). Each ships `checksums.txt` (sorted SHA-256), `checksums.txt.sigstore.json` (cosign **keyless** signature over it — no long-lived key), and `multiple.intoto.jsonl` (**SLSA Build L3** provenance). `install.sh` verifies the checksum and the signature, pinning **both** the workflow identity and the OIDC issuer, and fails closed if either check fails or `cosign` is absent. Verify by hand with `cosign verify-blob` and `slsa-verifier` — see SECURITY.md "Verifying a release". |
| `selo mcp serve` | Exposes the real checks over the Model Context Protocol (stdio), so MCP clients audit with the actual engine instead of a reimplementation. |
| Containment | Git worktree only. `containment.strategy: docker` or `local` is refused with an error instead of silently running in a worktree. |
| Real-time interception | Does not exist. The audit is post-execution only. |

## Why not Selo

Honest counterpoints — read these before adopting:

- **It is post-execution audit, not prevention.** Selo documents and rejects violating changes inside
  a contained worktree; it does not intercept the agent mid-run. A violating change is contained and
  never merged, but it did happen.
- **The checks are heuristics, not SAST.** Forbidden claims and secret scans are pattern-based
  (with leetspeak/zero-width folding). Selo governs agent *behavior*; it does not find SQLi. Pair it
  with a real scanner — see [the pipeline](#the-pipeline) below.
- **Containment is worktree-only.** Docker isolation is refused, not silently degraded. If you need
  container isolation today, Selo is not it.
- **The signer and the agent share a trust domain by default.** `selo run` strips the signing key
  from the agent's environment, but an agent running as the *same OS user* on the same host can still
  read `~/.selo/signing-key` and forge a receipt that passes `selo verify --pubkey`. Treat a receipt
  as tamper-evident evidence of what Selo observed, **not** as non-repudiable proof of agent
  behavior, unless you run the agent in a separate trust domain (different user, container, or a CI
  job that cannot read the key). See [SECURITY.md](SECURITY.md#threat-model-what-a-receipt-does-and-does-not-prove).
  The separation is **enforceable**, not just advisory: `selo verify --require-keysource=command`
  rejects any receipt whose key was not delegated to an external signer (ADR-008).
- **"Non-repudiable" is conditional, not a default.** With `--tsa` (ADR-005) and `--rekor` (ADR-006)
  a receipt carries an externally verifiable signing time and a public, append-only log entry. But
  both attest to *a signature by some key* — so they only mean something when that key is outside the
  agent's trust domain (the ADR-008 `command` backend). Under the default `file` backend the honest
  ceiling is "attributable and tamper-evident", not non-repudiable. When you *do* deploy
  `command` with a non-extractable key, `selo verify --require-keysource=command` lets a third party
  **require** that the key was external — turning the claim from marketing into a checkable gate. See
  [ADR-007](docs/decisions/ADR-007-tiered-non-repudiation-claim.md) and
  [ADR-008](docs/decisions/ADR-008-signer-trust-domain.md).
- **No accuracy numbers.** There is no annotated dataset, so no claim is made about how often the
  checks are right.
- **Linux and macOS binaries.** Windows has no release artifacts yet (build from source works).
- **Young project.** v0.5, one maintainer, breaking config changes possible before 1.0 — tracked in
  the [changelog](CHANGELOG.md).

## Use in CI

Surface the verdict where the merge decision happens — a GitHub Actions job summary, no extra
permissions or tokens needed:

```yaml
- name: Run Selo task
  run: |
    go build -o selo ./cmd/selo/
    # `selo run` exits non-zero when the task is rejected — do NOT append
    # `|| true`. That would disable the gate this job exists to enforce.
    ./selo run "NOOP"

- name: Post receipt card
  if: always()   # still surface the receipt when the task was rejected
  run: |
    receipt=$(ls -t receipts/*.json | head -1)
    ./selo receipt show "$receipt" --format github >> "$GITHUB_STEP_SUMMARY"
```

The card always shows the receipt's integrity state. Anyone can reproduce the
check with `selo verify <receipt.json>`.

<details>
<summary><strong>OpenCode integration</strong></summary>

Selo ships an OpenCode plugin providing safety tools directly in your editor:

```bash
# Start OpenCode in your project
opencode

# Use Selo tools
selo_daemon_status                     # Check queue state
selo_submit_task goal="fix the bug"    # Submit a task
selo_task_status taskId="run-xxx"      # Check receipt
selo_safety_scan                       # View safety results
selo_compliance_check stepsFile="steps.json"  # GateChain check
```

Or serve the checks over the Model Context Protocol: `selo mcp serve`.

</details>

<details>
<summary><strong>ZCode integration</strong></summary>

A ZCode skill teaches ZCode to submit tasks to the Selo daemon and surface the
receipt verdict in the conversation before claiming a task done:

```bash
# Into a project (tracked with the repo) or ~/.agents/skills/ (all projects)
cp -r .agents/skills/selo <project>/.agents/skills/
```

To wrap ZCode itself as the runner — Selo contains ZCode's run in a worktree,
audits the diff, and signs a receipt — see `config/selo.zcode.yaml`.

</details>

<details>
<summary><strong>Configuration</strong></summary>

Create `config/selo.yaml`:

```yaml
forge:
  poll_interval_sec: 5
  default_max_minutes: 30
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
    permission_allowlist:
      - "/project/src/*.go"
      - "/project/tests/*"
```

The loader refuses config Selo cannot honor (unimplemented containment
strategies, unknown runner modes) and warns about keys it does not read.

</details>

## The pipeline

Every task goes through:

1. **Governor**: Policy enforcement, forbidden claims detection
2. **Scans**: Secret scanning, forbidden file edit detection
3. **Pinocchio**: Consistency verification, test integrity checks
4. **GateChain**: Compliance validation, receipt generation

**Non-goals (yet).** Selo is not a vulnerability scanner. It governs agent
*behavior* — scope violations, test tampering, secret smuggling, false claims —
and produces auditable receipts. It does not find SQLi or CVEs; pair it with a
real SAST tool.

**Scope.** The verdict is a **post-execution audit** of the real diff, not
real-time inline interception. A violating change is contained in the worktree,
rejected with a signed receipt, and never merged — but it did happen.

## Architecture

```
selo/
├── cmd/selo/          # CLI commands (Cobra)
├── internal/
│   ├── containment/        # Unified isolation interface
│   ├── mcpserver/          # MCP stdio server (selo mcp serve)
│   ├── opencode/           # Native Go OpenCode adapter
│   ├── pipeline/           # Safety checks
│   ├── receipt/            # Cryptographic receipts
│   └── ...
├── .opencode/              # OpenCode plugin
└── .agents/skills/selo/    # ZCode skill
```

## Roadmap

Selo is v0.5 — one maintainer, moving deliberately. The near-term focus is the
engine: tighter checks, and the pieces needed for anyone to verify a receipt
independently. No accuracy claims until there is a dataset. See
[CHANGELOG.md](CHANGELOG.md) for what shipped, and
[What works](#what-works-in-v05) for the current capability list.

## Development

```bash
make check    # build + vet + gofmt-check + full test suite
make test     # SELO_HOME=$PWD go test ./... -count=1
make build    # go build -o selo ./cmd/selo/
```

## License

Apache-2.0 — see [LICENSE](LICENSE) for details.

Selo is maintained by [C1-run](https://github.com/C1-run) — reach the team at
team@c1.run. Forks must use a different name to avoid confusion.

**Open-core boundary:** everything in this repository is open source under
Apache-2.0 — the CLI, containment, every safety check, the receipt format,
the verifier, the plugins, and the MCP server. Hosted, org-scale features
(central receipt ledger, team policy distribution, SSO/RBAC, compliance
exports) may be offered as separate commercial services later; none of that
is in this repository, and none of it is being built yet. The checks and the
verifier will never be paywalled — a receipt you cannot verify independently
is not non-repudiable.
