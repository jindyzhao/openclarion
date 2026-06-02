#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

CONFIG="docs/design/ci/markdownlint/.markdownlint-cli2.jsonc"
BIN="web/node_modules/.bin/markdownlint-cli2"

reject_symlink_ancestors() {
  local file="$1"
  local dir=""
  local part=""
  local path_part=""
  local -a parts=()

  if [[ "$file" != */* ]]; then
    return 0
  fi
  dir="${file%/*}"

  IFS='/' read -r -a parts <<< "$dir"
  for part in "${parts[@]}"; do
    if [[ -z "$part" || "$part" == "." ]]; then
      continue
    fi
    if [[ -z "$path_part" ]]; then
      path_part="$part"
    else
      path_part="$path_part/$part"
    fi
    if [[ -L "$path_part" ]]; then
      echo "[markdownlint] $file parent directory $path_part must not be a symlink" >&2
      exit 2
    fi
    if [[ -e "$path_part" && ! -d "$path_part" ]]; then
      echo "[markdownlint] $file parent directory $path_part must be a directory" >&2
      exit 2
    fi
  done
  return 0
}

reject_symlink_ancestors "$CONFIG"
if [[ -L "$CONFIG" ]]; then
  echo "[markdownlint] $CONFIG must be a regular file, not a symlink" >&2
  exit 2
fi
if [[ ! -e "$CONFIG" ]]; then
  echo "[markdownlint] missing $CONFIG" >&2
  exit 2
fi
if [[ ! -f "$CONFIG" ]]; then
  echo "[markdownlint] $CONFIG must be a regular file" >&2
  exit 2
fi

if [[ ! -x "$BIN" ]]; then
  echo "[markdownlint] missing $BIN; run 'make frontend-install' first." >&2
  exit 2
fi

"$BIN" --config "$CONFIG"
echo "[markdownlint] OK"
