#!/usr/bin/env bash
#
# opencode-adapter.sh — Adapts C1 Forge's expected CLI interface
# (--task-file <path> --workdir <dir>) to the OpenCode binary.
#
# Uses opencode serve (headless) + opencode run --attach (one-shot) for clean exit.
# OpenCode is the first real coding worker for C1 Forge.
# Unlike C1 Loop v0.1 (audit-only), OpenCode can produce real file diffs.
#
# Process cleanup:
#   - Records server PID
#   - Installs EXIT/INT/TERM trap to kill serve process
#   - Waits for termination with timeout
#   - Force kills if still alive
#
# Timeout hardening:
#   - Serve startup timeout (30s, configurable via SELO_SERVE_TIMEOUT)
#   - Run timeout (from max_minutes or SELO_RUN_TIMEOUT_SEC)
#   - Cleanup timeout (5s)
#
# Safety rules (fail closed):
#   - must call real opencode binary
#   - must run inside worktree
#   - must not read .env
#   - must not git push
#   - must not deploy
#   - must not npm publish

set -euo pipefail
shopt -s inherit_errexit 2>/dev/null || true

# --- Configuration ---
SERVE_TIMEOUT="${SELO_SERVE_TIMEOUT:-30}"
RUN_TIMEOUT_SEC="${SELO_RUN_TIMEOUT_SEC:-300}"
CLEANUP_TIMEOUT=5
SERVER_PID=""
RUN_EXIT_STATUS=""
TIMEOUT_HIT="false"
SERVER_EXIT_STATUS=""
SERVER_STARTED=""
SERVER_KILLED=""

# --- Cleanup function ---
cleanup() {
    local exit_code=$?
    local now
    now=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    if [ -n "$SERVER_PID" ]; then
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            echo "[opencode-adapter] Cleaning up server (PID $SERVER_PID)..." >&2
            SERVER_KILLED="$now"
            # Graceful kill first
            kill "$SERVER_PID" 2>/dev/null || true
            # Wait up to CLEANUP_TIMEOUT seconds
            local waited=0
            while kill -0 "$SERVER_PID" 2>/dev/null && [ $waited -lt "$CLEANUP_TIMEOUT" ]; do
                sleep 1
                waited=$((waited + 1))
            done
            # Force kill if still alive
            if kill -0 "$SERVER_PID" 2>/dev/null; then
                echo "[opencode-adapter] Force killing server (PID $SERVER_PID)..." >&2
                kill -9 "$SERVER_PID" 2>/dev/null || true
            fi
            # Collect exit status
            wait "$SERVER_PID" 2>/dev/null || true
            SERVER_EXIT_STATUS=$?
        else
            # Server already exited; collect status
            wait "$SERVER_PID" 2>/dev/null || true
            SERVER_EXIT_STATUS=$?
        fi
    fi

    # Clean up serve log
    rm -f "$SERVE_LOG"

    # Write run info (only if workdir exists)
    if [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ]; then
        write_run_info "$exit_code"
    fi

    exit "$exit_code"
}

# --- Write opencode-run-info.json ---
write_run_info() {
    local final_exit=$1
    cat > "$WORKDIR/opencode-run-info.json" << EOF
{
  "binary_path": "$OPENCODE_BIN",
  "exit_code": $final_exit,
  "model": "$OPENCODE_MODEL",
  "agent": "$OPENCODE_AGENT",
  "server_pid": ${SERVER_PID:-null},
  "server_started": "${SERVER_STARTED:-}",
  "server_killed": "${SERVER_KILLED:-}",
  "server_exit_status": ${SERVER_EXIT_STATUS:-null},
  "run_exit_status": ${RUN_EXIT_STATUS:-null},
  "timeout_hit": $TIMEOUT_HIT
}
EOF
}

# --- Parse forge arguments ---
TASK_FILE=""
WORKDIR=""
MAX_ROUNDS=1
MAX_MINUTES=0

while [ $# -gt 0 ]; do
    case "$1" in
        --task-file) TASK_FILE="$2"; shift 2 ;;
        --workdir)   WORKDIR="$2"; shift 2 ;;
        --max-rounds) MAX_ROUNDS="$2"; shift 2 ;;
        --max-minutes) MAX_MINUTES="$2"; shift 2 ;;
        *) echo "[opencode-adapter] WARNING: unknown argument: $1" >&2; shift ;;
    esac
done

if [ -z "$TASK_FILE" ]; then
    echo "[opencode-adapter] FATAL: --task-file is required" >&2
    exit 99
fi
if [ -z "$WORKDIR" ]; then
    echo "[opencode-adapter] FATAL: --workdir is required" >&2
    exit 99
fi

# --- Move to workdir early ---
mkdir -p "$WORKDIR"
cd "$WORKDIR"

# --- Discover OpenCode binary ---
OPENCODE_BIN="${SELO_OPENCODE_BIN:-}"
if [ -z "$OPENCODE_BIN" ]; then
    OPENCODE_BIN="$(command -v opencode 2>/dev/null || true)"
fi

if [ -z "$OPENCODE_BIN" ]; then
    echo "[opencode-adapter] FATAL: no opencode binary found (SELO_OPENCODE_BIN or PATH opencode)" >&2
    exit 99
fi
if [ ! -x "$OPENCODE_BIN" ]; then
    echo "[opencode-adapter] FATAL: opencode binary not executable: $OPENCODE_BIN" >&2
    exit 99
fi

echo "[opencode-adapter] Using OpenCode binary: $OPENCODE_BIN" >&2
echo "[opencode-adapter] Task file: $TASK_FILE" >&2
echo "[opencode-adapter] Workdir: $WORKDIR" >&2

