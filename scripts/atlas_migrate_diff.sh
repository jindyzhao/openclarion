#!/usr/bin/env bash
# scripts/atlas_migrate_diff.sh
#
# Generate a new Atlas migration from the live ent schema diff.
#
# Usage:
#   bash scripts/atlas_migrate_diff.sh <migration_name>
#
# Wraps the same dev-Postgres + Atlas wrapper used by the smoke and
# drift gates (see scripts/lib_atlas.sh). The resulting migration SQL
# files plus atlas.sum are written to internal/persistence/migrations/
# and are owned by the invoking user (NOT root) thanks to the
# --user "$(id -u):$(id -g)" flag in the wrapper.

set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck source=lib_atlas.sh
source "$(dirname "$0")/lib_atlas.sh"

if [[ $# -ne 1 || -z "${1:-}" ]]; then
  echo "usage: $0 <migration_name>" >&2
  exit 2
fi
NAME="$1"

MIGRATIONS_DIR="internal/persistence/migrations"

atlas::require docker
atlas::resolve_goroot

mkdir -p "$MIGRATIONS_DIR"

trap 'atlas::stop_dev_pg' EXIT
atlas::start_dev_pg

atlas::run migrate diff "$NAME" \
  --dir "file://$MIGRATIONS_DIR" \
  --to "$ENT_SCHEMA_URL" \
  --dev-url "$(atlas::dev_url)"

echo "[atlas-migrate-diff] OK -- review and commit files under $MIGRATIONS_DIR/"
