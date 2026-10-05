# Changelog

All notable changes to Selo are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is semver-ish
for an early project — breaking config or receipt-schema changes bump the minor.

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
