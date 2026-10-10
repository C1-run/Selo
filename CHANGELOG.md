# Changelog

All notable changes to Selo are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is semver-ish
for an early project — breaking config or receipt-schema changes bump the minor.

## [Unreleased]

## [0.6.0] — 2026-10-10

### Security
- **The macOS binaries we had been shipping could not start.** Every darwin
  binary published in v0.4.1, v0.5.0 and v0.5.1 was missing the Mach-O `LC_UUID`
  load command, which newer macOS dyld refuses to load (`dyld: missing LC_UUID
  load command` → `signal: abort trap`). The release workflow pinned
  `go-version-file: go.mod`, and `go.mod` says `go 1.21`; Go toolchains before
  1.24 omit that load command on darwin, and the toolchain alone decides it —
  `CGO_ENABLED` and `-ldflags "-s -w"` make no difference. `install.sh` could not
  notice, because it verifies the bytes (checksum, cosign signature, SLSA
  provenance) and every one of those checks is satisfied perfectly by a binary the
  loader rejects. Fixed by pinning `go-version: '1.24.x'` in both workflows and
  adding a fail-closed release gate, `scripts/check-macho-uuid`, which parses the
  Mach-O load commands and fails the release *before* anything is signed or
  published if `LC_UUID` is absent. CI runs the same check on the macOS leg, so a
  toolchain regression fails on the commit that introduces it. Full post-mortem:
  `docs/releases/v0.6.0.md`. **Requires Go >= 1.24 to build a working darwin
  binary.**
- **A malformed glob in a scope contract silently disabled that boundary.**
  `VerifyScopeContract` evaluated each `allowed_paths` / `forbidden_paths` entry
  with `filepath.Match` and dropped the error (`if err == nil && matched`), so a
  pattern that does not compile matched nothing and the run still reported clean —
  an operator who typed `[a-z` instead of `[a-z]*` turned the control off with no
  signal. Patterns are now validated at config load and at the top of the contract
  check; an invalid one refuses the run and names the field and the pattern.
  Validation probes each pattern against its own bytes, because `filepath.Match`
  returns `ErrBadPattern` only for the chunks it walks and stops at the first
  chunk that fails to match, so probing with a short string misses a malformed
  chunk further along (`Match("abc*[", "x")` returns no error).
- **The forbidden-claims scan failed open.** `RunForbiddenClaimsScan` shelled out
  to `grep` and discarded the error, so on a host without `grep` — or with a
  `grep` that rejected the arguments — every term was silently skipped and the
  control reported no forbidden claims. It was also weaker than its own
  diff-scoped sibling: plain substring matching (missing `pr0duct1on_ready`) over
  7 file extensions (missing `.yml` and `.py`), so whether a claim was a
  violation depended only on whether a diff happened to be available. Both paths
  now share one in-process matcher.
- **Receipt IDs collided on a coarse clock.** `GenerateReceiptID` returned
  `c1f-<UnixNano>`, but macOS reports `time.Now()` at microsecond granularity —
  measured 1000 ns on darwin/amd64 — so calls in the same tick produced the same
  id: 3217 duplicates out of 5000 back-to-back calls. The id *is* the receipt's
  file name (`receipts/c1f-<id>.json`), so a duplicate overwrote an earlier
  receipt. `GenerateReceiptID` now holds a mutex and steps past the previous id.
  This had been hidden behind the dyld abort: `internal/receipt`'s test binary
  never ran on macOS at all.
- **`SECURITY.md` now states the install-time boundary explicitly.** New section
  *What `install.sh` verifies — and what it does not*: it proves supply-chain
  integrity and provenance, while loadability is enforced fail-closed at build
  time by the release job. Together they are the delivery trust chain; neither
  alone is sufficient.
- **The documented cosign identity used the wrong case.** SECURITY.md told users
  to pin `C1-run/selo`. cosign's identity regexp is case-sensitive, so anyone
  following the docs would have rejected our own signature — the same bug
  `install.sh` itself carried. Now `C1-run/Selo`.
- **Releases are signed, and the installer now verifies them (ADR-002,
  ADR-003).** `install.sh` previously downloaded a binary and executed it with
  zero verification — a tool that sells verifiability distributed itself
  unverifiably. Releases now publish a deterministic `checksums.txt`, sign it
  with cosign keyless (Fulcio + GitHub OIDC, so no long-lived signing key), and
  attach SLSA Build L3 provenance. `install.sh` checks the binary's SHA-256
  against `checksums.txt`, then verifies `checksums.txt` with `cosign
  verify-blob`, pinning **both** the workflow identity and the OIDC issuer. Any
  failure — including `cosign` being absent — aborts the install rather than
  degrading to an unverified one. See SECURITY.md "Verifying a release".
