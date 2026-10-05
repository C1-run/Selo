# Deployment Guide

Production deployment, CI/CD, and Docker setup for C1 Forge.

## Deployment Options

| Method | Use Case | Complexity |
|--------|----------|------------|
| Binary | Single server | Low |
| Docker | Containerized | Medium |
| Systemd | Linux service | Medium |
| Kubernetes | Cluster | High |

---

## Binary Installation

### Download

```bash
# Linux (amd64)
curl -L https://github.com/desmondkam/selo/releases/latest/download/selo-linux-amd64 -o selo
chmod +x selo
sudo mv selo /usr/local/bin/

# macOS (arm64)
curl -L https://github.com/desmondkam/selo/releases/latest/download/selo-darwin-arm64 -o selo
chmod +x selo
sudo mv selo /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/desmondkam/selo.git
cd selo
CGO_ENABLED=0 go build -ldflags="-s -w" -o selo ./cmd/selo/
sudo mv selo /usr/local/bin/
```

### Verify Installation

```bash
selo --help
selo status
```

---

## Docker

### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o selo ./cmd/selo/

FROM alpine:3.19

RUN apk add --no-cache git

WORKDIR /app
COPY --from=builder /app/selo .
COPY config/ ./config/

ENTRYPOINT ["./selo"]
```

### Build Image

```bash
docker build -t c1-forge:latest .
```

### Run Container

```bash
docker run -v /path/to/repo:/repo c1-forge:latest run "fix the bug" --repo /repo
```

### Docker Compose

```yaml
version: '3.8'

services:
  c1-forge:
    build: .
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./workspace:/workspace
      - ./config:/app/config
    environment:
      - SELO_CONFIG=/app/config/selo.yaml
    command: daemon
```

---

## Systemd Service

### Service File

```ini
[Unit]
Description=C1 Forge Daemon
After=network.target

[Service]
Type=simple
User=c1forge
Group=c1forge
WorkingDirectory=/opt/selo
ExecStart=/opt/selo/selo daemon --config /opt/selo/config/selo.yaml
Restart=always
RestartSec=10

# Security
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/selo/data

# Environment
Environment=SELO_CONFIG=/opt/selo/config/selo.yaml

[Install]
WantedBy=multi-user.target
```

### Install

```bash
# Create user
sudo useradd -r -s /bin/false c1forge

# Create directories
sudo mkdir -p /opt/selo/{config,data}
sudo chown -R c1forge:c1forge /opt/selo

# Copy binary and config
sudo cp selo /opt/selo/
sudo cp config/selo.yaml /opt/selo/config/

# Install service
sudo cp c1-forge.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable c1-forge
sudo systemctl start c1-forge
```

### Manage Service

```bash
# Status
sudo systemctl status c1-forge

# Logs
sudo journalctl -u selo -f

# Restart
sudo systemctl restart c1-forge

# Stop
sudo systemctl stop c1-forge
```

---

## CI/CD

### GitHub Actions

The project includes a CI pipeline in `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.21', '1.22', '1.23']
    
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
      
      - name: Download dependencies
        run: go mod download
      
      - name: Vet
        run: go vet ./...
      
      - name: Test
        run: go test ./... -count=1 -timeout 300s -race
      
      - name: Build
        run: go build -o selo ./cmd/selo/

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: --timeout=5m

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: securego/gosec@master
        with:
          args: ./...
```

### Release Pipeline

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Build binaries
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o selo-linux-amd64 ./cmd/selo/
          CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o selo-darwin-arm64 ./cmd/selo/
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            selo-linux-amd64
            selo-darwin-arm64
          generate_release_notes: true
```

---

## Production Configuration

### Recommended Settings

```yaml
forge:
  # Daemon
  poll_interval_sec: 10
  default_max_minutes: 60
  default_max_rounds: 5
  default_max_files: 20
  default_max_patch_lines: 500
  
  # Notifications
  notify: ntfy
  notify_config:
    mode: ntfy
    ntfy_url: "https://ntfy.sh/your-alerts-topic"
    timeout_seconds: 10
  
  # Security
  forbidden_claims:
    - PROFITABLE
    - LIVE_READY
    - MONEY_ENGINE
    - DEPLOY_NOW
    - SKIP_TESTS
  
  # Runner
  runner:
    mode: opencode
    command: /usr/local/bin/opencode
  
  # OpenCode
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
    serve_timeout: 60
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/data/repos/**/*.go"
      - "/data/repos/tests/**/*"
```

