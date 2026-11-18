#!/bin/sh
# Selo paid-pilot delivery rehearsal — offline, deterministic, one run.
#
# Produces receipt.json + verification.log in this directory from a real
# v0.7.x+ release binary. The "agent" is examples/playground/offender.sh,
# a deliberately offending script — NOT an LLM agent. No network, no API
# keys, no hosted services. The receipt is never hand-edited: editing it
# would break the signature.
#
# Requires: git, openssl, shasum, and `selo` in PATH (override with SELO=...).
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
SELO="${SELO:-selo}"

command -v "$SELO" >/dev/null 2>&1 || {
  echo "selo not found — install the current release first" >&2; exit 1;
}
command -v openssl >/dev/null 2>&1 || { echo "openssl required" >&2; exit 1; }

SB="$(mktemp -d "${TMPDIR:-/tmp}/selo-pilot-rehearsal.XXXXXX")"
TMPR="$HERE/receipt.json.tmp"
TMPL="$HERE/verification.log.tmp"
trap 'rm -f "$TMPR" "$TMPL"' EXIT

cp "$ROOT/examples/playground/offender.sh" "$SB/"
chmod +x "$SB/offender.sh"
git -C "$SB" init -q
mkdir -p "$SB/config"
cat > "$SB/config/selo.yaml" <<'EOF'
forge:
  poll_interval_sec: 2
  default_max_minutes: 1
  default_max_rounds: 2
  default_max_files: 20
  default_max_patch_lines: 400
  forbidden_claims:
    - SECURE_BY_DESIGN
    - BATTLE_TESTED
    - PRODUCTION_READY
    - FULLY_TESTED
    - ENTERPRISE_GRADE
    - SOC2_COMPLIANT
  runner:
    mode: real
    command: ./offender.sh
    args: []
  containment:
    strategy: worktree
EOF
git -C "$SB" add -A
git -C "$SB" -c user.name=selo-pilot -c user.email=pilot@local commit -qm seed

echo "== 1) sandbox signing key (isolated from any global key) =="
"$SELO" keys generate --out "$SB/signing-key" | tail -4
PUB="$("$SELO" keys pub --in "$SB/signing-key")"

echo
echo "== 2) run the scripted offender through the pipeline =="
cd "$SB"
SELO_SIGNING_KEY="$(cat "$SB/signing-key")" \
  "$SELO" --config config/selo.yaml run \
       --repo "$SB" --forbidden-files .env --max-minutes 1 \
       "add a login helper (scripted offender — violates on purpose)" || true

R="$(ls "$SB"/runs/*/receipt.json | head -1)"
FP="$(printf '%s' "$PUB" | openssl base64 -d -A | openssl dgst -sha256 | awk '{print $NF}')"

echo
echo "== 3) third-party verification with a pinned fingerprint =="
{
  echo "Selo pilot delivery rehearsal — $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "binary path: $(command -v "$SELO")"
  "$SELO" --version 2>&1
  echo "binary sha256: $(shasum -a 256 "$(command -v "$SELO")" | awk '{print $1}')"
  echo "receipt: $(basename "$R")"
  echo "signer fingerprint (sha256 of decoded public_key): $FP"
  echo "--- verifier output ---"
} > "$TMPL"
"$SELO" verify --pubkey "$FP" "$R" 2>&1 | tee -a "$TMPL"

if ! grep -q 'Result:.*VALID' "$TMPL" || ! grep -q 'Provenance: PINNED' "$TMPL"; then
  echo "verification did not pass — no receipt published" >&2
  exit 1
fi

"$SELO" receipt show "$R" --format text > "$HERE/receipt-dump.txt.tmp" 2>&1 || true
cp "$R" "$TMPR"
mv "$TMPR" "$HERE/receipt.json"
mv "$TMPL" "$HERE/verification.log"
mv "$HERE/receipt-dump.txt.tmp" "$HERE/receipt-dump.txt"

echo
echo "receipt id: $("$SELO" receipt show "$HERE/receipt.json" --format text | grep -m1 'Receipt:' || true)"
echo "fingerprint: $FP"
echo "verification: VALID / PINNED (see verification.log)"
echo "sandbox kept for inspection: $SB  (delete when finished)"
