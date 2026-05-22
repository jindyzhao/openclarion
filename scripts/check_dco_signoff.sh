#!/usr/bin/env bash
# scripts/check_dco_signoff.sh
#
# Validate Developer Certificate of Origin sign-off on commits.
#
# Modes:
#   - CI (pull_request):  driven by env DCO_BASE_REF and DCO_HEAD_SHA
#                         (set from github.base_ref and
#                          github.event.pull_request.head.sha).
#   - Local:              when env vars are absent, validate
#                         `@{u}..HEAD` if an upstream is configured,
#                         otherwise fall back to validating only HEAD.
#
# Exit non-zero if any non-merge commit in the range is missing a
# `Signed-off-by:` trailer. See DCO.md for the policy and
# docs/design/DEPENDENCIES.md for the dependency-pinning policy this
# gate is paired with.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v git >/dev/null 2>&1; then
  echo "[dco-check] git not available." >&2
  exit 1
fi

base_ref="${DCO_BASE_REF:-}"
head_sha="${DCO_HEAD_SHA:-}"

range=""
if [[ -n "$base_ref" && -n "$head_sha" ]]; then
  # CI mode: PR diff range. Fetch the base ref locally if needed so the
  # gate is self-contained and the workflow YAML stays a pure
  # `make <target>` caller (workflow-parity).
  if ! git rev-parse --verify "$base_ref" >/dev/null 2>&1; then
    if ! git fetch --no-tags --prune origin "${base_ref}:${base_ref}" >/dev/null 2>&1; then
      echo "[dco-check] base ref '$base_ref' not present and fetch failed." >&2
      exit 1
    fi
  fi
  range="${base_ref}..${head_sha}"
else
  # Local mode.
  if git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
    range='@{u}..HEAD'
  else
    range='HEAD~0..HEAD'
  fi
fi

commits="$(git rev-list --no-merges "$range" 2>/dev/null || true)"

if [[ -z "$commits" ]]; then
  echo "[dco-check] no commits to validate (range: $range)."
  exit 0
fi

missing=0
while IFS= read -r sha; do
  [[ -z "$sha" ]] && continue
  msg="$(git show -s --format=%B "$sha")"
  if ! printf '%s\n' "$msg" \
      | grep -Eiq '^Signed-off-by:[[:space:]]+.+<.+>[[:space:]]*$'; then
    if [[ "$missing" -eq 0 ]]; then
      echo "[dco-check] missing Signed-off-by trailer:"
    fi
    git show -s --format='  %h %s (author: %an <%ae>)' "$sha"
    missing=$((missing + 1))
  fi
done <<<"$commits"

if [[ "$missing" -gt 0 ]]; then
  echo ""
  echo "[dco-check] $missing commit(s) missing sign-off."
  echo "[dco-check] Fix with: git commit --amend -s   (or  git rebase --signoff <base>)"
  exit 1
fi

count="$(printf '%s\n' "$commits" | grep -c .)"
echo "[dco-check] OK ($count commit(s) verified, range: $range)"
