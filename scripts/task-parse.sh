#!/usr/bin/env bash
# Shared task.md field extraction for Selo's shell runners.
#
# One parser here instead of a copy per runner: the Go canonical parser is
# internal/taskmeta, and internal/taskmeta's shell drift test checks this file
# against it — a task file written by taskmeta.Write must extract to the same
# fields through bash as it parses to through Go.
#
# Usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/task-parse.sh"
#   task_parse_fields "$TASK_FILE"
# Sets:
#   TASK_GOAL             goal value, surrounding quotes stripped
#   TASK_ID               id value, surrounding quotes stripped
#   TASK_COMMANDS         command values, one per line (prefix and quotes
#                         stripped — eval-ready)
#   TASK_FORBIDDEN_FILES  forbidden_files values, one per line
#
# Contract with the Go side: the canonical writer (internal/taskmeta.Write)
# emits column-0 keys and two-space-indented quoted items ("- \"item\"").
# The list patterns here match indented items on purpose: the per-runner
# awk copies this replaces required column-0 items and therefore matched
# nothing the canonical writer ever emitted. Do not feed these extractors a
# task file written by anything else.

# strip_quotes removes surrounding double or single quotes from one line.
strip_quotes() {
	sed 's/^"//; s/"$//; s/^'"'"'//; s/'"'"'$//'
}

task_parse_fields() {
	local file="$1"

	TASK_GOAL=$(grep '^goal:' "$file" 2>/dev/null | sed 's/^goal:[[:space:]]*//' | head -1 | strip_quotes)
	TASK_ID=$(grep '^id:' "$file" 2>/dev/null | sed 's/^id:[[:space:]]*//' | head -1 | strip_quotes)

	# commands_to_run is accepted as an alias, matching the Go parser.
	local item_sed='s/^[[:space:]]*-[[:space:]]*//; s/^"//; s/"$//; s/^'"'"'//; s/'"'"'$//'
	TASK_COMMANDS=$(awk '/^(commands|commands_to_run):/{flag=1; next} /^[a-z]/{flag=0} flag && /^[[:space:]]*- /{print $0}' "$file" 2>/dev/null | sed "$item_sed" || true)
	TASK_FORBIDDEN_FILES=$(awk '/^forbidden_files:/{flag=1; next} /^[a-z]/{flag=0} flag && /^[[:space:]]*- /{print $0}' "$file" 2>/dev/null | sed "$item_sed" || true)
}
