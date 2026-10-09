# Changelog

All notable changes to Selo are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is semver-ish
for an early project — breaking config or receipt-schema changes bump the minor.

## [Unreleased]

### Security
- **The agent no longer inherits the signing key.** `selo run` passed the full
  host environment to the agent subprocess, so `SELO_SIGNING_KEY` reached the
  very process the receipt attests to. The runner now strips `SELO_SIGNING_KEY`
  and `SELO_ALLOW_EPHEMERAL_KEY` from the agent's environment. A same-user agent
  can still read `~/.selo/signing-key` from disk, so non-repudiation requires a
  separate trust domain — see SECURITY.md "Threat model". Regression test:
  `TestAgentEnvDropsSigningKey`. The phased plan to move the key fully out of
  the agent's trust domain is ADR-008.
- **Receipts bind the audited change, the policy, and the build.** Added
  `diff_hash` (sha256 of the full diff), `policy_hash` (sha256 of the effective
  forbidden claims/files and limits) and `selo_version`, all covered by the
  signature. A `SUCCESS` receipt can no longer be replayed against a different
  diff or confused with one produced under an empty policy.

### Added
- **Pluggable signer (ADR-008 Phase 2).** `SELO_SIGNER` selects where the signing
  key lives and who signs: `file` (default, unchanged), `keychain` (the seed is
  stored in the OS keychain — macOS Keychain / Linux Secret Service via
  `secret-tool`, no CGO — with `selo keys generate --keychain`, `selo keys store
  --keychain [--delete-file]`, `selo keys pub --keychain`), or `command` (an
  external program signs; Selo never holds the key, via `SELO_SIGNER_COMMAND` +
  `SELO_SIGNER_PUBKEY`). `selo run` gains `--signer`. Only `command` with a
  non-extractable key moves the signer out of the audited agent's trust domain.
- Receipts record a signed `key_source` (`env | file | keychain | command |
  ephemeral`); `selo verify` reports it.
- **in-toto / DSSE receipt export (ADR-001).** `selo receipt export <id> --format
  in-toto` re-signs a receipt as an in-toto Statement v1 wrapped in a DSSE
  envelope, so third parties can verify it with standard tooling (cosign,
  slsa-verifier) without installing Selo. The signature covers the DSSE PAE over
  the exact statement bytes — not a re-serialization — so a verifier in any
  language can check it. The native `receipt.json` is unchanged. `selo verify`
  now detects and verifies an envelope, reporting `Format: in-toto/DSSE`, with
  the same `--pubkey` pinning.
- `selo selftest` — plants a known secret, a forbidden-file edit, a forbidden
  claim, and a removed test, then asserts every control rejects it; also asserts
  signing fails closed with no key. Runs in CI so a detector that regresses to
  "always clean" cannot ship silently.

### Fixed
- **The README CI example no longer disables the gate.** It showed
  `selo run "NOOP" || true`, teaching users to swallow the exit code the check
  exists to raise.
- **`WriteReceipt` no longer swallows filesystem errors.** It discarded every
  `MkdirAll`/`WriteFile`/`Marshal` error and always returned nil, so a full disk
  or permission problem left the caller believing the receipt had been written.
- **Unknown task keys are warned about instead of silently ignored.** A
  misspelled `max_minutes:` used to leave the default in force with no signal.
- **CI now runs on macOS as well as Linux.** Release binaries are built for
  darwin, but only `ubuntu-latest` had ever been tested.

## [0.5.1] — 2026-10-09

### Changed
- **Signing keys auto-load.** `selo run` and the receipt signer now fall back to
  `~/.selo/signing-key` (the file `selo keys generate` writes) when
  `SELO_SIGNING_KEY` is unset, so you no longer have to export the seed in your
  shell profile. Still **fail-closed**: with neither the env var nor the key file
  present, signing is refused. A present-but-malformed key file is a hard error,
  not a silent fallback to an ephemeral key. `selo keys generate` now reports
  that the key loads automatically instead of telling you to export it.

### Fixed
- `selo run`'s unsigned-receipt error message now names the auto-loaded key path
  instead of only `SELO_SIGNING_KEY`.

## [0.5.0] — 2026-10-08

### Security
- **Receipt forgery closed (ADR-004).** `VerifyReceipt` previously read the
  public key from the receipt itself, so a tampered receipt could be re-signed
  with a fresh key and still verify as `VALID`. Signing is now **fail-closed**:
  without `SELO_SIGNING_KEY` Selo refuses to sign (and `selo run` exits
  non-zero) unless you opt into a per-process key with `selo run --dev` or
  `SELO_ALLOW_EPHEMERAL_KEY=1`. Every receipt now records `key_mode`
  (`persistent` | `ephemeral`), signed into the receipt so it cannot be forged.
- **`selo verify` now requires a pinned key to prove who signed.** Without
  `--pubkey`, `selo verify` reports internal self-consistency only and prints
  `Provenance: UNPINNED_*` with an explicit warning — it no longer prints a bare
  `VALID` that could be mistaken for provenance. Supply `--pubkey
  <fingerprint|file>` to confirm the signer; a fingerprint mismatch fails the
  check. `selo receipt show` accepts the same `--pubkey` flag.

### Added
- `selo run --dev` — accept an ephemeral per-process signing key when
  `SELO_SIGNING_KEY` is unset (dev only; receipts are not attributable across
  runs).
- `selo verify --pubkey <file|fingerprint>` and `selo receipt show --pubkey
  <file|fingerprint>` — pin the trusted signer and prove provenance.
- `receipt.PublicKeyFingerprint` and `LoadSigningKeyWithMode` (report the key
  mode) in the signing library.

### Changed
- **BREAKING:** default signing behavior. `SELO_SIGNING_KEY` is now required for
  production receipts; missing key fails closed instead of silently using an
  ephemeral key. Migration: run `selo keys generate` once and `export
  SELO_SIGNING_KEY=$(cat ~/.selo/signing-key)` (or pass `selo run --dev` for
  throwaway/dev runs).
- Receipt schema adds `key_mode`. Older receipts without the field verify as
  `UNPINNED_UNKNOWN` and are still internally consistent.

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
