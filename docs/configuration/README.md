# Configuration Reference

Complete configuration options for C1 Forge.

## Configuration File

C1 Forge uses YAML configuration. The default location is `config/selo.yaml`.

### File Locations

| Priority | Location | Description |
|----------|----------|-------------|
| 1 | `--config` flag | Command-line override |
| 2 | `./config/selo.yaml` | Project-local config |
| 3 | `~/.config/c1-forge/config.yaml` | User-global config |
| 4 | Built-in defaults | Fallback values |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SELO_CONFIG` | Path to config file | `config/selo.yaml` |
| `SELO_OPENCODE_BIN` | Path to OpenCode binary | Auto-discover |
| `SELO_DANGEROUSLY_SKIP_PERMISSIONS` | Override permissions flag | `false` |

---

## Full Configuration Reference

```yaml
# config/selo.yaml

# Forge settings
forge:
  # Poll interval for daemon (seconds)
  poll_interval_sec: 5
  
  # Default maximum minutes per task
  default_max_minutes: 30
  
  # Default maximum rounds per task
  default_max_rounds: 3
  
  # Default maximum files changed per task
  default_max_files: 10
  
  # Default maximum patch lines per task
  default_max_patch_lines: 200
  
  # Stop file path (relative to worktree)
  stop_file: ".selo-stop"
  
  # Lock file path (relative to base dir)
  lock_file: ".selo.lock"
  
  # Notification mode: stdout, ntfy, none
  notify: stdout
  
  # Notification configuration
  notify_config:
    # Notification mode
    mode: stdout
    
    # Ntfy URL (for ntfy.sh)
    ntfy_url: "https://ntfy.sh/your-topic"
    
    # Timeout for notifications (seconds)
    timeout_seconds: 5
  
  # Ntfy topic (legacy, use notify_config.ntfy_url)
  ntfy_topic: ""
  
  # Forbidden claims (block tasks containing these terms)
  forbidden_claims:
    - PROFITABLE
    - LIVE_READY
    - MONEY_ENGINE
    - CAPITAL_APPROVED
    - LIVE_CAPITAL_APPROVED
    - MONEY_MACHINE
  
  # Runner configuration
  runner:
    # Runner mode: opencode, c1-loop, mock
    mode: opencode
    
    # Runner command (binary path or command)
    command: opencode
    
    # Extra arguments for the runner
    args:
      - "--model"
      - "anthropic/claude-sonnet-4-5"
  
  # OpenCode-specific configuration
  opencode:
    # Model to use
    model: anthropic/claude-sonnet-4-5
    
    # Agent to use
    agent: default
    
    # Serve timeout (seconds)
    serve_timeout: 30
    
    # Dangerously skip permissions (WARNING: disables safety checks)
    dangerously_skip_permissions: false
    
    # Permission allowlist (glob patterns)
    permission_allowlist:
      - "/project/src/*.go"
      - "/project/tests/*"
      - "/project/cmd/*"
```

---

## Configuration Sections

### `forge` (Root)

The top-level configuration section.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `poll_interval_sec` | int | `5` | Daemon poll interval (seconds) |
| `default_max_minutes` | int | `30` | Default task timeout |
| `default_max_rounds` | int | `3` | Default max agent rounds |
| `default_max_files` | int | `10` | Default max files changed |
| `default_max_patch_lines` | int | `200` | Default max patch lines |
| `stop_file` | string | `.selo-stop` | Stop file path |
| `lock_file` | string | `.selo.lock` | Lock file path |
| `notify` | string | `stdout` | Notification mode |
| `forbidden_claims` | []string | [...] | Blocked terms |

### `forge.runner`

Agent runner configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mode` | string | `opencode` | Runner mode |
| `command` | string | `opencode` | Runner command |
| `args` | []string | `[]` | Extra arguments |

**Runner Modes:**

| Mode | Description |
|------|-------------|
| `opencode` | Native Go OpenCode adapter |
| `c1-loop` | Shell-based C1 Loop runner |
| `mock` | Mock agent for testing |

### `forge.opencode`

OpenCode-specific configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `model` | string | `anthropic/claude-sonnet-4-5` | AI model |
| `agent` | string | `default` | Agent name |
| `serve_timeout` | int | `30` | Serve timeout (seconds) |
| `dangerously_skip_permissions` | bool | `false` | Skip permissions |
| `permission_allowlist` | []string | `[]` | File allowlist |

### `forge.notify_config`

