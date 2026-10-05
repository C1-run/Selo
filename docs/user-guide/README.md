# User Guide

How to set up and use C1 Forge in your projects.

## Two Ways to Use C1 Forge

### Option 1: Standalone CLI

Run C1 Forge as a command-line tool in your project.

```bash
# Install
go build -o selo ./cmd/selo/
sudo mv selo /usr/local/bin/

# Use in your project
cd /path/to/your/project
selo init
selo run "fix the login bug"
```

### Option 2: OpenCode Integration

Use C1 Forge as a plugin in OpenCode (your AI coding assistant).

```bash
# Start OpenCode in your project
cd /path/to/your/project
opencode

# Use C1 Forge tools directly in the TUI
c1forge_submit_task goal="fix the bug"
c1forge_task_status taskId="run-123"
```

---

## Quick Start (Standalone)

### Step 1: Install C1 Forge

```bash
# Option A: Build from source
git clone https://github.com/desmondkam/openselo.git
cd selo
go build -o selo ./cmd/selo/
sudo mv selo /usr/local/bin/

# Option B: Download binary
curl -L https://github.com/desmondkam/openselo/releases/latest/download/selo-linux-amd64 -o selo
chmod +x selo
sudo mv selo /usr/local/bin/
```

### Step 2: Initialize Your Project

```bash
cd /path/to/your/project
selo init
```

This creates:
```
your-project/
├── .selo/
│   ├── queue/           # Task queue
│   ├── runs/            # Receipts
│   ├── receipts/        # Historical receipts
│   └── worktrees/       # Isolated workspaces
└── config/
    └── selo.yaml    # Configuration
```

### Step 3: Configure

Edit `config/selo.yaml`:

```yaml
forge:
  runner:
    mode: opencode        # Use OpenCode as the agent
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    dangerously_skip_permissions: false
    permission_allowlist:
      - "src/**/*.go"
      - "tests/**/*.go"
```

### Step 4: Run a Task

```bash
# One-shot execution
selo run "fix the login bug" \
  --repo . \
  --commands "go test ./..." \
  --max-minutes 30

# Or start daemon for continuous processing
selo daemon
```

### Step 5: Check Results

```bash
# View status
selo status

# View specific receipt
cat runs/run-*/receipt.json
```

---

## Quick Start (OpenCode Integration)

### Step 1: Set Up OpenCode

```bash
# Install OpenCode
curl -fsSL https://opencode.ai/install.sh | sh

# Configure API key
export ANTHROPIC_API_KEY=your-key-here
```

### Step 2: Set Up C1 Forge Plugin

```bash
# Clone C1 Forge
git clone https://github.com/desmondkam/openselo.git
cd /path/to/your/project

# Create .opencode directory
mkdir -p .opencode/tools

# Copy plugin files
cp /path/to/selo/.opencode/plugin.ts .opencode/
cp /path/to/selo/.opencode/tools/*.ts .opencode/tools/
cp /path/to/selo/.opencode/package.json .opencode/

# Install dependencies
cd .opencode && npm install && cd ..
```

### Step 3: Configure Plugin

Create `opencode.json` in your project root:

```json
{
  "plugin": ["./.opencode/plugin.ts"]
}
```

### Step 4: Start OpenCode

```bash
opencode
```

### Step 5: Use C1 Forge Tools

In the OpenCode TUI:

```
# Check daemon status
c1forge_daemon_status

# Submit a task
c1forge_submit_task goal="fix the login bug"

# Check task status
c1forge_task_status taskId="run-123"

# View safety scan results
c1forge_safety_scan
```

---

## Integration with AI Assistants

### OpenCode (Recommended)

C1 Forge has native integration with OpenCode:

```bash
# Install OpenCode
curl -fsSL https://opencode.ai/install.sh | sh

# Use C1 Forge in OpenCode
opencode
> c1forge_submit_task goal="add error handling"
```

**Configuration:**

```yaml
# config/selo.yaml
forge:
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
```

### Claude (Anthropic)

C1 Forge can wrap Claude via OpenCode:

```bash
# Configure OpenCode to use Claude
export ANTHROPIC_API_KEY=your-key-here

# Use C1 Forge with Claude
selo run "refactor the auth module" \
  --repo . \
  --commands "go test ./..."
```

**Configuration:**

