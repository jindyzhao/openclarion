#!/usr/bin/env bash
# Verify the custom analyzer module uses the same x/tools version as golangci-lint.

set -euo pipefail

cd "$(dirname "$0")/.."

binary="${1:-bin/golangci-lint}"
module_dir="${2:-tools/openclarion-linter}"

if [[ ! -x "$binary" ]]; then
  echo "[lint-version-check] FAIL: golangci-lint binary not found or not executable: $binary" >&2
  exit 1
fi

binary_tools="$(
  go version -m "$binary" |
    awk '$1 == "dep" && $2 == "golang.org/x/tools" { print $3; exit }'
)"
if [[ -z "$binary_tools" ]]; then
  echo "[lint-version-check] FAIL: unable to read golang.org/x/tools from $binary" >&2
  exit 1
fi

module_tools="$(cd "$module_dir" && go list -m -f '{{.Version}}' golang.org/x/tools)"
if [[ -z "$module_tools" ]]; then
  echo "[lint-version-check] FAIL: unable to read golang.org/x/tools from $module_dir/go.mod" >&2
  exit 1
fi

if [[ "$binary_tools" != "$module_tools" ]]; then
  echo "[lint-version-check] FAIL: golang.org/x/tools version mismatch" >&2
  echo "  golangci-lint binary: $binary_tools" >&2
  echo "  openclarion-linter:  $module_tools" >&2
  exit 1
fi

echo "[lint-version-check] OK golang.org/x/tools $module_tools"
