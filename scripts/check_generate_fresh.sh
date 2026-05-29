#!/usr/bin/env bash
set -euo pipefail

# Verify that repository generators are both fresh and idempotent.
#
# The check snapshots tracked plus non-ignored untracked files before and after
# generation, so it works in a dirty local worktree as long as generators do not
# introduce additional changes.

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

snapshot_state() {
  local output="$1"

  git ls-files -z -c -o --exclude-standard \
    | sort -z \
    | while IFS= read -r -d '' path; do
        if [[ -f "$path" || -L "$path" ]]; then
          hash="$(sha256sum -- "$path" | awk '{print $1}')"
          printf '%s  %s\n' "$hash" "$path"
        fi
      done >"$output"
}

show_state_diff() {
  local before="$1"
  local after="$2"

  diff -u "$before" "$after" | sed -n '1,160p' >&2 || true
}

before="$tmp_dir/before.txt"
after_first="$tmp_dir/after-first.txt"
after_second="$tmp_dir/after-second.txt"

snapshot_state "$before"

make --no-print-directory generate
snapshot_state "$after_first"

if ! cmp -s "$before" "$after_first"; then
  echo "[generate-fresh] FAIL: make generate changed repository files." >&2
  echo "Fix: run 'make generate' and commit the generated outputs." >&2
  show_state_diff "$before" "$after_first"
  exit 1
fi

make --no-print-directory generate
snapshot_state "$after_second"

if ! cmp -s "$after_first" "$after_second"; then
  echo "[generate-fresh] FAIL: make generate is not idempotent." >&2
  echo "Fix: make generator outputs deterministic across repeated runs." >&2
  show_state_diff "$after_first" "$after_second"
  exit 1
fi

echo "[generate-fresh] OK (make generate is fresh and idempotent)"
