# C1 Forge

**Safety-first task runner for AI coding agents with cryptographic audit trails.**

C1 Forge wraps any coding agent (OpenCode, Claude, GPT, etc.) and produces a tamper-proof receipt for every task. It ensures AI-generated code changes are verified, tested, and auditable before reaching production.

## Features

- **Safety Pipeline**: Governor, scans, Pinocchio, GateChain — multi-layer verification
- **Cryptographic Receipts**: Tamper-proof audit trail for every task
- **OpenCode Integration**: Native Go adapter, plugin tools for seamless workflow
- **Containment Strategies**: Git worktrees (lightweight) or Docker (strong isolation)
- **Permission Control**: Granular allowlists for file access
- **CLI + Daemon**: Run one-shot tasks or start a processing queue

## Quick Start

```bash
# Build
go build -o selo ./cmd/selo/

# Initialize workspace
./selo init

# Run a single task
./selo run "fix the login bug"

# Start daemon for queue processing
./selo daemon

# Check status
./selo status
```

> Note: v0.1 renamed the binary from `c1-forged` to `selo` (`cmd/c1-forged` removed).

## OpenCode Integration

C1 Forge integrates with OpenCode as a plugin, providing safety tools directly in your editor:

```bash
# Start OpenCode in your project
opencode

# Use C1 Forge tools
c1forge_daemon_status                     # Check queue state
c1forge_submit_task goal="fix the bug"    # Submit a task
c1forge_task_status taskId="run-xxx"      # Check receipt
c1forge_safety_scan                       # View safety results
c1_forge_compliance_check stepsFile="steps.json"  # GateChain check
```

## Configuration

Create `config/c1-forge.yaml`:

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
c1-forged/
├── cmd/selo/          # CLI commands (Cobra)
├── internal/
│   ├── containment/        # Unified isolation interface
│   ├── opencode/           # Native Go OpenCode adapter
│   ├── pipeline/           # Safety pipeline
│   ├── receipt/            # Cryptographic receipts
│   └── ...
└── .opencode/              # OpenCode plugin
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
