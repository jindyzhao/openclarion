#!/usr/bin/env bash
set -euo pipefail

# Validate pull request titles against the Conventional Commits header shape:
#   <type>[optional scope][optional !]: <description>
#
# The CI job provides PR_TITLE from github.event.pull_request.title. For local
# checks, run: PR_TITLE='feat: add report trigger' make pr-title-check

title="${PR_TITLE:-}"
max_title_chars=120

if [[ -z "$title" ]]; then
  echo "[pr-title-check] PR_TITLE is required." >&2
  echo "[pr-title-check] Usage: PR_TITLE='feat: add concise description' make pr-title-check" >&2
  exit 2
fi

if [[ "$title" == *$'\n'* || "$title" == *$'\r'* ]]; then
  echo "[pr-title-check] PR title must be a single line." >&2
  exit 1
fi

if (( ${#title} > max_title_chars )); then
  echo "[pr-title-check] PR title is ${#title} characters; maximum is ${max_title_chars}." >&2
  exit 1
fi

conventional_header_re='^[a-z][a-z0-9-]*(\([a-z0-9._/-]+\))?(!)?: [^[:space:]]([^[:cntrl:]]*[^[:space:]])?$'
if [[ ! "$title" =~ $conventional_header_re ]]; then
  echo "[pr-title-check] PR title is not a Conventional Commit header:" >&2
  echo "  $title" >&2
  echo "" >&2
  echo "Expected: type(scope)!: description" >&2
  echo "Examples:" >&2
  echo "  feat: add report trigger" >&2
  echo "  fix(alerting): handle duplicate events" >&2
  echo "  chore(ci)!: tighten workflow policy" >&2
  exit 1
fi

echo "[pr-title-check] OK"
