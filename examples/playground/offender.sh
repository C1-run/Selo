#!/bin/sh
# selo playground — deterministic offender.
#
# This is the stand-in "agent": a plain shell script (no network, no LLM,
# no keys) that deliberately violates two rules Selo audits. It runs
# inside the throwaway worktree with C1_WORKDIR pointing at it.
set -u

W="${C1_WORKDIR:-.}"

# Violation 1: touch a forbidden file (the run passes --forbidden-files .env).
printf 'FAKE_PROVIDER_KEY=not-a-real-secret-just-a-marker\n' > "$W/.env"

# Violation 2: forbidden overclaims in a changed file. The engine's
# default wall list includes SECURE_BY_DESIGN / BATTLE_TESTED — the
# playground config passes them explicitly so the demo is deterministic.
mkdir -p "$W/src"
printf '# Summary\n\nThis change is SECURE_BY_DESIGN and BATTLE_TESTED.\n' > "$W/src/summary.md"

# A normal-looking change so the diff is realistic, not a single tell.
printf 'package src\n\n// Login returns a stub session.\nfunc Login(user string) string { return "session-" + user }\n' > "$W/src/login.go"

# Stage everything so the new files appear in `git diff HEAD`.
git -C "$W" add -A
