#!/usr/bin/env bash
# Compare the current OpenAPI contract against a base revision and report
# breaking changes. During W4 rollout this gate is soft-fail only until the
# audited sunset date; invalid sunset configuration is always fail-closed.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

SPEC_PATH="${OPENAPI_SPEC_PATH:-api/openapi.yaml}"
SOFT_FAIL_UNTIL="${OPENAPI_BREAKING_SOFT_FAIL_UNTIL:-2026-06-10}"
OASDIFF_VERSION="${OASDIFF_VERSION:-v1.11.7}"

require_regular_file() {
  local label="$1"
  local path="$2"
  if [[ -L "$path" ]]; then
    echo "[openapi-breaking] $label must be a regular file, not a symlink: $path" >&2
    exit 1
  fi
  if [[ ! -f "$path" ]]; then
    echo "[openapi-breaking] $label not found or not a regular file: $path" >&2
    exit 1
  fi
}

if [[ ! "$SOFT_FAIL_UNTIL" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "[openapi-breaking] SOFT_FAIL_UNTIL must be YYYY-MM-DD, got: ${SOFT_FAIL_UNTIL:-<empty>}" >&2
  exit 1
fi
if ! sunset_ts="$(date -u -d "$SOFT_FAIL_UNTIL" +%s 2>/dev/null)"; then
  echo "[openapi-breaking] SOFT_FAIL_UNTIL is not a valid date: $SOFT_FAIL_UNTIL" >&2
  exit 1
fi
today="${OPENAPI_BREAKING_TODAY:-$(date -u +%F)}"
if [[ ! "$today" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || ! today_ts="$(date -u -d "$today" +%s 2>/dev/null)"; then
  echo "[openapi-breaking] today is not a valid YYYY-MM-DD date: $today" >&2
  exit 1
fi

require_regular_file "current spec" "$SPEC_PATH"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
base_spec="$tmpdir/base-openapi.yaml"

if [[ -n "${OPENAPI_BASE_SPEC:-}" ]]; then
  require_regular_file "OPENAPI_BASE_SPEC" "$OPENAPI_BASE_SPEC"
  cp "$OPENAPI_BASE_SPEC" "$base_spec"
else
  candidates=()
  if [[ -n "${OPENAPI_BASE_REF:-}" ]]; then
    candidates+=("$OPENAPI_BASE_REF" "origin/$OPENAPI_BASE_REF")
  fi
  candidates+=("HEAD")

  for ref in "${candidates[@]}"; do
    if git cat-file -e "$ref:$SPEC_PATH" 2>/dev/null; then
      mode="$(git ls-tree "$ref" -- "$SPEC_PATH" | awk '{ print $1; exit }')"
      case "$mode" in
        100644|100755) ;;
        *)
          echo "[openapi-breaking] base spec at $ref:$SPEC_PATH must be a regular file blob, got git mode ${mode:-<missing>}" >&2
          exit 1
          ;;
      esac
      git show "$ref:$SPEC_PATH" >"$base_spec"
      echo "[openapi-breaking] base: $ref:$SPEC_PATH"
      break
    fi
  done
fi

if [[ ! -s "$base_spec" ]]; then
  echo "[openapi-breaking] could not resolve a base OpenAPI spec." >&2
  echo "[openapi-breaking] Set OPENAPI_BASE_SPEC=<file> or OPENAPI_BASE_REF=<git-ref>." >&2
  exit 1
fi

set +e
output="$(go run "github.com/oasdiff/oasdiff@${OASDIFF_VERSION}" breaking "$base_spec" "$SPEC_PATH" -f text 2>&1)"
status=$?
set -e

if [[ $status -eq 0 ]]; then
  echo "[openapi-breaking] OK"
  if [[ -n "$output" ]]; then
    printf '%s\n' "$output"
  fi
  exit 0
fi

printf '%s\n' "$output"
if (( today_ts < sunset_ts )); then
  echo "[openapi-breaking] WARNING: breaking-change gate is soft-fail until $SOFT_FAIL_UNTIL (owner: CI maintainers)." >&2
  exit 0
fi

echo "[openapi-breaking] FAIL: breaking OpenAPI changes detected after soft-fail sunset $SOFT_FAIL_UNTIL." >&2
exit "$status"
