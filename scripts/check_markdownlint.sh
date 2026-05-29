#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

CONFIG="docs/design/ci/markdownlint/.markdownlint-cli2.jsonc"
BIN="web/node_modules/.bin/markdownlint-cli2"

if [[ ! -f "$CONFIG" ]]; then
  echo "[markdownlint] missing $CONFIG" >&2
  exit 2
fi

if [[ ! -x "$BIN" ]]; then
  echo "[markdownlint] missing $BIN; run 'make frontend-install' first." >&2
  exit 2
fi

"$BIN" --config "$CONFIG"
echo "[markdownlint] OK"