# --- Pre-initialize run info fields (needed by cleanup trap) ---
OPENCODE_MODEL="${SELO_OPENCODE_MODEL:-}"
OPENCODE_AGENT="${SELO_OPENCODE_AGENT:-}"

# --- Install cleanup trap ---
trap cleanup EXIT INT TERM

# --- Extract task info from task file ---
TASK_GOAL=""
if [ -f "$TASK_FILE" ]; then
    TASK_GOAL="$(grep -E '^(goal|title):' "$TASK_FILE" | head -1 | sed 's/^[^:]*:[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' || true)"

    # Extract max_minutes for run timeout
    TASK_MAX_MINUTES="$(grep -E '^max_minutes:' "$TASK_FILE" | head -1 | sed 's/^max_minutes:[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' || true)"
    if [ -n "$TASK_MAX_MINUTES" ] && [ "$TASK_MAX_MINUTES" -gt 0 ] 2>/dev/null; then
        MAX_MINUTES="$TASK_MAX_MINUTES"
    fi
fi
if [ -z "$TASK_GOAL" ]; then
    TASK_GOAL="process task $(basename "$TASK_FILE")"
fi

echo "[opencode-adapter] Task goal: $TASK_GOAL" >&2
echo "[opencode-adapter] Max minutes: $MAX_MINUTES" >&2

# --- Resolve OpenCode config ---
MODEL="${SELO_OPENCODE_MODEL:-}"
AGENT="${SELO_OPENCODE_AGENT:-}"

# --- Start OpenCode headless server ---
SERVE_LOG="$(mktemp -t opencode-serve-XXXXXX.log)"
SERVER_STARTED="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
"$OPENCODE_BIN" serve --port 0 --print-logs > "$SERVE_LOG" 2>&1 &
SERVER_PID=$!

# Wait for server and parse port from log
SERVER_URL=""
for i in $(seq 1 "$SERVE_TIMEOUT"); do
    sleep 1
    SERVER_URL="$(grep -o 'listening on http://[^ ]*' "$SERVE_LOG" 2>/dev/null | head -1 | awk '{print $3}')"
    if [ -n "$SERVER_URL" ]; then
        break
    fi
done

if [ -z "$SERVER_URL" ]; then
    echo "[opencode-adapter] FATAL: opencode serve did not start within ${SERVE_TIMEOUT}s" >&2
    kill "$SERVER_PID" 2>/dev/null || true
    exit 99
fi

echo "[opencode-adapter] Server started at $SERVER_URL (PID $SERVER_PID)" >&2

# --- Build run arguments ---
RUN_ARGS=("--dangerously-skip-permissions" "--attach" "$SERVER_URL")
if [ -n "$MODEL" ]; then
    RUN_ARGS+=("--model" "$MODEL")
fi
if [ -n "$AGENT" ]; then
    RUN_ARGS+=("--agent" "$AGENT")
fi

echo "[opencode-adapter] Running: $OPENCODE_BIN run ${RUN_ARGS[*]} <goal>" >&2

# --- Compute run deadline ---
# Use max_minutes (in minutes) from task, convert to seconds, cap at RUN_TIMEOUT_SEC
RUN_TIMEOUT=$RUN_TIMEOUT_SEC
if [ "$MAX_MINUTES" -gt 0 ] 2>/dev/null; then
    TASK_TIMEOUT=$((MAX_MINUTES * 60))
    # Use the smaller of task timeout and default run timeout
    if [ "$TASK_TIMEOUT" -lt "$RUN_TIMEOUT" ]; then
        RUN_TIMEOUT=$TASK_TIMEOUT
    fi
fi

echo "[opencode-adapter] Run timeout: ${RUN_TIMEOUT}s" >&2

# --- Run OpenCode one-shot with timeout ---
TIMEOUT_HIT="false"
RUN_EXIT_STATUS=""
if command -v timeout >/dev/null 2>&1; then
    # Use GNU timeout if available
    timeout "$RUN_TIMEOUT" "$OPENCODE_BIN" run "${RUN_ARGS[@]}" "$TASK_GOAL"
    RUN_EXIT_STATUS=$?
    if [ $RUN_EXIT_STATUS -eq 124 ]; then
        TIMEOUT_HIT="true"
        echo "[opencode-adapter] FATAL: OpenCode run timed out after ${RUN_TIMEOUT}s" >&2
    fi
else
    # Fallback: no timeout command, run directly
    "$OPENCODE_BIN" run "${RUN_ARGS[@]}" "$TASK_GOAL"
    RUN_EXIT_STATUS=$?
fi

echo "[opencode-adapter] OpenCode run exited with code: $RUN_EXIT_STATUS (timeout=$TIMEOUT_HIT)" >&2

# --- Capture model/agent from env or config ---
OPENCODE_MODEL="$MODEL"
if [ -z "$OPENCODE_MODEL" ]; then
    OPENCODE_MODEL="$(grep -i 'model' "$WORKDIR/.opencode/opencode.json" 2>/dev/null | head -1 | sed 's/.*"model"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)"
fi

OPENCODE_AGENT="$AGENT"
if [ -z "$OPENCODE_AGENT" ]; then
    OPENCODE_AGENT="$(grep -i 'agent' "$WORKDIR/.opencode/opencode.json" 2>/dev/null | head -1 | sed 's/.*"agent"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)"
fi

# --- Exit with run exit status (cleanup trap will write run info and kill server) ---
if [ "$TIMEOUT_HIT" = "true" ]; then
    exit 124
fi
exit "$RUN_EXIT_STATUS"
