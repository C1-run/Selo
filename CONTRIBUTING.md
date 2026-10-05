# Contributing to Selo

Selo is a v0.1 alpha. Contributions are welcome, with one standing priority:
**correctness and honesty of claims come before feature count.** A smaller
project that does exactly what it says is more useful to us than a larger one
that overstates itself. If a change would make a README or doc sentence more
confident than the code supports, that is a reason to change the sentence, not
the code.

For the development guide with deeper notes on structure and testing, see
`docs/development/README.md`.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Build and test |
| git | any recent version | Worktrees, anchoring receipts |
| Node.js | only if working on `.opencode/` | OpenCode plugin |

The module declares `go 1.21` (see `go.mod`) and CI pins `1.21`. The OpenCode
plugin under `.opencode/` is TypeScript and needs Node.js; nothing else in the
repository does.

## Build and test

```bash
go build ./...
go vet ./...
go test ./internal/... -count=1
go test ./cmd/selo/ -count=1
```

Notes:

- The `cmd/selo` suite is slow — a minute or more — because it drives real
  worktrees and processes. Budget for it rather than assuming it is hung.
- That suite resolves the repository root from the `SELO_HOME` environment
  variable. When you run it outside CI, set it yourself, e.g.
  `SELO_HOME=$(pwd) go test ./cmd/selo/ -count=1`.
- `go test ./... -count=1` runs everything if you want a single command.

## Go style

- Run `go fmt ./...`. The tree is **not** currently gofmt-clean everywhere, so
  only format the files you touch. Do not reformat unrelated files in the same
  change; it buries the real diff.
- Use **table-driven tests** with subtests (`t.Run`) for multiple cases. Use
  `t.TempDir()` for scratch directories.
- Run `go test -race` where concurrency is involved.
- Follow the error-wrapping style already in the tree: `fmt.Errorf("doing
  thing: %w", err)`.

## Trying the pipeline without an agent

You do not need a coding agent to exercise the pipeline end to end:

```bash
go build -o selo ./cmd/selo/
./selo init
./selo run "NOOP"
```

The default config sets `runner.mode: mock`. In `mock` mode Selo echoes a
placeholder instead of running an agent — it exercises worktree creation, the
safety checks, and receipt generation, but does not produce real changes. It is
for testing Selo, not for testing an agent.

To drive a real agent, set `runner.mode: real` and give `runner.command` an
agent CLI. The command receives `{{task_file}}` and `{{worktree}}` substituted
into its arguments, and `C1_TASK`, `C1_WORKDIR`, and `C1_MAX_MINUTES` in its
environment. `runner.mode: opencode` instead drives the bundled native OpenCode
adapter.

## Pull request expectations

- **Keep changes focused.** One purpose per PR.
- **Add a test for a behavior change.** If a change cannot be tested, say why
  in the description.
- **State clearly if you changed anything safety-relevant.** This includes the
  safety checks, the receipt format, containment, signing, and configuration
  validation.
- **Extra scrutiny applies to safety checks and the receipt format.** A change
  to what Selo considers safe, or to what a receipt attests, affects every past
  and future receipt. Expect review to be slower and more conservative there,
  and expect us to ask whether the change weakens a check to make something
  pass.

If a check needs to be relaxed, the burden is on showing the check was wrong,
not on showing the relaxation is convenient.
