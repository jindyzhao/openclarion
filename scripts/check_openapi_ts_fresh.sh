#!/usr/bin/env bash
# Regenerate frontend OpenAPI TypeScript types and fail if the committed file
# is stale.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/web"
OPENAPI_FILE="$ROOT_DIR/api/openapi.yaml"
PACKAGE_JSON="$WEB_DIR/package.json"
GENERATED_FILE="$WEB_DIR/src/lib/api/openapi.ts"

require_regular_file() {
  local label="$1"
  local path="$2"
  if [[ -L "$path" ]]; then
    echo "[openapi-ts-fresh] $label must be a regular file, not a symlink: $path" >&2
    exit 1
  fi
  if [[ ! -f "$path" ]]; then
    echo "[openapi-ts-fresh] $label not found or not a regular file: $path" >&2
    exit 1
  fi
}

if [[ -L "$WEB_DIR" ]]; then
  echo "[openapi-ts-fresh] web/ must be a real directory." >&2
  exit 1
fi
if [[ ! -e "$WEB_DIR" ]]; then
  echo "[openapi-ts-fresh] no web/ tree; skipping."
  exit 0
fi
if [[ ! -d "$WEB_DIR" ]]; then
  echo "[openapi-ts-fresh] web/ must be a real directory." >&2
  exit 1
fi
require_regular_file "OpenAPI spec" "$OPENAPI_FILE"
require_regular_file "web/package.json" "$PACKAGE_JSON"
require_regular_file "generated TypeScript file" "$GENERATED_FILE"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cp "$GENERATED_FILE" "$tmp"

(cd "$WEB_DIR" && npm run api:generate)
require_regular_file "generated TypeScript file" "$GENERATED_FILE"

if ! cmp -s "$tmp" "$GENERATED_FILE"; then
  echo "[openapi-ts-fresh] FAIL: web/src/lib/api/openapi.ts is stale." >&2
  diff -u "$tmp" "$GENERATED_FILE" || true
  exit 1
fi

echo "[openapi-ts-fresh] generated TypeScript types are up-to-date."
