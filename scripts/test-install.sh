#!/bin/bash
# Hermetic tests for install.sh's verification control flow.
#
# These do NOT exercise real Sigstore verification — that needs Fulcio, Rekor
# and a genuine release (see docs/decisions/ADR-002). What they pin down is the
# control flow: that verification is on by default, that every failure mode
# aborts the install, that the pinned identity/issuer flags are actually passed
# to cosign, and that SELO_SKIP_VERIFY is a loud opt-out.
#
# Artifacts are served over file:// URLs and cosign is replaced by a shim, so
# the suite needs no network and no credentials.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SH="$REPO_ROOT/install.sh"

PASS=0
FAIL=0
ok()  { echo "  PASS  $1"; PASS=$((PASS + 1)); }
bad() { echo "  FAIL  $1"; FAIL=$((FAIL + 1)); }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --- platform ---------------------------------------------------------------
case "$(uname -s)" in
    Darwin*) OS=darwin ;;
    Linux*)  OS=linux ;;
    *)       echo "unsupported test OS"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *)             echo "unsupported test arch"; exit 1 ;;
esac
ASSET="selo-$OS-$ARCH"

DIST="$WORK/dist"

sha() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# Build a well-formed, fully-verifiable release layout.
make_dist() {
    rm -rf "$DIST"
    mkdir -p "$DIST"
    printf '#!/bin/sh\necho "selo fake"\n' > "$DIST/$ASSET"
    chmod +x "$DIST/$ASSET"
    printf '%s  %s\n' "$(sha "$DIST/$ASSET")" "$ASSET" > "$DIST/checksums.txt"
    printf '{"fake":"bundle"}\n' > "$DIST/checksums.txt.sigstore.json"
}

# Fake cosign: records its argv and exits with a controllable status.
SHIM="$WORK/shim"
mkdir -p "$SHIM"
cat > "$SHIM/cosign" <<'SHIMEOF'
#!/bin/bash
printf '%s\n' "$*" >> "${COSIGN_SHIM_LOG:-/dev/null}"
exit "${COSIGN_SHIM_EXIT:-0}"
SHIMEOF
chmod +x "$SHIM/cosign"

# A PATH that has everything install.sh needs except cosign.
BARE_PATH="/usr/bin:/bin"

LAST_OUT=""
run_install() {
    local dest="$1"; shift
    local out="$WORK/out.txt"
    local rc=0
    env INSTALL_DIR="$dest" \
        SELO_BASE_URL="file://$DIST" \
        SELO_VERSION="v9.9.9" \
        COSIGN_SHIM_LOG="$WORK/cosign.log" \
        "$@" \
        bash "$INSTALL_SH" >"$out" 2>&1 || rc=$?
    LAST_OUT="$(cat "$out")"
    return $rc
}

new_dest() { local d="$WORK/$1"; mkdir -p "$d"; echo "$d"; }

echo "install.sh verification tests (asset: $ASSET)"
echo ""

# --- 1. happy path ----------------------------------------------------------
echo "1. happy path"
make_dist
: > "$WORK/cosign.log"
dest="$(new_dest dest1)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    if [ -x "$dest/selo" ]; then ok "installs a verified binary"; else bad "binary missing after install"; fi
else
    bad "install failed unexpectedly: $LAST_OUT"
fi
if grep -q 'verify-blob' "$WORK/cosign.log" &&
   grep -q -- '--bundle' "$WORK/cosign.log" &&
   grep -q -- '--certificate-identity-regexp' "$WORK/cosign.log" &&
   grep -q -- '--certificate-oidc-issuer' "$WORK/cosign.log"; then
    ok "cosign called with bundle + pinned identity + pinned issuer"
else
    bad "cosign flags wrong: $(cat "$WORK/cosign.log")"
fi
if grep -q 'token.actions.githubusercontent.com' "$WORK/cosign.log"; then
    ok "issuer pinned to GitHub Actions OIDC"
