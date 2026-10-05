# C1 Forge Developer Documentation

Complete documentation for developers contributing to or extending C1 Forge.

## Table of Contents

### Core Documentation

| Document | Description |
|----------|-------------|
| [User Guide](user-guide/README.md) | **Start here** — how to use C1 Forge in your projects |
| [Architecture](architecture/README.md) | System design, components, data flow |
| [API Reference](api/README.md) | Go packages, interfaces, function signatures |
| [Configuration](configuration/README.md) | All config options, YAML reference |
| [Development](development/README.md) | Setup, testing, contributing guidelines |
| [Deployment](deployment/README.md) | Production deployment, CI/CD, Docker |
| [Plugins](plugins/README.md) | OpenCode plugin development, tool creation |

### Additional Resources

| Document | Description |
|----------|-------------|
| [Demo](demo.md) | Step-by-step walkthrough |
| [White Paper](white-paper.md) | Problem/solution overview |
| [Landing Page](index.html) | Project website |

---

## Quick Links

### For New Developers

1. Start with [User Guide](user-guide/README.md) for quick setup
2. Read [Architecture](architecture/README.md) for system overview
3. Review [API Reference](api/README.md) for package details

### For Contributors

1. Read [Development Guide](development/README.md) for conventions
2. Check [Configuration](configuration/README.md) for config options
3. Review [Plugins](plugins/README.md) for plugin development

### For Deployment

1. Start with [Deployment Guide](deployment/README.md)
2. Review [Configuration](configuration/README.md) for production settings
3. Check [Architecture](architecture/README.md) for system requirements

---

## Getting Started

```bash
# Clone the repository
git clone https://github.com/desmondkam/selo.git
cd selo

# Build
go build -o selo ./cmd/selo/

# Run tests
go test ./...

# Run vet
go vet ./...
```

---

## Architecture Overview

```
selo
├── cmd/selo/          # CLI (Cobra)
│   ├── root.go             # Root command, global flags
│   ├── cmd_daemon.go       # Daemon mode
│   ├── cmd_run.go          # One-shot execution
│   ├── cmd_status.go       # Status display
│   ├── cmd_init.go         # Workspace init
│   ├── cmd_check.go        # GateChain check
│   ├── cmd_smoke.go        # Smoke tests
│   ├── cmd_soak.go         # Soak tests
│   └── cmd_completion.go   # Shell completions
├── internal/
│   ├── containment/        # Unified isolation interface
│   ├── opencode/           # Native Go OpenCode adapter
│   ├── pipeline/           # Safety pipeline
│   ├── receipt/            # Cryptographic receipts
│   ├── governor/           # Policy enforcement
│   ├── pinocchio/          # Consistency checks
│   ├── gatechain/          # Compliance gates
│   ├── runner/             # C1 Loop runner
│   ├── queue/              # Task queue
│   ├── workspace/          # Git worktree management
│   └── ...
├── .opencode/              # OpenCode plugin
│   ├── plugin.ts           # Plugin entry point
│   └── tools/              # Plugin tools
└── docs/                   # This documentation
```

---

## Key Concepts

### Task Lifecycle

```
1. User submits task (CLI or plugin)
2. Task queued in .selo/queue/pending/
3. Daemon claims task → moves to queue/running/
4. Worktree created (isolation)
5. Agent executes in worktree
6. Safety pipeline runs (4 stages)
7. GateChain decides verdict
8. Receipt written to runs/
9. Task moved to queue/done/ or queue/failed/
```

### Safety Pipeline

```
Governor → Scans → Pinocchio → GateChain
   ↓         ↓         ↓           ↓
 Policy   Secrets   Consistency  Compliance
 Claims   Files     Tests        Receipts
 Timeout  Forbidden Phantom      Verdict
```

### Containment Strategies

| Strategy | Isolation | Use Case |
|----------|-----------|----------|
| `worktree` | Git worktree | Lightweight, host-based |
| `docker` | Container | Strong, network-isolated |
| `local` | Process | Development, testing |

---

## Contributing

See [Development Guide](development/README.md) for:

- Code style and conventions
- Testing requirements
- Pull request process
- Issue templates

---

## License

Apache-2.0 — see [LICENSE](../LICENSE) for details.

"Selo" is a trademark of Anomaly. Forks must use a different name to
avoid confusion.
