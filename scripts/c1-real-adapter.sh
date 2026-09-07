#!/usr/bin/env bash
#
# c1-real-adapter.sh — Adapts C1 Forge's expected CLI interface
# (--task-file <path> --workdir <dir>) to the real C1 Loop binary's
# actual interface: c1 loop "<task>" --runtime=<name> --max-rounds=<n>
#
# Supports SELO_C1_RUNTIME env var to select runtime:
#   mock   (default) - audit only, no file changes
#   shell            - shell-based agent, audit only in v0.1
#
# Fail closed if the real c1 binary is unavailable.
# Must not fake work — always calls real c1 binary.

set -euo pipefail

# --- Parse forge arguments ---
TASK_FILE=""
WORKDIR=""
MAX_ROUNDS=1
RUNTIME="mock"

while [ $# -gt 0 ]; do
    case "$1" in
        --task-file) TASK_FILE="$2"; shift 2 ;;
        --workdir)   WORKDIR="$2"; shift 2 ;;
        --max-rounds) MAX_ROUNDS="$2"; shift 2 ;;
        --runtime)   RUNTIME="$2"; shift 2 ;;
        *) echo "[c1-real-adapter] WARNING: unknown argument: $1" >&2; shift ;;
    esac
done

if [ -z "$TASK_FILE" ]; then
    echo "[c1-real-adapter] FATAL: --task-file is required" >&2
    exit 99
fi
if [ -z "$WORKDIR" ]; then
    echo "[c1-real-adapter] FATAL: --workdir is required" >&2
    exit 99
fi

# Resolve runtime: env var overrides --runtime flag
if [ -n "${SELO_C1_RUNTIME:-}" ]; then
    RUNTIME="$SELO_C1_RUNTIME"
fi

# --- Move to workdir early ---
mkdir -p "$WORKDIR"
cd "$WORKDIR"

# --- Discover real C1 binary ---
C1_BIN="${SELO_C1_BIN:-}"
if [ -z "$C1_BIN" ]; then
    C1_BIN="$(command -v c1 2>/dev/null || true)"
fi
if [ -z "$C1_BIN" ]; then
    C1_BIN="$(command -v opencode 2>/dev/null || true)"
fi

if [ -z "$C1_BIN" ]; then
    echo "[c1-real-adapter] FATAL: no c1 binary found (SELO_C1_BIN, PATH c1, or PATH opencode)" >&2
    exit 99
fi
if [ ! -x "$C1_BIN" ]; then
    echo "[c1-real-adapter] FATAL: c1 binary not executable: $C1_BIN" >&2
    exit 99
fi

echo "[c1-real-adapter] Using C1 binary: $C1_BIN" >&2
echo "[c1-real-adapter] Runtime: $RUNTIME" >&2
echo "[c1-real-adapter] Task file: $TASK_FILE" >&2
echo "[c1-real-adapter] Workdir: $WORKDIR" >&2

# --- Extract task goal from task file ---
TASK_GOAL=""
if [ -f "$TASK_FILE" ]; then
    TASK_GOAL="$(grep -E '^(goal|title):' "$TASK_FILE" | head -1 | sed 's/^[^:]*:[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' || true)"
fi
if [ -z "$TASK_GOAL" ]; then
    TASK_GOAL="process task $(basename "$TASK_FILE")"
fi

echo "[c1-real-adapter] Task goal: $TASK_GOAL" >&2

# --- Validate runtime ---
case "$RUNTIME" in
    mock|shell) ;;
    *)
        echo "[c1-real-adapter] ERROR: unsupported runtime: $RUNTIME (supported: mock, shell)" >&2
        exit 99
        ;;
esac

# --- Ensure c1 is initialized ---
if [ ! -f ".c1/config.yaml" ]; then
    echo "[c1-real-adapter] Running c1 init..." >&2
    "$C1_BIN" init 2>&1
fi
if [ ! -f ".c1/config.yaml" ]; then
    echo "[c1-real-adapter] Error: c1 init failed" >&2
    exit 99
fi

# Update config runtime if different
CURRENT_RUNTIME="$(grep -E '^runtime:' .c1/config.yaml 2>/dev/null | sed 's/^runtime:[[:space:]]*//' || true)"
if [ "$CURRENT_RUNTIME" != "$RUNTIME" ]; then
    echo "[c1-real-adapter] Config says runtime=$CURRENT_RUNTIME, requested=$RUNTIME" >&2
    # The --runtime flag overrides config, so this is informational only
fi

# --- Run c1 loop ---
echo "[c1-real-adapter] Running: c1 loop \"$TASK_GOAL\" --runtime=$RUNTIME --max-rounds=$MAX_ROUNDS" >&2
"$C1_BIN" loop "$TASK_GOAL" --runtime="$RUNTIME" --max-rounds="$MAX_ROUNDS"
C1_EXIT=$?

echo "[c1-real-adapter] C1 loop exited with code: $C1_EXIT" >&2

# --- Find and copy the latest C1 receipt to worktree root ---
LATEST_RUN=""
if [ -d ".c1/runs" ]; then
    LATEST_RUN="$(ls -t .c1/runs 2>/dev/null | head -1 || true)"
fi

if [ -n "$LATEST_RUN" ] && [ -f ".c1/runs/$LATEST_RUN/receipt.json" ]; then
    cp ".c1/runs/$LATEST_RUN/receipt.json" "c1-receipt.json"
    echo "[c1-real-adapter] Copied C1 receipt to: $WORKDIR/c1-receipt.json" >&2
elif [ -f "c1-receipt.json" ]; then
    echo "[c1-real-adapter] C1 receipt already exists" >&2
else
    echo "[c1-real-adapter] No C1 receipt found (may be a no-op run)" >&2
fi

# Write runtime info for Forge to read
IS_MOCK="false"
if [ "$RUNTIME" = "mock" ]; then IS_MOCK="true"; fi
echo "{\"runtime_requested\": \"$RUNTIME\", \"runtime_used\": \"$RUNTIME\", \"is_mock\": $IS_MOCK}" > "c1-runtime-info.json"

exit $C1_EXIT
