# Selo

**Safety-first task runner for AI coding agents with cryptographic audit trails.**

Selo wraps any coding agent (OpenCode, Claude, GPT, etc.) and produces a tamper-proof receipt for every task. It ensures AI-generated code changes are verified, tested, and auditable before reaching production.

## Features

- **Safety Pipeline**: Governor, scans, Pinocchio, GateChain — multi-layer verification
- **Cryptographic Receipts**: Tamper-proof audit trail for every task
- **OpenCode Integration**: Native Go adapter, plugin tools for seamless workflow
- **Containment**: Git worktree isolation (the `docker`/`local` strategies are refused with an error — not implemented in v0.2)
- **Permission Control**: Granular allowlists for file access
- **CLI + Daemon**: Run one-shot tasks or start a processing queue

## What works in v0.2

| Capability | Status in v0.2 |
|---|---|
| Post-run safety audit (forbidden file edits, forbidden claims, secret scan, patch/round limits, test integrity) | Enforced on the diff and worktree after the agent runs. Violations reject the task and produce a signed receipt. Claims matching is case-insensitive and folds common evasion (leetspeak, zero-width characters). |
| `opencode.permission_allowlist` | Enforced as a post-run scope check: with a non-empty allowlist, any changed file that does not match it (repo-relative glob patterns; directory prefixes like `src/` or `src/**`) rejects the task. It does not sandbox the agent process itself. |
| `selo verify` | New in v0.2. Re-checks a receipt's content hash and Ed25519 signature; `--anchor` also verifies the git anchor. |
| `selo receipt` | New in v0.3. `receipt list` and `receipt show` render a receipt as a decision card — always including its integrity state — with `--format github` ready for a CI job summary. |
| Signed receipts | Ed25519 over canonical receipt JSON. `selo keys generate` writes a signing key (mode 0600) and prints the export line and public key; without `SELO_SIGNING_KEY` an ephemeral per-process key is used, with a warning. |
| Containment | Git worktree only. `containment.strategy: docker` or `local` is refused with an error instead of silently running in a worktree. |
| Real-time interception | Does not exist. The audit is post-execution only. |

## Quick Start

```bash
# Build
go build -o selo ./cmd/selo/

# Initialize workspace
./selo init

# One-time: make receipt signatures attributable across runs
./selo keys generate

# Run a single task
./selo run "fix the login bug"

# Start daemon for queue processing
./selo daemon

# Check status
./selo status
```

> History: v0.1 was the first public release under the Selo name.

## Use in CI

Surface the verdict where the merge decision happens — a GitHub Actions job
summary, no extra permissions or tokens needed:

```yaml
- name: Run Selo task
  run: |
    go build -o selo ./cmd/selo/
    ./selo run "NOOP" || true   # the receipt records the outcome either way

- name: Post receipt card
  run: |
    receipt=$(ls -t receipts/*.json | head -1)
    ./selo receipt show "$receipt" --format github >> "$GITHUB_STEP_SUMMARY"
```

The card always shows the receipt's integrity state. Anyone can reproduce the
check with `selo verify <receipt.json>`.

## ZCode Integration

A ZCode skill teaches ZCode to submit tasks to the Selo daemon and surface
the receipt verdict in the conversation before claiming a task done:

```bash
# Into a project (tracked with the repo) or ~/.agents/skills/ (all projects)
cp -r .agents/skills/selo <project>/.agents/skills/
```

To wrap ZCode itself as the runner — Selo contains ZCode's run in a worktree,
audits the diff, and signs a receipt — see `config/selo.zcode.yaml`. Point
`runner.command` at your ZCode CLI's non-interactive mode; `{{task_file}}` and
`{{worktree}}` are substituted at run time.

## OpenCode Integration

Selo integrates with OpenCode as a plugin, providing safety tools directly in your editor:

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

## Configuration

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
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/project/src/*.go"
      - "/project/tests/*"
```

## Safety Pipeline

Every task goes through:

1. **Governor**: Policy enforcement, forbidden claims detection
2. **Scans**: Secret scanning, forbidden file edit detection
3. **Pinocchio**: Consistency verification, test integrity checks
4. **GateChain**: Compliance validation, receipt generation

**Non-goals (yet):** generic SAST / CVE scanning is roadmap, not v0.1.
Selo focuses on agent behavior governance: scope violations, test
tampering, secret smuggling, false claims, and auditable receipts.
Third-party scanners (Semgrep, Snyk Code, Codex Security) are welcome
as an optional Check plugin — see `docs/BENCHMARK_SELO_VS_CODEX.md`.

| Capability | Selo | Codex Security |
|---|---|---|
| Finding code vulns (SQLi/auth/OOB) | Weak (limited rules, no PoC validation) | Strong (frontier model + 14 CVE track record) |
| Agent behavior governance | Unique to Selo | Does not exist |
| Non-repudiable audit (signed receipts + hash chain) | Unique to Selo | Does not exist |
| Execution isolation | Standard (worktree / Docker) | Standard (Seatbelt / cloud containers) |

> Scope note: Selo's checks are **post-execution audit** (verdict on the
> diff after the agent runs), not real-time inline interception. A violating
> change is contained in the worktree/container, rejected with a signed
> receipt, and never merged — see `docs/BENCHMARK_SELO_VS_CODEX.md`.

## Architecture

```
selo/
├── cmd/selo/          # CLI commands (Cobra)
├── internal/
│   ├── containment/        # Unified isolation interface
│   ├── opencode/           # Native Go OpenCode adapter
│   ├── pipeline/           # Safety pipeline
│   ├── receipt/            # Cryptographic receipts
│   └── ...
├── .opencode/              # OpenCode plugin
└── .agents/skills/selo/    # ZCode skill
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o selo ./cmd/selo/

# Run vet
go vet ./...
```

## License

Apache-2.0 — see [LICENSE](LICENSE) for details.

"Selo" is a trademark of Anomaly. Forks must use a different name to
avoid confusion.

**Commercial boundary:** the Selo core (CLI, containment, broker,
pipeline, receipt format + verifier) is Apache-2.0 open source. Hosted
receipt ledger, team policy distribution, SSO/RBAC, and compliance
exports will be offered as separate services or private modules.
