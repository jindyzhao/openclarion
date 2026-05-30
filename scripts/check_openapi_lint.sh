#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

RULESET="docs/design/ci/vacuum/.vacuum.yaml"
SPEC="api/openapi.yaml"

for input in "$RULESET" "$SPEC"; do
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
