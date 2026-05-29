#!/usr/bin/env bash
# Regenerate frontend OpenAPI TypeScript types and fail if the committed file
# is stale.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/web"
GENERATED_FILE="$WEB_DIR/src/lib/api/openapi.ts"

if [[ ! -d "$WEB_DIR" ]]; then
  echo "[openapi-ts-fresh] no web/ tree; skipping."
  exit 0
fi
if [[ ! -f "$WEB_DIR/package.json" ]]; then
  echo "[openapi-ts-fresh] missing web/package.json" >&2
  exit 1
fi
if [[ ! -f "$GENERATED_FILE" ]]; then
  echo "[openapi-ts-fresh] missing $GENERATED_FILE; run npm run api:generate in web/." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cp "$GENERATED_FILE" "$tmp"

(cd "$WEB_DIR" && npm run api:generate)

if ! cmp -s "$tmp" "$GENERATED_FILE"; then
  echo "[openapi-ts-fresh] FAIL: web/src/lib/api/openapi.ts is stale." >&2
  diff -u "$tmp" "$GENERATED_FILE" || true
  exit 1
fi

echo "[openapi-ts-fresh] generated TypeScript types are up-to-date."
