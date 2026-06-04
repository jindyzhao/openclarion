#!/usr/bin/env bash
# Lightweight governance-memory check: shipped rows in CURRENT_STATE.md may
# mention concrete repository paths, and those path hints must exist.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

doc="docs/design/CURRENT_STATE.md"
if [[ -L "$doc" ]]; then
  echo "[doc-claims] $doc must be a regular file, not a symlink" >&2
  exit 1
fi
if [[ ! -f "$doc" ]]; then
  echo "[doc-claims] missing or non-regular $doc" >&2
  exit 1
fi

is_path_hint() {
  case "$1" in
    Makefile|go.mod|go.sum|.custom-gcl.yml|.gitleaks.toml|.golangci.yml)
      return 0
      ;;
    .github/*.*|ai-code/*.*|api/*.*|cmd/*.*|docs/*.*|internal/*.*|scripts/*.*|tools/*.*)
      return 0
      ;;
    .github/*/|ai-code/*/|api/*/|cmd/*/|docs/*/|internal/*/|scripts/*/|tools/*/)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

checked=0
err_file="$(mktemp)"
seen_file="$(mktemp)"
trap 'rm -f "$err_file" "$seen_file"' EXIT

while IFS= read -r line; do
  [[ "$line" == *"|"*"shipped"* ]] || continue
  rest="$line"
  while [[ "$rest" == *\`* ]]; do
    rest="${rest#*\`}"
    token="${rest%%\`*}"
    [[ "$rest" == "$token" ]] && break
    rest="${rest#*\`}"
    is_path_hint "$token" || continue
    grep -qxF "$token" "$seen_file" && continue
    printf '%s\n' "$token" >>"$seen_file"
    checked=$((checked + 1))
    if [[ ! -e "$token" ]]; then
      printf '[doc-claims] %s claims shipped path that does not exist: %s\n' "$doc" "$token" >>"$err_file"
    fi
  done
done <"$doc"

if [[ -s "$err_file" ]]; then
  cat "$err_file" >&2
  exit 1
fi

echo "[doc-claims] OK ($checked shipped path hints)"
