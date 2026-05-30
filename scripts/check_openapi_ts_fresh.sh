#!/usr/bin/env bash
# Regenerate frontend OpenAPI TypeScript types and fail if the committed file
# is stale.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/web"
OPENAPI_FILE="$ROOT_DIR/api/openapi.yaml"
PACKAGE_JSON="$WEB_DIR/package.json"
GENERATED_FILE="$WEB_DIR/src/lib/api/openapi.ts"

if [[ -L "$WEB_DIR" || ( -e "$WEB_DIR" && ! -d "$WEB_DIR" ) ]]; then
  echo "[openapi-ts-fresh] web/ must be a real directory." >&2
  exit 1
elif [[ ! -e "$WEB_DIR" ]]; then
  echo "[openapi-ts-fresh] no web/ tree; skipping."
  exit 0
fi
if [[ ! -f "$OPENAPI_FILE" || -L "$OPENAPI_FILE" ]]; then
  echo "[openapi-ts-fresh] api/openapi.yaml must be a regular file." >&2
  exit 1
fi
if [[ ! -f "$PACKAGE_JSON" || -L "$PACKAGE_JSON" ]]; then
  echo "[openapi-ts-fresh] web/package.json must be a regular file." >&2
  exit 1
fi
if [[ ! -f "$GENERATED_FILE" || -L "$GENERATED_FILE" ]]; then
  echo "[openapi-ts-fresh] web/src/lib/api/openapi.ts must be a regular file; run npm run api:generate in web/." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cp "$GENERATED_FILE" "$tmp"

(cd "$WEB_DIR" && npm run api:generate)

if [[ ! -f "$GENERATED_FILE" || -L "$GENERATED_FILE" ]]; then
  echo "[openapi-ts-fresh] web/src/lib/api/openapi.ts must remain a regular file after generation." >&2
  exit 1
fi

if ! cmp -s "$tmp" "$GENERATED_FILE"; then
  echo "[openapi-ts-fresh] FAIL: web/src/lib/api/openapi.ts is stale." >&2
  diff -u "$tmp" "$GENERATED_FILE" || true
  exit 1
fi

echo "[openapi-ts-fresh] generated TypeScript types are up-to-date."
