#!/usr/bin/env bash
# Scan first-party npm package-lock files with OSV-Scanner.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OSV_SCANNER_VERSION="${OSV_SCANNER_VERSION:-v1.9.2}"

require_regular_file() {
  local label="$1"
  local path="$2"
  if [[ -L "$path" ]]; then
    echo "[osv-scan] $label must be a regular file, not a symlink: $path" >&2
    exit 1
  fi
  if [[ ! -f "$path" ]]; then
    echo "[osv-scan] $label not found or not a regular file: $path" >&2
    exit 1
  fi
}

mapfile -t lockfiles < <(
  find . \
    -path './.git' -prune -o \
    -path './node_modules' -prune -o \
    -path '*/node_modules' -prune -o \
    -name package-lock.json -print |
    sed 's#^\./##' |
    sort
)

mapfile -t package_manifests < <(
  find . \
    -path './.git' -prune -o \
    -path './node_modules' -prune -o \
    -path '*/node_modules' -prune -o \
    -name package.json -print |
    sed 's#^\./##' |
    sort
)

if [[ ${#lockfiles[@]} -eq 0 ]]; then
  if [[ ${#package_manifests[@]} -gt 0 ]]; then
    echo "[osv-scan] package.json exists but no package-lock.json was found." >&2
    echo "[osv-scan] npm dependency scanning requires committed lockfiles." >&2
    exit 1
  fi
  echo "[osv-scan] OK (no npm lockfiles)"
  exit 0
fi

for manifest in "${package_manifests[@]}"; do
  require_regular_file "package manifest" "$manifest"
done

for lockfile in "${lockfiles[@]}"; do
  require_regular_file "lockfile" "$lockfile"
  echo "[osv-scan] $lockfile"
  go run "github.com/google/osv-scanner/cmd/osv-scanner@${OSV_SCANNER_VERSION}" scan \
    --lockfile="$lockfile" \
    --format=json \
    --verbosity=error \
    >/dev/null
done

echo "[osv-scan] OK (${#lockfiles[@]} lockfiles scanned)"
