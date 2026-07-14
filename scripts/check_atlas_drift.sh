#!/usr/bin/env bash
# scripts/check_atlas_drift.sh
#
# Atlas migration drift gate (M1-PR1).
#
# Reject any PR where the committed Atlas migrations under
# internal/persistence/migrations/ do NOT fully cover the current ent
# schema under internal/persistence/ent/schema/.
#
# Mechanism (post-2026-05-22 wrapper redesign):
#   1. Copy committed migrations into a throwaway directory.
#   2. Launch an ephemeral pgvector PostgreSQL 18 on a dedicated network.
#   3. Run `atlas migrate diff drift_check` in the pinned Atlas image
#      with the host Go toolchain mounted, talking to the dev Postgres
#      via a plain postgres:// URL.
#   4. If Atlas writes any new file (or mutates an existing migration)
#      the schema and migrations are out of sync -- fail the gate.
#
# This gate is a no-op until the first migration lands; the wiring is
# permanent from M1-PR1.
#
# In CI the workflow MUST run actions/setup-go before invoking this
# gate (the Atlas image needs a host Go toolchain mounted in to read
# ent://... schema URLs).

set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck source=lib_atlas.sh
source "$(dirname "$0")/lib_atlas.sh"

MIGRATIONS_DIR="internal/persistence/migrations"
TMP_DIR=".atlas-drift-tmp"

# No migrations yet: gate is wired but inert.
if [[ ! -d "$MIGRATIONS_DIR" ]] || ! ls "$MIGRATIONS_DIR"/*.sql >/dev/null 2>&1; then
  echo "[atlas-drift] no migrations yet; skipping (will activate after first migration lands)."
  exit 0
fi

atlas::require docker
atlas::resolve_goroot

trap 'atlas::stop_dev_pg; rm -rf "$TMP_DIR"' EXIT
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"
cp -a "$MIGRATIONS_DIR/." "$TMP_DIR/"

atlas::start_dev_pg

atlas::run migrate diff drift_check \
  --dir "file://$TMP_DIR" \
  --to "$ENT_SCHEMA_URL" \
  --dev-url "$(atlas::dev_url)"

# Compare temp dir against the real migrations dir.
new_files=()
while IFS= read -r f; do
  rel="${f#$TMP_DIR/}"
  if [[ ! -f "$MIGRATIONS_DIR/$rel" ]]; then
    new_files+=("$rel (new)")
  elif ! cmp -s "$f" "$MIGRATIONS_DIR/$rel"; then
    new_files+=("$rel (mutated)")
  fi
done < <(find "$TMP_DIR" -type f)

if (( ${#new_files[@]} > 0 )); then
  echo "" >&2
  echo "[atlas-drift] FAIL: ent schema and committed migrations are out of sync." >&2
  for f in "${new_files[@]}"; do
    echo "  - $f" >&2
  done
  echo "" >&2
  echo "Inspect the would-be migration here (temp dir, will be removed): $TMP_DIR/" >&2
  echo "Fix: run 'make atlas-migrate-diff NAME=<descriptive_name>' and commit the result." >&2
  exit 1
fi

echo "[atlas-drift] OK"
