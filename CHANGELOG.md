# Changelog

All notable changes to Selo are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is semver-ish
for an early project — breaking config or receipt-schema changes bump the minor.

## [0.4.1] — 2026-10-08

### Fixed
- **Secret scan detected nothing outside GNU grep.** The scan shelled out to
  `grep -E` with `\b`, `\s` and `(?:...)`, none of which is POSIX ERE. BSD and
  macOS grep reject or silently fail to match them, so the scan returned zero
  hits and the control failed open with no error. It now runs in-process as Go
  regexps, and no longer depends on a `grep` binary at all. (The v0.4.0 note
  about AWS/GitHub/Slack/Google/OpenAI coverage was therefore only true of the
  full-worktree scan; see below.)
- **The run path never scanned for real tokens.** `RunSecretScanDiffScoped`
  (used by `selo run` and the MCP server) carried 2 of the 7 patterns, so AWS,
  GitHub, Slack, Google and OpenAI tokens were undetectable during an actual
  run. Both paths now share one pattern set.
- **`Result.Output` contained stderr as well as stdout.** `cell` pointed
  `cmd.Stdout` and `cmd.Stderr` at the same buffer, so tool noise — a docker
  image pull, on a cold runner — landed in the result. `Output` is stdout only;
  stderr is on `Result.Stderr`.
- **CI never ran.** The gofmt gate wrote `out=$$(go env GOROOT)`, which is not
  valid shell (`$$` is the PID, leaving a bare `(`), so the step died with a
  syntax error. Go is now pinned via `go-version-file`.
- **Tests only passed on one machine.** The real-run tests assumed
  `/tmp/selo-fixture` already existed; on a clean checkout all four
  `TestC1Loop*` tests failed with `FAILED_INTERNAL_ERROR`. The fixture is now
  created by the suite.

### Changed
- `gofmt` drift across 26 tracked files corrected (whitespace and import
  ordering only), so the format gate can pass.

## [0.4.0] — 2026-10-05

### Added
- `selo mcp serve` — exposes the real checks over the Model Context Protocol
  (stdio) so MCP clients audit with the actual engine.
- `selo keys generate` / `selo keys pub` — Ed25519 signing-key management
  (mode-0600 seed file, export line, public key) so signatures are
  attributable across runs by default.
- Evasion-resistant forbidden-claims matching: case-insensitive, folds
  leetspeak (`pr0duct1on_ready`) and zero-width characters, over changed files.
- Secret scan now covers AWS, GitHub, Slack, Google, and OpenAI-style tokens.
- Release workflow builds `selo-<os>-<arch>` binaries per tag.
- ZCode skill (`.agents/skills/selo/`) and dogfood runner config.

### Changed
- `selo run` exit codes now follow the spec: 1 on safety failure, 2 on timeout.
- CI runs on `main`, `public-v01`, and manual dispatch.

### Fixed
- `--forbidden-files` / task-file `forbidden_files`, `allowed_files` and
  `forbidden_claims` were parsed with surrounding quotes intact and therefore
  never matched the paths audited against them — a forbidden file sailed
  through as `SUCCESS`.

## [0.3.0] — 2026-10-05

### Added
- `selo receipt list` and `selo receipt show <id-or-path>` — decision cards
  (text / markdown / github / json) that always include the receipt's
  integrity state; `--format github` targets CI job summaries.
- GitHub Actions "Use in CI" recipe via `$GITHUB_STEP_SUMMARY`.

### Changed
- c1.run-era codenames removed from user-facing output (`✓ Selo`), worktree
  branch prefix (`selo/…`), audit-log source, and the soak output directory.

## [0.2.0] — 2026-10-05

### Added
- `opencode.permission_allowlist` enforcement: a post-run scope check (repo-
  relative globs) that rejects tasks touching files outside the allowlist.
- `validateConfig`: refuses `containment.strategy: docker|local` (only the
  git-worktree strategy exists) and unrecognized `runner.mode` values — both
  previously failed silently into weaker or mock behavior.
- Config loader migrated to yaml.v3 with warnings on keys Selo does not read.
- `selo verify <receipt.json>` — content-hash + Ed25519 signature + optional
  git-anchor verification.
- ZCode/OpenCode plugin manifest fixes and legacy codename file removal.

### Fixed
- `selo init` now creates the runtime directories the queue manager actually
  uses, instead of a parallel `.selo/` tree nothing read.
- Default `forbidden_claims` are generic overclaims; the retired
  capital-markets tokens are no longer shipped as defaults.

## [0.1.0] — 2026-09-08

- First public snapshot of the CLI: containment, safety checks, signed
  receipts, queue/daemon, smoke/soak harness.