```yaml
forge:
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
```

### GitHub Copilot

C1 Forge can work alongside GitHub Copilot:

```bash
# Use C1 Forge for safety verification
selo run "add new feature" \
  --repo . \
  --commands "go test ./..." \
  --forbidden-files ".env"
```

### Custom AI Agent

Integrate your own AI agent:

```yaml
forge:
  runner:
    mode: custom
    command: /path/to/your/agent
    args:
      - "--model"
      - "your-model"
```

---

## Configuration Reference

### Minimal Configuration

```yaml
forge:
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
```

### Production Configuration

```yaml
forge:
  poll_interval_sec: 10
  default_max_minutes: 60
  default_max_rounds: 5
  default_max_files: 20
  default_max_patch_lines: 500
  
  notify: ntfy
  notify_config:
    mode: ntfy
    ntfy_url: "https://ntfy.sh/your-topic"
  
  forbidden_claims:
    - DEPLOY_NOW
    - SKIP_TESTS
  
  runner:
    mode: opencode
    command: /usr/local/bin/opencode
    args:
      - "--model"
      - "anthropic/claude-sonnet-4-5"
  
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
    serve_timeout: 60
    dangerously_skip_permissions: false
    permission_allowlist:
      - "src/**/*.go"
      - "tests/**/*.go"
      - "cmd/**/*.go"
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SELO_CONFIG` | Config file path | `config/selo.yaml` |
| `SELO_OPENCODE_BIN` | OpenCode binary path | Auto-discover |
| `SELO_DANGEROUSLY_SKIP_PERMISSIONS` | Override permissions | `false` |
| `ANTHROPIC_API_KEY` | Anthropic API key | Required for Claude |

---

## Common Use Cases

### 1. Safe Code Review

```bash
# Submit code for AI review with safety checks
selo run "review this pull request for security issues" \
  --repo . \
  --commands "go vet ./..." \
  --max-minutes 15
```

### 2. Automated Refactoring

```bash
# Refactor with safety guarantees
selo run "refactor the database layer to use interfaces" \
  --repo . \
  --commands "go test ./..." \
  --forbidden-files ".env"
```

### 3. Bug Fix with Verification

```bash
# Fix bug and verify with tests
selo run "fix the race condition in the worker pool" \
  --repo . \
  --commands "go test -race ./..."
```

### 4. Feature Development

```bash
# Add feature with safety checks
selo run "add rate limiting to the API" \
  --repo . \
  --commands "go test ./..." \
  --max-minutes 60
```

---

## Workflow Examples

### Solo Developer

```bash
# Morning: Review overnight changes
selo status

# During day: Submit tasks as needed
selo run "optimize the query performance" \
  --repo . \
  --commands "go test ./..."

# Evening: Check all receipts
cat runs/*/receipt.json | jq '.verdict'
```

### Team Workflow

```bash
# CI/CD: Run safety checks on PRs
selo run "review PR #123 for security" \
  --repo . \
  --commands "go test ./..." \
  --forbidden-files "secrets.yaml"

# Merge only after GateChain passes
if [ "$(selo status --json | jq -r '.latest_verdict')" = "PASS" ]; then
  git merge feature-branch
fi
```

### Automated Pipeline

```bash
#!/bin/bash
# Run C1 Forge in daemon mode
selo daemon --config /etc/selo/config.yaml &

# Monitor for tasks
while true; do
  selo status --json | jq '.pending'
  sleep 60
done
```

---

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| `opencode not found` | Install OpenCode or set `SELO_OPENCODE_BIN` |
| `model not available` | Check `ANTHROPIC_API_KEY` is set |
| `permission denied` | Check file permissions or use `--repo` flag |
| `queue locked` | Remove `.selo/queue/running/*.lock` |
| `worktree exists` | Run `git worktree prune` |

### Debug Mode

```bash
SELO_DEBUG=1 selo run "fix the bug" --repo .
```

### View Logs

```bash
# Application logs
tail -f /var/log/selo/selo.log

# Systemd logs
journalctl -u selo -f
```

---

## Next Steps

- [Configuration Reference](configuration/README.md) — All config options
- [Architecture](architecture/README.md) — System design
- [Plugin Development](plugins/README.md) — Extend C1 Forge
- [Deployment](deployment/README.md) — Production setup
