#!/bin/sh
# Pilot readiness check — run before any paid offer is sent.
#
# Gate 1: the rehearsal receipt verifies with a current release binary.
# Gate 2: all four written templates have been filled in (no _fill_ left).
# Gate 3: a named client-side technical approver is recorded in the scope.
#
# Prints READY / NOT READY per gate and exits 1 if anything is open.
# No network, no CLI telemetry, no side effects.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
SELO="${SELO:-selo}"
OPEN=0

say() { printf '%s\n' "$*"; }

say "Selo pilot readiness — $(date -u '+%Y-%m-%dT%H:%M:%SZ')"

# --- Gate 1: real, verifiable, engine-signed rehearsal receipt -------------
R="$HERE/rehearsal/receipt.json"
if [ ! -f "$R" ]; then
  say "GATE 1 NOT READY: rehearsal/receipt.json is missing"
  OPEN=1
elif ! command -v "$SELO" >/dev/null 2>&1; then
  say "GATE 1 NOT READY: selo not found (install a current release)"
  OPEN=1
elif ! command -v openssl >/dev/null 2>&1; then
  say "GATE 1 NOT READY: openssl required to derive the signer fingerprint"
  OPEN=1
else
  PUB="$(grep -o '"public_key" *: *"[^"]*"' "$R" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
  FP="$(printf '%s' "$PUB" | openssl base64 -d -A 2>/dev/null | openssl dgst -sha256 | awk '{print $NF}')"
  OUT="$("$SELO" verify --pubkey "$FP" "$R" 2>&1)" || OUT=""
  if printf '%s\n' "$OUT" | grep -q 'Result:.*VALID' && \
     printf '%s\n' "$OUT" | grep -q 'Provenance: PINNED'; then
    RID="$(grep -o '"receipt_id" *: *"[^"]*"' "$R" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
    say "GATE 1 READY: $RID verifies PINNED / VALID ($("$SELO" --version 2>&1 | head -1))"
  else
    say "GATE 1 NOT READY: receipt did not verify PINNED / VALID"
    OPEN=1
  fi
fi

# --- Gate 2: the four written terms are filled, not templates -------------
for name in scope privacy ip acceptance; do
  F="$HERE/$name.md"
  if [ ! -f "$F" ]; then
    say "GATE 2 NOT READY: $name.md missing — fill a copy of $name-template.md"
    OPEN=1
  elif grep -q '_fill_' "$F"; then
    say "GATE 2 NOT READY: $name.md still contains _fill_ placeholders"
    OPEN=1
  fi
done
if [ "$OPEN" -eq 0 ]; then
  say "GATE 2 READY: scope, privacy, IP and acceptance terms are filled"
fi

# --- Gate 3: a named client-side technical approver ----------------------
S="$HERE/scope.md"
APPROVER=""
if [ -f "$S" ]; then
  APPROVER="$(grep -i 'Named client-side technical approver' "$S" | head -1 | awk -F'|' '{v=$3; gsub(/^[ \t]+|[ \t]+$/, "", v); print v}')"
fi
if [ -z "$APPROVER" ] || printf '%s' "$APPROVER" | grep -qi '^_\|fill'; then
  say "GATE 3 NOT READY: no named client-side technical approver in scope.md"
  OPEN=1
else
  say "GATE 3 READY: named approver recorded — $APPROVER"
fi

if [ "$OPEN" -ne 0 ]; then
  say "RESULT: NOT READY — do not send a paid offer"
  exit 1
fi
say "RESULT: READY — all three gates closed; a paid offer may be discussed"
