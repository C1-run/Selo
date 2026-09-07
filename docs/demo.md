# C1 Forge Demo

A step-by-step walkthrough of using C1 Forge locally with a mock runner. This demo shows the complete flow from project initialization to safety verification, without requiring a real AI model.

## Prerequisites

```bash
# 1. Clone or enter the C1-forge repository
cd /path/to/C1-forge

# 2. Build the binary
go build -o selo ./cmd/selo/

# 3. (Optional) Install to PATH
sudo cp selo /usr/local/bin/
```

## Step 1: Initialize a Test Project

Create a simple Go project:

```bash
mkdir demo-project && cd demo-project
git init
cat > main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
EOF

# Initialize C1 Forge workspace
selo init
```

This creates the `.selo/` directory and a default `selo.local.yaml` config.

## Step 2: Run a Task

Execute a task that simulates an AI agent making a risky change:

```bash
selo run "Add a function that returns an error but ignore it" \
  --repo . \
  --commands "go test ./..." \
  --max-minutes 5
```

**What happens:**
- A git worktree is created in an isolated directory
- The mock agent applies changes to the worktree
- Tests are run against the modified code
- The safety pipeline verifies the changes

## Step 3: Observe the Output

The command output shows the safety pipeline in action:

```
[INFO] Creating worktree at /tmp/c1forge-run-12345
[INFO] Running task: Add a function that returns an error but ignore it
[INFO] Mock agent applying changes...
[INFO] Running test command: go test ./...
[INFO] Test passed
[INFO] Running safety pipeline...
[WARN] Secret scan detected potential API key in main.go:12
[WARN] Test integrity check: no tests removed
[INFO] GateChain decision: NEEDS_HUMAN
[INFO] Receipt written to runs/run-<taskID>/receipt.json
```

If the mock agent introduces a hardcoded secret, the **Secret scan** triggers and GateChain decides `NEEDS_HUMAN` (human review required).

## Step 4: View the Safety Receipt

Check the status of all tasks:

```bash
selo status
```

Output:

```
Queue: empty
Recent runs:
  run-123abc  goal="Add a function..."  verdict=NEEDS_HUMAN  time=2025-03-15T10:30:00Z
```

View the specific receipt:

```bash
cat runs/run-123abc/receipt.json
```

```json
{
  "task_id": "run-123abc",
  "goal": "Add a function that returns an error but ignore it",
  "verdict": "NEEDS_HUMAN",
  "safety_checks": {
    "secret_scan": {
      "passed": false,
      "hits": [
        {
          "file": "main.go",
          "line": 12,
          "rule": "generic-api-key",
          "description": "Potential API key found"
        }
      ]
    },
    "test_integrity": {
      "passed": true,
      "tests_removed": []
    },
    "pinocchio": {
      "passed": false,
      "reason": "Agent claimed to modify only main.go but also added a new file"
    }
  },
  "recommendation": "Human review required due to secret and inconsistency"
}
```

A human-readable `receipt.md` is also generated.

## Step 5: Inspect the Worktree

The main branch remains unmodified — all changes happen in the isolated worktree:

```bash
cd /tmp/c1forge-run-12345
git diff
```

You'll see the mock agent's changes, including any hardcoded secrets it introduced.

## What C1 Forge Did

1. **Isolation**: Changes were made in a temporary worktree, not your main branch
2. **Execution**: Ran the agent (mock) and executed test commands
3. **Safety Checks**: Secret scanning, test integrity, Pinocchio consistency verification
4. **Decision**: GateChain综合判断, issued `NEEDS_HUMAN` to block unsafe auto-merge
5. **Audit**: Complete receipt (JSON + Markdown) for traceability

## Using Real AI

To use a real AI model instead of the mock runner:

1. Set `runner.mode: "opencode"` in your config
2. Configure the OpenCode binary path and model
3. Run the same `selo run` command

```yaml
# selo.local.yaml
forge:
  runner:
    mode: opencode
    command: opencode
  opencode:
    model: anthropic/claude-sonnet-4-5
    agent: default
```

The safety pipeline works the same way — it wraps any agent and produces verifiable receipts.
