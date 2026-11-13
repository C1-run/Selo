#!/usr/bin/env bash
# Post (or update) a sticky PR comment containing a rendered Selo receipt card.
#
# The comment is found by its leading marker and updated in place, so repeated
# runs do not pile up comments on a PR.
#
# Usage: post-receipt-comment.sh <card-file>
# Environment (set by GitHub Actions):
#   PR_NUMBER          PR to comment on (required)
#   GITHUB_REPOSITORY  owner/repo
#   GH_TOKEN           token with pull-requests: write
set -euo pipefail

if [ $# -ne 1 ]; then
	echo "usage: $0 <card-file>" >&2
	exit 2
fi
if [ -z "${PR_NUMBER:-}" ]; then
	echo "PR_NUMBER is required" >&2
	exit 2
fi

card_file="$1"
marker="<!-- selo-receipt: updated in place, do not remove this marker -->"

body="${marker}
$(cat "$card_file")"

existing_id=$(gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
	--paginate --jq ".[] | select(.body | startswith(\"${marker}\")) | .id" | head -1)

if [ -n "${existing_id:-}" ]; then
	gh api -X PATCH "repos/${GITHUB_REPOSITORY}/issues/comments/${existing_id}" -f body="$body" >/dev/null
	echo "updated sticky comment ${existing_id} on PR ${PR_NUMBER}"
else
	gh api -X POST "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" -f body="$body" >/dev/null
	echo "posted receipt comment on PR ${PR_NUMBER}"
fi