else
    bad "issuer not pinned to the expected value"
fi

# --- 2. tampered checksum ---------------------------------------------------
echo "2. tampered checksum"
make_dist
printf '%s  %s\n' \
    "0000000000000000000000000000000000000000000000000000000000000000" "$ASSET" \
    > "$DIST/checksums.txt"
dest="$(new_dest dest2)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    bad "tampered checksum was accepted"
else
    if [ ! -e "$dest/selo" ]; then ok "rejected, nothing installed"; else bad "installed despite mismatch"; fi
fi

# --- 3. tampered binary -----------------------------------------------------
echo "3. tampered binary (checksums.txt left honest)"
make_dist
printf '#!/bin/sh\necho EVIL\n' > "$DIST/$ASSET"
dest="$(new_dest dest3)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    bad "tampered binary was accepted"
else
    if [ ! -e "$dest/selo" ]; then ok "rejected, nothing installed"; else bad "installed tampered binary"; fi
fi

# --- 4. cosign missing ------------------------------------------------------
echo "4. cosign missing"
make_dist
dest="$(new_dest dest4)"
if run_install "$dest" PATH="$BARE_PATH"; then
    bad "install succeeded with no cosign on PATH"
else
    if echo "$LAST_OUT" | grep -qi 'cosign'; then
        ok "fails closed with an actionable message"
    else
        bad "error does not mention cosign: $LAST_OUT"
    fi
fi

# --- 5. cosign rejects the signature ---------------------------------------
echo "5. cosign verification fails"
make_dist
dest="$(new_dest dest5)"
if run_install "$dest" PATH="$SHIM:$PATH" COSIGN_SHIM_EXIT=1; then
    bad "invalid signature was accepted"
else
    if [ ! -e "$dest/selo" ]; then ok "aborts, nothing installed"; else bad "installed despite bad signature"; fi
fi

# --- 6. SELO_SKIP_VERIFY ----------------------------------------------------
echo "6. SELO_SKIP_VERIFY=1 escape hatch"
make_dist
# Deliberately corrupt the checksum to prove verification really was skipped.
printf '%s  %s\n' "deadbeef" "$ASSET" > "$DIST/checksums.txt"
dest="$(new_dest dest6)"
if run_install "$dest" PATH="$BARE_PATH" SELO_SKIP_VERIFY=1; then
    if [ -x "$dest/selo" ]; then ok "installs without verification"; else bad "binary missing"; fi
    if echo "$LAST_OUT" | grep -q 'WARNING'; then ok "prints a loud warning"; else bad "no warning printed"; fi
else
    bad "SKIP_VERIFY install failed: $LAST_OUT"
fi

# --- 7. missing checksums.txt ----------------------------------------------
echo "7. missing checksums.txt"
make_dist
rm -f "$DIST/checksums.txt"
dest="$(new_dest dest7)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    bad "missing checksums.txt was accepted"
else
    if [ ! -e "$dest/selo" ]; then ok "fails closed"; else bad "installed without checksums"; fi
fi

# --- 8. missing sigstore bundle --------------------------------------------
echo "8. missing sigstore bundle"
make_dist
rm -f "$DIST/checksums.txt.sigstore.json"
dest="$(new_dest dest8)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    bad "missing bundle was accepted"
else
    if [ ! -e "$dest/selo" ]; then ok "fails closed"; else bad "installed without bundle"; fi
fi

# --- 9. asset absent from checksums.txt ------------------------------------
echo "9. asset not listed in checksums.txt"
make_dist
printf '%s  %s\n' "$(sha "$DIST/$ASSET")" "selo-somewhere-else" > "$DIST/checksums.txt"
dest="$(new_dest dest9)"
if run_install "$dest" PATH="$SHIM:$PATH"; then
    bad "unlisted asset was accepted"
else
    ok "rejected"
fi