### Environment Variables

```bash
# Required
export SELO_CONFIG=/etc/selo/config.yaml

# Optional
export SELO_OPENCODE_BIN=/usr/local/bin/opencode
export SELO_DANGEROUSLY_SKIP_PERMISSIONS=false
```

### Directory Structure

```
/opt/selo/
├── selo              # Binary
├── config/
│   └── config.yaml        # Configuration
├── data/
│   ├── .selo/
│   │   ├── queue/
│   │   ├── runs/
│   │   ├── receipts/
│   │   └── worktrees/
│   └── repos/             # Repositories to process
└── logs/
    └── selo.log
```

---

## Monitoring

### Health Check

```bash
selo status --json
```

```json
{
  "daemon": "running",
  "pending": 0,
  "running": 1,
  "done": 42,
  "failed": 2
}
```

### Log Aggregation

```bash
# Systemd logs
journalctl -u selo -f --output=json

# Docker logs
docker logs -f c1-forge
```

### Metrics

C1 Forge exposes metrics via receipt analysis:

```bash
# Count verdicts
cat runs/*/receipt.json | jq -r '.verdict' | sort | uniq -c

# Average duration
cat runs/*/receipt.json | jq -r '.duration_sec' | awk '{sum+=$1; count++} END {print sum/count}'

# Success rate
cat runs/*/receipt.json | jq -r '.gatechain_action' | sort | uniq -c
```

---

## Backup

### What to Backup

| Path | Contents |
|------|----------|
| `.selo/queue/` | Pending/running tasks |
| `.selo/runs/` | Receipts and artifacts |
| `.selo/receipts/` | Historical receipts |
| `config/` | Configuration |

### Backup Script

```bash
#!/bin/bash
BACKUP_DIR="/backup/c1-forge/$(date +%Y%m%d)"
mkdir -p "$BACKUP_DIR"

# Backup queue and receipts
cp -r /data/selo/.selo/queue "$BACKUP_DIR/"
cp -r /data/selo/.selo/runs "$BACKUP_DIR/"
cp -r /data/selo/.selo/receipts "$BACKUP_DIR/"

# Backup config
cp -r /opt/selo/config "$BACKUP_DIR/"

# Cleanup old backups (keep 30 days)
find /backup/c1-forge -mtime +30 -exec rm -rf {} +
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `permission denied` | Wrong user/group | Check file permissions |
| `queue locked` | Stale lock file | Remove `.selo/queue/running/*.lock` |
| `worktree exists` | Previous crash | `git worktree prune` |
| `opencode not found` | Missing binary | Set `SELO_OPENCODE_BIN` |
| `model not available` | API key missing | Configure OpenCode API key |

### Debug Mode

```bash
SELO_DEBUG=1 selo daemon --config config/selo.yaml
```

### Log Files

```bash
# Application logs
tail -f /var/log/selo/selo.log

# Systemd logs
journalctl -u selo -f

# Docker logs
docker logs -f c1-forge
```

---

## Security

### Best Practices

1. **Run as non-root user**
   ```bash
   sudo useradd -r -s /bin/false c1forge
   ```

2. **Enable permissions check**
   ```yaml
   dangerously_skip_permissions: false
   ```

3. **Use permission allowlist**
   ```yaml
   permission_allowlist:
     - "/data/repos/**/*.go"
   ```

4. **Restrict network access**
   - Block outbound except API endpoints
   - Use firewall rules

5. **Monitor logs**
   - Set up alerts for failed tasks
   - Review receipts regularly

### Security Checklist

- [ ] Running as non-root user
- [ ] `dangerously_skip_permissions: false`
- [ ] Permission allowlist configured
- [ ] Forbidden claims configured
- [ ] Notifications enabled
- [ ] Logs monitored
- [ ] Backups configured
- [ ] Network restricted

---

## References

- [Architecture](../architecture/README.md) — System design
- [Configuration](../configuration/README.md) — Config options
- [Development](../development/README.md) — Contributing guide
