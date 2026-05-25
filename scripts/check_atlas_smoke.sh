#!/usr/bin/env bash
# scripts/check_atlas_smoke.sh
#
# Atlas smoke gate (M1-PR1, manual one-shot acceptance).
#
# Proves that the redesigned Atlas wrapper (host launches dev Postgres
# on a dedicated Docker network; Atlas container mounts the host Go
# toolchain; Atlas talks to dev Postgres via plain postgres://) can
# read the ent schema and emit at least one would-be migration file.
#
# This gate runs against a throwaway directory; it never touches
# internal/persistence/migrations/. It is intentionally NOT part of
# `make ci` because it requires a host Docker daemon AND network
# access to pull arigaio/atlas + postgres:18-alpine. Run it manually:
#
#   make atlas-smoke
#
# If this gate fails, do NOT edit the wrapper to "work around" it;
# capture the exact stderr and feed it back so the wrapper definition
# covers the new failure mode. See
# docs/design/database/migrations.md for the failure escalation path.

set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck source=lib_atlas.sh
source "$(dirname "$0")/lib_atlas.sh"

TMP_DIR=".atlas-smoke-tmp"

atlas::require docker

if [[ ! -d "internal/persistence/ent/schema" ]]; then
  echo "[atlas-smoke] no ent schema directory yet; nothing to smoke." >&2
  exit 1
fi

atlas::resolve_goroot

trap 'atlas::stop_dev_pg; rm -rf "$TMP_DIR"' EXIT
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

echo "[atlas-smoke] image:        $ATLAS_IMAGE"
echo "[atlas-smoke] dev pg image: $DEV_PG_IMAGE"
echo "[atlas-smoke] GOROOT_HOST:  $GOROOT_HOST"
echo "[atlas-smoke] ent schema:   $ENT_SCHEMA_URL"

atlas::start_dev_pg

echo "[atlas-smoke] running 'atlas migrate diff smoke' against ent schema..."
atlas::run migrate diff smoke \
  --dir "file://$TMP_DIR" \
  --to "$ENT_SCHEMA_URL" \
  --dev-url "$(atlas::dev_url)"

generated=$(find "$TMP_DIR" -type f | wc -l)
if (( generated == 0 )); then
  echo "" >&2
  echo "[atlas-smoke] FAIL: atlas produced no files." >&2
  exit 1
fi

echo ""
echo "[atlas-smoke] OK -- atlas produced $generated file(s) in $TMP_DIR/:"
find "$TMP_DIR" -type f -printf '  %P\n'
echo ""
echo "The smoke output above is throwaway (the temp dir is removed on exit)."
echo "Next step: 'make atlas-migrate-diff NAME=initial_schema' to commit the real first migration."