# --- 10. pinned identity regexp matches the canonical repo ------------------
echo "10. pinned identity regexp"
make_dist
: > "$WORK/cosign.log"
dest="$(new_dest dest10)"
run_install "$dest" PATH="$SHIM:$PATH" >/dev/null 2>&1 || true
REGEX="$(sed -n 's/.*--certificate-identity-regexp \([^ ]*\).*/\1/p' "$WORK/cosign.log" | head -1)"
if [ -z "$REGEX" ]; then
    bad "no identity regexp was passed to cosign"
elif printf '%s' "$REGEX" | grep -q 'C1-run/Selo'; then
    # The canonical repo case must survive: cosign's regexp is case-sensitive,
    # so a lowercase default would reject a legitimate release signature.
    ok "identity regexp carries the canonical repo case (C1-run/Selo)"
else
    bad "identity regexp does not carry the canonical case: $REGEX"
fi
if command -v python3 >/dev/null 2>&1 && [ -n "$REGEX" ]; then
    if python3 - "$REGEX" <<'PY'
import re, sys
rx = re.compile(sys.argv[1])
good = "https://github.com/C1-run/Selo/.github/workflows/release.yml@refs/tags/v0.6.0"
foreign = "https://github.com/evil/Selo/.github/workflows/release.yml@refs/tags/v0.6.0"
branch = "https://github.com/C1-run/Selo/.github/workflows/release.yml@refs/heads/main"
sys.exit(0 if (rx.search(good) and not rx.search(foreign) and not rx.search(branch)) else 1)
PY
    then
        ok "identity regexp matches our tag identity and rejects a foreign repo / branch ref"
    else
        bad "identity regexp does not pin correctly: $REGEX"
    fi
fi

# --- 11. SECURITY.md documents the same canonical identity ------------------
# The lowercase repo spelling was wrong in install.sh AND in the docs. The docs
# are what a user copies by hand, so they get the same guard.
echo "11. SECURITY.md identity example"
SECURITY_MD="$REPO_ROOT/SECURITY.md"
if [ ! -f "$SECURITY_MD" ]; then
    bad "SECURITY.md not found at $SECURITY_MD"
elif grep -q 'C1-run/selo' "$SECURITY_MD"; then
    bad "SECURITY.md contains the lowercase repo spelling; keep the canonical case so the example stays correct if the (?i) prefix is dropped"
elif grep -q 'C1-run/Selo' "$SECURITY_MD"; then
    ok "SECURITY.md documents the canonical repo case (C1-run/Selo)"
else
    bad "SECURITY.md does not document the pinned identity at all"
fi

# --- 12. Loadability guard: one authority, Go, invoked not `go run` --------
# The Mach-O guard was a Python script and is now a Go command. Keeping both
# would give two authorities for one property, and `go run` collapses every
# non-zero exit to 1, which destroys the exit-2 "a glob matched nothing" signal
# that stops a renamed artifact from passing the release gate.
echo "12. loadability guard wiring"
GUARD_GO="$REPO_ROOT/scripts/check-macho-uuid/main.go"
GUARD_PY="$REPO_ROOT/scripts/check-macho-uuid.py"

if [ -f "$GUARD_GO" ]; then
    ok "the guard is a Go command (scripts/check-macho-uuid)"
else
    bad "the Go guard is missing at $GUARD_GO"
fi

if [ -e "$GUARD_PY" ]; then
    bad "the superseded Python guard still exists at $GUARD_PY; two copies of one check will drift"
else
    ok "the superseded Python guard is gone"
fi

for wf in release.yml ci.yml; do
    f="$REPO_ROOT/.github/workflows/$wf"
    if [ ! -f "$f" ]; then
        bad "$wf not found at $f"
        continue
    fi
    if grep -q 'check-macho-uuid\.py' "$f"; then
        bad "$wf still invokes the removed Python guard"
    fi
    if grep -q 'go run .*check-macho-uuid' "$f"; then
        bad "$wf runs the guard via 'go run', which collapses exit 2 into 1"
    fi
    if grep -q 'check-macho-uuid' "$f"; then
        ok "$wf invokes the Go guard"
    else
        bad "$wf does not invoke the loadability guard at all"
    fi
