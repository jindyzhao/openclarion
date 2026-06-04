#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

RULESET="docs/design/ci/vacuum/.vacuum.yaml"
SPEC="api/openapi.yaml"

reject_symlink_ancestors() {
  local file="$1"
  local dir=""
  local part=""
  local path_part=""
  local -a parts=()

  if [[ "$file" != */* ]]; then
    return 0
  fi
  dir="${file%/*}"

  IFS='/' read -r -a parts <<< "$dir"
  for part in "${parts[@]}"; do
    if [[ -z "$part" || "$part" == "." ]]; then
      continue
    fi
    if [[ -z "$path_part" ]]; then
      path_part="$part"
    else
      path_part="$path_part/$part"
    fi
    if [[ -L "$path_part" ]]; then
      echo "[openapi-lint] $file parent directory $path_part must not be a symlink" >&2
      exit 2
    fi
  done
}

for input in "$RULESET" "$SPEC"; do
  reject_symlink_ancestors "$input"
  if [[ -L "$input" ]]; then
    echo "[openapi-lint] $input must be a regular file, not a symlink" >&2
    exit 2
  fi
  if [[ ! -e "$input" ]]; then
    echo "[openapi-lint] missing $input" >&2
    exit 2
  fi
  if [[ ! -f "$input" ]]; then
    echo "[openapi-lint] $input must be a regular file" >&2
    exit 2
  fi
done

go tool github.com/daveshanley/vacuum lint -r "$RULESET" --details --fail-severity error "$SPEC"
