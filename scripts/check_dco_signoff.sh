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
# `Signed-off-by:` trailer whose email matches the commit author email, or if
# any commit message includes AI tool branding that would weaken authorship
# accountability.
# See DCO.md for the policy and docs/design/DEPENDENCIES.md for the
# dependency-pinning policy this gate is paired with.

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
single_head=0
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
    range='HEAD'
    single_head=1
  fi
fi

if [[ "$single_head" -eq 1 ]]; then
  commits="$(git rev-list --no-merges -n 1 HEAD 2>/dev/null || true)"
else
  commits="$(git rev-list --no-merges "$range" 2>/dev/null || true)"
fi

if [[ -z "$commits" ]]; then
  echo "[dco-check] no commits to validate (range: $range)."
  exit 0
fi

missing=0
mismatched=0
branded=0
signoff_re='^Signed-off-by:[[:space:]]+.*<([^<>[:space:]]+@[^<>[:space:]]+)>[[:space:]]*$'
ai_branding_re='(^|[[:space:]])(claude|chatgpt|gpt-[0-9][[:alnum:].-]*|codex|github[[:space:]-]*copilot|copilot|gemini)([[:space:][:punct:]]|$)'
while IFS= read -r sha; do
  [[ -z "$sha" ]] && continue
  msg="$(git show -s --format=%B "$sha")"
  author_email="$(git show -s --format=%ae "$sha")"
  signoff_emails=()
  branded_lines=()
  shopt -s nocasematch
  while IFS= read -r line; do
    if [[ "$line" =~ $signoff_re ]]; then
      signoff_emails+=("${BASH_REMATCH[1]}")
    fi
    if [[ "$line" =~ ^Generated-by: || "$line" =~ ^Co-authored-by:.*$ai_branding_re || "$line" =~ $ai_branding_re ]]; then
      branded_lines+=("$line")
    fi
  done <<<"$msg"
  shopt -u nocasematch

  if [[ "${#branded_lines[@]}" -gt 0 ]]; then
    if [[ "$branded" -eq 0 ]]; then
      echo "[dco-check] commit message contains AI tool branding:"
    fi
    git show -s --format='  %h %s (author: %an <%ae>)' "$sha"
    for branded_line in "${branded_lines[@]}"; do
      printf '    %s\n' "$branded_line"
    done
    branded=$((branded + 1))
  fi

  if [[ "${#signoff_emails[@]}" -eq 0 ]]; then
    if [[ "$missing" -eq 0 ]]; then
      echo "[dco-check] missing Signed-off-by trailer:"
    fi
    git show -s --format='  %h %s (author: %an <%ae>)' "$sha"
    missing=$((missing + 1))
    continue
  fi

  author_email_lc="${author_email,,}"
  matched=0
  for email in "${signoff_emails[@]}"; do
    if [[ "${email,,}" == "$author_email_lc" ]]; then
      matched=1
      break
    fi
  done
  if [[ "$matched" -eq 0 ]]; then
    if [[ "$mismatched" -eq 0 ]]; then
      echo "[dco-check] Signed-off-by email does not match commit author email:"
    fi
    git show -s --format='  %h %s (author: %an <%ae>)' "$sha"
    printf '    sign-off email(s): %s\n' "${signoff_emails[*]}"
    mismatched=$((mismatched + 1))
  fi
done <<<"$commits"

if [[ "$missing" -gt 0 || "$mismatched" -gt 0 || "$branded" -gt 0 ]]; then
  echo ""
  if [[ "$missing" -gt 0 ]]; then
    echo "[dco-check] $missing commit(s) missing sign-off."
  fi
  if [[ "$mismatched" -gt 0 ]]; then
    echo "[dco-check] $mismatched commit(s) have sign-off emails that do not match the author email."
  fi
  if [[ "$branded" -gt 0 ]]; then
    echo "[dco-check] $branded commit(s) contain AI tool branding."
  fi
  echo "[dco-check] Fix with: git commit --amend -s   (or  git rebase --signoff <base>)"
  exit 1
fi

count="$(printf '%s\n' "$commits" | grep -c .)"
echo "[dco-check] OK ($count commit(s) verified, range: $range)"