done

if grep -q 'check-macho-uuid\.py' "$SECURITY_MD"; then
    bad "SECURITY.md still points at the removed Python guard"
else
    ok "SECURITY.md does not reference the removed Python guard"
fi

# --- 13. README version label tracks the changelog -------------------------
# The README's capability table and roadmap both name a version, and the roadmap
# links to the table by an anchor that encodes it. Bumping the changelog and not
# the README is silent drift; a stale heading also breaks the anchor silently.
echo "13. README version tracks the changelog"
README_MD="$REPO_ROOT/README.md"
CHANGELOG_MD="$REPO_ROOT/CHANGELOG.md"
latest=$(grep -m1 -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$CHANGELOG_MD" \
    | sed -E 's/^## \[([0-9]+\.[0-9]+)\..*/\1/')
if [ -z "$latest" ]; then
    bad "could not find a released version header in CHANGELOG.md"
else
    if grep -q "What works in v$latest" "$README_MD"; then
        ok "README capability table names v$latest, matching the changelog"
    else
        bad "README does not say 'What works in v$latest' (the changelog's latest release)"
    fi
    anchor=$(echo "$latest" | tr -d '.')
    if grep -q "#what-works-in-v$anchor" "$README_MD"; then
        ok "README roadmap anchor resolves to the current heading"
    else
        bad "README roadmap anchor is stale: expected #what-works-in-v$anchor"
    fi
fi

# --- 14. cosign is fetched with a pinned digest, not via the broken action ---
# sigstore/cosign-installer@v3 cannot install cosign v3.x: after downloading the
# binary it fetches the legacy detached signature (cosign-linux-amd64.sig), which
# cosign v3.x stopped publishing, so `curl -f` exits 22 and the release job dies
# before anything is signed. That is exactly what happened to v0.6.0. Guard the
# replacement so it does not get "simplified" back into the action.
echo "14. cosign acquisition"
RELEASE_YML="$REPO_ROOT/.github/workflows/release.yml"
if grep -qE 'uses:[[:space:]]*sigstore/cosign-installer' "$RELEASE_YML"; then
    bad "release.yml uses sigstore/cosign-installer, which cannot install cosign v3.x (its legacy .sig asset 404s)"
else
    ok "release.yml does not use the cosign installer action"
fi
if grep -qE 'COSIGN_SHA256:[[:space:]]*[0-9a-f]{64}[[:space:]]*$' "$RELEASE_YML"; then
    ok "release.yml pins a full 64-hex cosign digest"
else
    bad "release.yml does not pin a full cosign SHA-256"
fi
if grep -q 'sha256sum -c -' "$RELEASE_YML"; then
    ok "release.yml verifies the downloaded cosign against that digest"
else
    bad "release.yml downloads cosign without verifying it"
fi

# --- 15. the release job can actually see the release notes ----------------
# body_path points at docs/releases/<tag>.md, which only exists if the job has
# checked out the repository. Everything else in that job comes from
# download-artifact, so a checkout is easy to forget -- and forgetting it fails
# the release *after* the signature has been produced.
echo "15. release job can see the release notes"
if grep -q 'body_path' "$RELEASE_YML"; then
    release_job="$(awk '/^  release:/{f=1} /^  provenance:/{f=0} f' "$RELEASE_YML")"
    if printf '%s\n' "$release_job" | grep -q 'actions/checkout'; then
        ok "the release job checks out the tree, so body_path resolves"
    else
        bad "release.yml uses body_path but the release job has no actions/checkout"
    fi
    if printf '%s\n' "$release_job" | grep -q 'docs/releases'; then
        ok "the release job checks the notes file exists before publishing"
    else
        bad "release.yml uses body_path without checking the file exists"
    fi
else
    ok "release.yml does not use body_path"
fi

echo ""
echo "passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ]
