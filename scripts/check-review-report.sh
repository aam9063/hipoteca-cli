#!/usr/bin/env bash
# Enforces the review gate of the arranque, section 10.5: a PR whose title carries a
# backlog ID must ship odd/reviews/<ID>.md.
set -euo pipefail

TITLE="${PR_TITLE:-}"
if [ -z "$TITLE" ]; then
  echo "PR_TITLE is empty: nothing to check"
  exit 0
fi

# A title may reference a branch like hc/OPS-01-something; those tokens carry a slash
# and are ignored: only a token that is exactly a backlog ID counts.
id="$(printf '%s' "$TITLE" | grep -oE '[[:alnum:]_/-]+' | while read -r token; do
  case "$token" in */*) continue ;; esac
  printf '%s\n' "$token" | tr '[:lower:]' '[:upper:]' | grep -E '^[A-Z]+-[0-9]+$' && break
done | head -1 || true)"
if [ -z "$id" ]; then
  echo "PR title carries no backlog ID: skipping the review-report check"
  exit 0
fi

report="odd/reviews/$id.md"
if [ ! -f "$report" ]; then
  echo "PR title names $id but $report is missing." >&2
  echo "Write the review gate report (arranque section 10.5) before asking for review." >&2
  exit 1
fi

echo "review report present: $report"
