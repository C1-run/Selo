#!/bin/sh
# selo playground — one command, full pipeline, all offline.
#
# Deterministic by design: the "agent" is offender.sh, a script that
# violates two rules on purpose. No LLM, no API keys, no network.
# Everything savable lives in a throwaway sandbox that is kept at the
# end so you can open the receipt yourself. See README.md first.
#
# Requires: git, an installed `selo` binary, openssl (fingerprint math).
set -eu

PG="$(cd "$(dirname "$0")" && pwd)"
SB="$(mktemp -d "${TMPDIR:-/tmp}/selo-playground.XXXXXX")"

cp "$PG/offender.sh" "$SB/"
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
  # Explicit for determinism (the engine ships these as defaults too).
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
git -C "$SB" -c user.name=selo-playground -c user.email=playground@local commit -qm seed

echo "== 1) sandbox signing key — isolated from your global ~/.selo/signing-key =="
selo keys generate --out "$SB/signing-key" | tail -4
PUB="$(selo keys pub --in "$SB/signing-key")"

echo
echo "== 2) run the offender through the pipeline (no network) =="
cd "$SB"
SELO_SIGNING_KEY="$(cat "$SB/signing-key")" \
  selo --config config/selo.yaml run \
       --repo "$SB" --forbidden-files .env --max-minutes 1 \
       "add a login helper (script agent — it violates on purpose)" || true

echo
echo "== 3) the receipt: read the verdict from it, never from the agent =="
selo receipt show "$SB"/runs/*/receipt.json --format text

echo
echo "== 4) verify with your own tool — pinned key, recheck by hand any time =="
FP="$(printf '%s' "$PUB" | openssl base64 -d -A 2>/dev/null | openssl dgst -sha256 | awk '{print $NF}')"
if [ -n "$FP" ]; then
  echo "key fingerprint (sha256 of the signer's public key): $FP"
  selo verify --pubkey "$FP" "$SB"/runs/*/receipt.json
else
  echo "(fingerprint unavailable — verifying from pubkey file instead)"
  printf '%s\n' "$PUB" > "$SB/out.pub"
  selo verify --pubkey "$SB/out.pub" "$SB"/runs/*/receipt.json
fi

echo
echo "== 5) tamper test — flip one byte in the receipt; verify must fail =="
perl -pi -e 's/FAILED_SAFETY/FAILED_TAMPER_DEMO/g' "$SB"/runs/*/receipt.json
selo verify --pubkey "$FP" "$SB"/runs/*/receipt.json | tail -6

echo
echo "sandbox kept for inspection: $SB  (open it, then delete it)"
