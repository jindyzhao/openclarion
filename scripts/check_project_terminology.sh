#!/usr/bin/env bash
# Enforce canonical OpenClarion product and architecture terminology in governed docs.

set -euo pipefail

cd "$(dirname "$0")/.."

rules_file="docs/design/ci/terminology.tsv"
if [[ -L "$rules_file" ]]; then
  echo "[terminology] $rules_file must be a regular file, not a symlink" >&2
  exit 1
fi
if [[ ! -e "$rules_file" ]]; then
  echo "[terminology] missing $rules_file" >&2
  exit 1
fi
if [[ ! -f "$rules_file" ]]; then
  echo "[terminology] $rules_file must be a regular file" >&2
  exit 1
fi

match_file="$(mktemp)"
trap 'rm -f "$match_file"' EXIT

doc_roots=(
  README.md
  DEVELOPMENT_WORKFLOW.md
  CONTRIBUTING.md
  GOVERNANCE.md
  SECURITY.md
  CODE_OF_CONDUCT.md
  DCO.md
  MAINTAINERS.md
)

doc_files=()
for file in "${doc_roots[@]}"; do
  [[ -f "$file" ]] && doc_files+=("$file")
done
mapfile -t docs_markdown < <(find docs -name '*.md' -print 2>/dev/null | sort)
doc_files+=("${docs_markdown[@]}")

trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

is_allowed_file() {
  local file="$1"
  local allowed="$2"
  local entry
  local -a allowed_files=()

  [[ "$allowed" == "*" ]] && return 0
  IFS=',' read -r -a allowed_files <<< "$allowed"
  for entry in "${allowed_files[@]}"; do
    entry="$(trim_spaces "$entry")"
    if [[ "$file" == "$entry" ]]; then
      return 0
    fi
  done
  return 1
}

failed=0
while IFS=$'\t' read -r kind pattern allowed message || [[ -n "${kind:-}" ]]; do
  [[ -z "${kind:-}" || "${kind:0:1}" == "#" ]] && continue

  if [[ "$kind" != "forbidden" && "$kind" != "restricted" ]]; then
    echo "[terminology] $rules_file: unknown rule kind '$kind'." >&2
    failed=1
    continue
  fi
  if [[ -z "${pattern:-}" || -z "${allowed:-}" || -z "${message:-}" ]]; then
    echo "[terminology] $rules_file: malformed rule for pattern '$pattern'." >&2
    failed=1
    continue
  fi

  for file in "${doc_files[@]}"; do
    if [[ "$kind" == "restricted" ]] && is_allowed_file "$file" "$allowed"; then
      continue
    fi
    if grep -nEi -- "$pattern" "$file" >"$match_file"; then
      sed "s|^|$file:|" "$match_file" >&2
      echo "[terminology] $message" >&2
      failed=1
    fi
  done
done < "$rules_file"

if [[ $failed -ne 0 ]]; then
  echo "[terminology] violation(s)." >&2
  exit 1
fi

echo "[terminology] OK (${#doc_files[@]} files checked)"
