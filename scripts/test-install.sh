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

echo ""
echo "passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ]