Notification configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mode` | string | `stdout` | Notification mode |
| `ntfy_url` | string | `""` | Ntfy URL |
| `timeout_seconds` | int | `5` | Timeout (seconds) |

**Notification Modes:**

| Mode | Description |
|------|-------------|
| `stdout` | Print to terminal |
| `ntfy` | Send via ntfy.sh |
| `none` | Disable notifications |

---

## Examples

### Minimal Configuration

```yaml
forge:
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
```

### Development Configuration

```yaml
forge:
  poll_interval_sec: 2
  default_max_minutes: 10
  runner:
    mode: mock
  opencode:
    dangerously_skip_permissions: true
    permission_allowlist:
      - "*"
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
    ntfy_url: "https://ntfy.sh/c1-forge-alerts"
    timeout_seconds: 10
  forbidden_claims:
    - PROFITABLE
    - LIVE_READY
    - MONEY_ENGINE
    - DEPLOY_NOW
    - SKIP_TESTS
  runner:
    mode: opencode
    command: /usr/local/bin/opencode
    args:
      - "--model"
      - "anthropic/claude-sonnet-4-5"
      - "--agent"
      - "default"
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
    serve_timeout: 60
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/project/src/*.go"
      - "/project/internal/**/*.go"
      - "/project/cmd/**/*.go"
      - "/project/tests/**/*"
      - "/project/config/*"
```

### CI/CD Configuration

```yaml
forge:
  poll_interval_sec: 1
  default_max_minutes: 15
  runner:
    mode: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/github/workspace/**/*.go"
```

---

## Permission Allowlist

The `permission_allowlist` restricts which files the agent can modify.

### Pattern Syntax

| Pattern | Description | Example Match |
|---------|-------------|---------------|
| `*.go` | Any .go file | `main.go` |
| `src/*.go` | .go files in src/ | `src/main.go` |
| `src/**/*.go` | .go files recursively | `src/cmd/main.go` |
| `/absolute/path/*` | Absolute path | `/project/main.go` |
| `dir/` | Directory prefix | `dir/file.txt` |

### Examples

```yaml
permission_allowlist:
  # Only Go source files
  - "/project/src/*.go"
  
  # All files in tests/
  - "/project/tests/*"
  
  # Recursive Go files
  - "/project/**/*.go"
  
  # Multiple patterns
  - "/project/src/*.go"
  - "/project/internal/**/*.go"
  - "/project/cmd/**/*.go"
```

### Security Notes

- Empty allowlist = permissive (all files allowed)
- `dangerously_skip_permissions: true` overrides allowlist
- Environment variable `SELO_DANGEROUSLY_SKIP_PERMISSIONS=1` overrides config

---

## Forbidden Claims

The `forbidden_claims` list blocks tasks containing specific terms.

### Default Blocked Terms

```yaml
forbidden_claims:
  - PROFITABLE
  - LIVE_READY
  - MONEY_ENGINE
  - CAPITAL_APPROVED
  - LIVE_CAPITAL_APPROVED
  - MONEY_MACHINE
```

### Custom Terms

```yaml
forbidden_claims:
  - DEPLOY_NOW
  - SKIP_TESTS
  - IGNORE_ERRORS
  - DISABLE_SECURITY
  - BYPASS_CHECKS
```

### How It Works

1. Task goal is scanned for forbidden terms
2. If any term is found, task is blocked
3. GateChain records the violation
4. Receipt shows `STOP` verdict

---

## Validation

### Config Validation Rules

| Rule | Description |
|------|-------------|
| `poll_interval_sec` | Must be > 0 |
| `default_max_minutes` | Must be > 0 |
| `default_max_rounds` | Must be > 0 |
| `default_max_files` | Must be > 0 |
| `default_max_patch_lines` | Must be > 0 |
| `serve_timeout` | Must be > 0 |
| `timeout_seconds` | Must be > 0 |

### Example Validation Error

```
Error: invalid config: poll_interval_sec must be > 0 (got -1)
```

---

## Migration

### From v0 to v1

```yaml
# v0 config
runner:
  mode: shell
  command: ./adapter.sh

# v1 config
runner:
  mode: opencode
  command: opencode
```

### Legacy Fields

| Old Field | New Field | Notes |
|-----------|-----------|-------|
| `notify` | `notify_config.mode` | Migrated automatically |
| `ntfy_topic` | `notify_config.nttfy_url` | Derived automatically |

---

## References

- [Architecture](../architecture/README.md) — System design
- [API Reference](../api/README.md) — Package documentation
- [Development](../development/README.md) — Contributing guide
