#!/usr/bin/env bash
# scripts/run_secrets_scan.sh
#
# Secret scanning gate using gitleaks.
#
# Runs two gitleaks passes with the repository .gitleaks.toml config:
#
#   1. git history (`gitleaks git .`)
#   2. current source snapshot for tracked files and untracked, non-ignored files
#
# The snapshot pass intentionally uses `git ls-files --exclude-standard` so
# generated artifacts and private local workspaces ignored by git do not become
# gate inputs.
#
# This script uses `go run` with a pinned version so no pre-installed
# binary is needed. The version is controlled by GITLEAKS_VERSION.
#
# Exit codes:
#   0 — no secrets detected
#   1 — one or more secrets detected (or tool error)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Pinned version — update deliberately, not silently.
GITLEAKS_VERSION="${GITLEAKS_VERSION:-v8.28.0}"

CONFIG_FILE=".gitleaks.toml"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "[secrets-scan] FAIL: $CONFIG_FILE not found at repository root."
  exit 1
fi

echo "[secrets-scan] running gitleaks ${GITLEAKS_VERSION} (git history)..."

go run "github.com/zricethezav/gitleaks/v8@${GITLEAKS_VERSION}" \
  git \
  . \
  --config="$CONFIG_FILE" \
  --redact=100 \
  --no-banner

SNAPSHOT_DIR="$(mktemp -d)"
trap 'rm -rf "$SNAPSHOT_DIR"' EXIT

while IFS= read -r -d '' path; do
  [[ -f "$path" ]] || continue
  mkdir -p "$SNAPSHOT_DIR/$(dirname "$path")"
  cp -p "$path" "$SNAPSHOT_DIR/$path"
done < <(git ls-files -z --cached --others --exclude-standard)

echo "[secrets-scan] running gitleaks ${GITLEAKS_VERSION} (current source snapshot)..."

go run "github.com/zricethezav/gitleaks/v8@${GITLEAKS_VERSION}" \
  dir \
  "$SNAPSHOT_DIR" \
  --config="$CONFIG_FILE" \
  --redact=100 \
  --no-banner

echo "[secrets-scan] OK — no secrets detected."
