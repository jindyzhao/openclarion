#!/usr/bin/env bash
set -euo pipefail

top_level_docs=(
  README.md
  DEVELOPMENT_WORKFLOW.md
  CONTRIBUTING.md
  GOVERNANCE.md
  SECURITY.md
  CODE_OF_CONDUCT.md
  DCO.md
  MAINTAINERS.md
)
governed_paths=("${top_level_docs[@]}" docs)

failed=0
for path in "${top_level_docs[@]}"; do
  if [[ -L "$path" || ( -e "$path" && ! -f "$path" ) ]]; then
    echo "[docs-hygiene] governed documentation file must be a regular file: $path" >&2
    failed=1
  elif [[ ! -e "$path" ]]; then
    echo "[docs-hygiene] governed documentation file is missing: $path" >&2
    failed=1
  fi
done

if [[ -L docs || ( -e docs && ! -d docs ) ]]; then
  echo "[docs-hygiene] governed documentation directory must be a real directory: docs" >&2
  failed=1
elif [[ ! -e docs ]]; then
  echo "[docs-hygiene] governed documentation directory is missing: docs" >&2
  failed=1
else
  mapfile -t indirect_docs < <(find docs \( -type l -o \( ! -type f ! -type d \) \) -print 2>/dev/null | sort)
  if [[ ${#indirect_docs[@]} -gt 0 ]]; then
    echo "[docs-hygiene] governed documentation paths must be regular files or directories:" >&2
    printf '%s\n' "${indirect_docs[@]}" >&2
    failed=1
  fi
fi

if [[ $failed -ne 0 ]]; then
  exit 1
fi

han_matches="$(mktemp)"
trap 'rm -f "$han_matches"' EXIT

set +e
rg --pcre2 "\p{Han}" "${governed_paths[@]}" >"$han_matches" 2>&1
rg_status=$?
set -e

if [[ $rg_status -eq 0 ]]; then
  cat "$han_matches"
  echo "Non-English CJK characters found in governed documentation." >&2
  exit 1
elif [[ $rg_status -ne 1 ]]; then
  cat "$han_matches" >&2
  echo "[docs-hygiene] failed to scan governed documentation." >&2
  exit "$rg_status"
fi
