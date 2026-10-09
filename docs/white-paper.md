# C1 Forge: Safety-First AI Code Generation

## The Problem

AI coding agents are transforming software development, but they introduce new risks:

- **Uncontrolled changes**: Agents can modify files without human oversight
- **No audit trail**: Changes lack cryptographic proof of what was done
- **Security vulnerabilities**: Secret leaks, forbidden file access, policy violations
- **Quality gaps**: No automated verification that changes are safe

## The Solution

C1 Forge is a safety-first task runner that wraps any coding agent and produces a cryptographic audit trail for every task. It ensures AI-generated code changes are verified, tested, and auditable before reaching production.

### Key Components

1. **Safety Pipeline**
   - Governor: Policy enforcement
   - Scans: Secret detection, forbidden file detection
   - Pinocchio: Consistency verification
   - GateChain: Compliance validation

2. **Cryptographic Receipts**
   - Signed, tamper-evident audit trail
   - Task metadata, diffs, test results
   - Verdict (pass/review/stop) with reasoning

3. **Containment Strategies**
   - Git worktrees: Lightweight, host-based isolation
   - Docker: Strong container isolation with network=none
   - Local: Process-level isolation for development

4. **OpenCode Integration**
   - Native Go adapter (replaces shell script)
   - Plugin tools for seamless workflow
   - Daemon for queue processing

### How It Works

```
User → C1 Forge → Safety Pipeline → Agent → Receipt
         ↓
    Governor → Scans → Pinocchio → GateChain
         ↓
    Worktree/Container → Agent Execution → Verification
```

### Benefits

- **Safety**: Multi-layer verification prevents dangerous changes
- **Auditability**: Cryptographic receipts for compliance
- **Flexibility**: Works with any coding agent
- **Integration**: Seamless OpenCode plugin
- **Performance**: Native Go implementation

## Use Cases

1. **Enterprise Development**
   - AI-assisted coding with compliance requirements
   - Audit trails for regulated industries
   - Policy enforcement for code changes

2. **Open Source Projects**
   - Safe AI contributions with verification
   - Automated code review with safety checks
   - Community trust through transparency

3. **CI/CD Pipelines**
   - Automated safety checks in CI
   - Receipt generation for deployments
   - Rollback capabilities with audit trails

## Technical Details

### Safety Pipeline

The safety pipeline consists of four stages:

1. **Governor**: Enforces policies (forbidden claims, max rounds, timeouts)
2. **Scans**: Detects secrets, forbidden file edits, policy violations
3. **Pinocchio**: Verifies consistency (no hallucinated tests, no phantom changes)
4. **GateChain**: Validates compliance and generates cryptographic receipts

### Containment

C1 Forge supports three containment strategies:

- **Worktree**: Git worktree-based isolation (lightweight, host-based)
- **Docker**: Container-based isolation (strong, network-isolated)
- **Local**: Process-level isolation (development, testing)

### OpenCode Integration

The OpenCode integration provides:

- **Native Go adapter**: Replaces shell script with native implementation
- **Plugin tools**: Submit tasks, check receipts, view safety results
- **Daemon**: Queue processing for batch operations

## Conclusion

C1 Forge provides the safety layer needed for AI coding agents in production. By combining multi-layer verification, cryptographic receipts, and seamless integration, it enables organizations to leverage AI while maintaining control and compliance.

---

**C1 Forge** - Safety-first AI code generation with cryptographic audit trails.