- **The L3 wording is now backed by capability, but stays conditional.** With
  ADR-005 (trusted timestamp) and ADR-006 (transparency log) implemented, Selo
  can support a "non-repudiable" claim — but only when the signer sits outside
  the audited agent's trust domain (ADR-008 `command` backend with a
  non-extractable key). Under the default `file` backend the key is readable by
  the same user, so the honest ceiling remains L1/L2. See SECURITY.md "Threat
  model" and ADR-007.
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
- **`scripts/check-macho-uuid`** — a small Go command that fails closed if a
  Mach-O binary lacks the `LC_UUID` load command, used as the release gate
  described under Security. It parses the container with `debug/macho` (thin or
  fat, 32- or 64-bit, either byte order) and needs no macOS tooling, so it runs
  on the Linux release runner. It exits `0` only when every Mach-O input carries
  the load command, `1` if any does not or any input cannot be read, and `2` if no
  Mach-O was checked at all — so a glob that matches nothing fails instead of
  passing vacuously. Non-Mach-O inputs (the Linux ELF binaries) are reported and
  skipped.
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
- **RFC3161 trusted timestamps (ADR-005).** `selo run --tsa <url>` obtains a
  timestamp over the receipt's canonical bytes (message imprint = `receipt_hash`)
  and records it as `timestamp`. Because a timestamp attests to when a signature
  existed, it is produced after signing and excluded from the canonical JSON.
  `selo receipt timestamp <id> --tsa <url>` retrofits one onto an existing
  receipt — refusing if its signature or content hash does not verify. `selo
  verify --tsa-ca <pem>` anchors the token to a trusted TSA (chain + timestamping
  EKU, evaluated at the timestamped instant); without `--tsa-ca` the token is
  reported `UNVERIFIED`, and `--require-tsa` makes anything short of OK fatal.
  Fail-closed: an unreachable TSA fails the run unless `--tsa-soft` records the
  absence explicitly. Uses `github.com/digitorus/timestamp` (pinned; two pure-Go
  modules, no CGO) — not `sigstore-go`, so the Go floor stays at 1.21.
- **Sigstore Rekor transparency log (ADR-006).** `selo run --rekor <url>` logs
  the receipt hash in a Rekor log as a DSSE entry signed by the receipt's key,
  and records the log record as `transparency` (`log_index`, `integrated_time`,
  `body`, inclusion proof, signed entry timestamp). `selo receipt log <id>
  --rekor <url>` logs an existing receipt. Only the receipt hash is published:
  Rekor stores the envelope/payload *hashes*, so no task name, file path, or
  receipt content leaves the machine. `selo verify --rekor-pubkey <pem>`
  recomputes the RFC 6962 inclusion proof to the checkpoint root and verifies the
  checkpoint's signature against the log key; `--require-rekor` gates on it.
  Implemented against Rekor's REST API with the standard library — no
  `sigstore-go`, so no Go 1.23+ requirement and no heavy dependency.
- `selo selftest` — plants a known secret, a forbidden-file edit, a forbidden
  claim, and a removed test, then asserts every control rejects it; also asserts
  signing fails closed with no key. Runs in CI so a detector that regresses to
  "always clean" cannot ship silently.
- **Signed releases and build provenance (ADR-002, ADR-003).** Every release now
  ships three verification artifacts alongside the binaries: `checksums.txt`
  (sorted, deterministic SHA-256 of each binary), `checksums.txt.sigstore.json`
  (cosign keyless signature over it), and `multiple.intoto.jsonl` (SLSA Build L3
  provenance). The same `checksums.txt` serves both purposes — it is the blob
  cosign signs and, base64-encoded, the `base64-subjects` input the SLSA
  generator consumes. Verify a download independently with:
  ```
  cosign verify-blob --bundle checksums.txt.sigstore.json \
    --certificate-identity-regexp \
      '^https://github\.com/C1-run/selo/\.github/workflows/release\.yml@refs/tags/v.*$' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    checksums.txt
  slsa-verifier verify-artifact <binary> \
    --provenance-path multiple.intoto.jsonl --source-uri github.com/C1-run/selo
  ```
- **`scripts/test-install.sh`** — hermetic tests for the installer's
  verification control flow. Serves fake artifacts over `file://` URLs and
  substitutes a `cosign` shim, so it needs no network and no credentials; covers
  the happy path, a tampered checksum, a tampered binary, missing `cosign`,
  a rejected signature, the `SELO_SKIP_VERIFY` opt-out, and missing
  `checksums.txt`/bundle. Wired into CI. It pins the control flow only — real
  Fulcio/Rekor verification needs a genuine release.
- **`selo verify --require-keysource` enforces signer isolation (ADR-008).** The
  signing side could already delegate to an external program (`command` backend,
  Phase 2), but `verify` accepted any `key_source` — so isolation could not be
  enforced, only deployed. Now `selo verify --require-keysource=command` fails the
  receipt unless its signing key was delegated to an external signer (the only
  source outside the agent's trust domain). The gate is enforced only together
  with `--pubkey`: `key_source` is self-asserted, so a gate without a pinned key
  would accept a receipt that merely claims "command" — that combination is now
  refused rather than reported as passing. The result reports `key_source_state`
  (`OK`/`UNVERIFIED`/`FAILED`/`ABSENT`/`SKIPPED`); without the flag the source is
  shown but not required. A reference external signer ships at
  `scripts/selo-signer/main.go`, meant to run as a separate user / container / HSM /
  CI job so Selo never sees the private key.

### Fixed
- **`install.sh` no longer invokes `sudo` unnecessarily.** It only escalates
  when the target directory is not writable, and creates the directory if it is
  missing.
- **`install.sh` version lookup no longer fails on a successful fetch.** It used
  `grep '"tag_name"' | head -1`; `head` closing the pipe early can raise SIGPIPE
  in `grep`, which `set -o pipefail` then turned into a spurious install
  failure. Now uses `grep -m1`.
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
