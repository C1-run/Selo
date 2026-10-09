#!/bin/bash
# Selo installer — downloads the latest release binary and verifies it before
# installing.
#
#   curl -fsSL https://raw.githubusercontent.com/C1-run/selo/main/install.sh | bash
#
# Verification is on by default and fails closed. The binary's SHA-256 is
# checked against checksums.txt, and checksums.txt is checked against a Sigstore
# keyless signature produced by the release workflow (ADR-002). If cosign is
# missing, or if either check fails, nothing is installed.
#
# Escape hatch: SELO_SKIP_VERIFY=1 skips verification with a loud warning.
#
# Environment overrides:
#   INSTALL_DIR                 target directory (default /usr/local/bin)
#   SELO_VERSION                install a specific tag instead of "latest"
#   SELO_BASE_URL               fetch artifacts from here instead of GitHub
#                               (used by scripts/test-install.sh)
#   SELO_SKIP_VERIFY            1 = skip verification (dangerous)
#   SELO_REPO                   owner/name (default C1-run/selo)
#   SELO_CERT_IDENTITY_REGEXP   override the pinned workflow identity
#   SELO_CERT_OIDC_ISSUER       override the pinned OIDC issuer
set -euo pipefail

REPO="${SELO_REPO:-C1-run/selo}"
BINARY="selo"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

BASE_URL="${SELO_BASE_URL:-}"
VERSION="${SELO_VERSION:-}"
SKIP_VERIFY="${SELO_SKIP_VERIFY:-0}"

# A release signature is accepted only if the Fulcio certificate matches BOTH
# the workflow identity and the OIDC issuer. Pinning both is what stops a
# signature from some other repository, workflow, or OIDC provider from being
# accepted in place of ours.
DEFAULT_CERT_IDENTITY_REGEXP="^https://github\.com/${REPO}/\.github/workflows/release\.yml@refs/tags/v.*\$"
CERT_IDENTITY_REGEXP="${SELO_CERT_IDENTITY_REGEXP:-$DEFAULT_CERT_IDENTITY_REGEXP}"
CERT_OIDC_ISSUER="${SELO_CERT_OIDC_ISSUER:-https://token.actions.githubusercontent.com}"

die() { echo "Error: $*" >&2; exit 1; }

# --- platform detection -----------------------------------------------------
OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
    Linux*)  OS="linux" ;;
    Darwin*) OS="darwin" ;;
    *)       die "unsupported OS: $OS" ;;
esac
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             die "unsupported architecture: $ARCH" ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"

# --- resolve the download base ---------------------------------------------
if [ -n "$BASE_URL" ]; then
    BASE="${BASE_URL%/}"
    VERSION="${VERSION:-local}"
else
    if [ -z "$VERSION" ]; then
        echo "Fetching latest release..."
        # grep -m1 rather than `| head -1`: head closing the pipe early can
        # raise SIGPIPE in grep, which `set -o pipefail` would turn into a
        # spurious failure.
        VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
            | grep -m1 '"tag_name"' \
            | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')" || VERSION=""
    fi
    [ -n "$VERSION" ] || die "could not determine latest version"
    echo "Latest version: $VERSION"
    BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

# --- download ---------------------------------------------------------------
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "Downloading $ASSET from $BASE ..."
# Fail closed: a missing checksum file or bundle aborts the install instead of
# silently degrading to an unverified one.
curl -fsSL -o "$WORK/$ASSET"                      "$BASE/$ASSET"
curl -fsSL -o "$WORK/checksums.txt"               "$BASE/checksums.txt"
curl -fsSL -o "$WORK/checksums.txt.sigstore.json" "$BASE/checksums.txt.sigstore.json"

# --- verify -----------------------------------------------------------------
sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        die "no SHA-256 tool found (need sha256sum or shasum)"
    fi
}

if [ "$SKIP_VERIFY" = "1" ]; then
    echo "" >&2
    echo "!! WARNING: SELO_SKIP_VERIFY=1 — signature verification is DISABLED." >&2
    echo "!! The downloaded binary will be installed WITHOUT verification." >&2
    echo "" >&2
else
    # 1. The binary must match the checksum recorded for it.
    expected="$(awk -v f="$ASSET" '$2 == f { print $1 }' "$WORK/checksums.txt")"
    [ -n "$expected" ] || die "$ASSET is not listed in checksums.txt"
    actual="$(sha256_of "$WORK/$ASSET")"
    if [ "$expected" != "$actual" ]; then
        echo "  expected $expected" >&2
        echo "  actual   $actual" >&2
        die "checksum mismatch for $ASSET"
    fi
    echo "Checksum OK: $ASSET"

    # 2. checksums.txt must carry a valid signature from our release workflow.
    command -v cosign >/dev/null 2>&1 || die \
"cosign is required to verify the release signature but was not found on PATH.
Install it (https://docs.sigstore.dev/cosign/system_config/installation/) or
re-run with SELO_SKIP_VERIFY=1 to install without verification."

    cosign verify-blob \
        --bundle "$WORK/checksums.txt.sigstore.json" \
        --certificate-identity-regexp "$CERT_IDENTITY_REGEXP" \
        --certificate-oidc-issuer "$CERT_OIDC_ISSUER" \
        "$WORK/checksums.txt" >/dev/null \
        || die "signature verification failed for checksums.txt"
    echo "Signature OK: checksums.txt"
fi

# --- install ----------------------------------------------------------------
chmod +x "$WORK/$ASSET"
if [ ! -d "$INSTALL_DIR" ]; then
    echo "Creating $INSTALL_DIR ..."
    sudo mkdir -p "$INSTALL_DIR"
fi
if [ -w "$INSTALL_DIR" ]; then
    mv "$WORK/$ASSET" "$INSTALL_DIR/$BINARY"
else
    sudo mv "$WORK/$ASSET" "$INSTALL_DIR/$BINARY"
fi

echo ""
echo "Selo $VERSION installed to $INSTALL_DIR/$BINARY"
echo "Run 'selo --help' to get started."
