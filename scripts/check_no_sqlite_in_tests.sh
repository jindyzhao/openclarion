#!/usr/bin/env bash
# Reject SQLite usage in Go test files.
#
# Background:
#   - ADR-0001: PostgreSQL is the single source of truth. Tests must run
#     against the same engine to catch dialect-level surprises (JSONB
#     operators, advisory locks, partial indexes, etc.).
#   - docs/design/DEPENDENCIES.md and docs/design/CHECKLIST.md restate this constraint.
#
# Behavior:
#   - Scans `*_test.go` files for SQLite drivers and DSN patterns.
#   - Rejects symlinked or otherwise non-regular `*_test.go` entries before
#     scanning, so the grep-based gate cannot be bypassed by indirection.
#   - No-op when no Go test files exist.

set -euo pipefail

cd "$(dirname "$0")/.."

mapfile -t test_paths < <(find . \
  -path './.git' -prune -o \
  -path './vendor' -prune -o \
  -name '*_test.go' -print 2>/dev/null | sort)

# Skip when no tests exist yet.
if [[ ${#test_paths[@]} -eq 0 ]]; then
  echo "[forbidden-sqlite] no _test.go files yet; skipping."
  exit 0
fi

failed=0
regular_test_paths=()
non_regular_test_paths=()
for path in "${test_paths[@]}"; do
  if [[ -f "$path" && ! -L "$path" ]]; then
    regular_test_paths+=("$path")
  else
    non_regular_test_paths+=("$path")
  fi
done

if [[ ${#non_regular_test_paths[@]} -gt 0 ]]; then
  echo "[forbidden-sqlite] Go test file paths must be regular files:" >&2
  printf '%s\n' "${non_regular_test_paths[@]}" >&2
  failed=1
fi

# Forbidden SQLite import paths and DSN markers.
sqlite_imports=(
  'github.com/mattn/go-sqlite3'
  'modernc.org/sqlite'
  'github.com/glebarez/sqlite'
  'github.com/glebarez/go-sqlite'
  'github.com/ncruces/go-sqlite3'
)
imp_pattern="$(IFS='|'; echo "${sqlite_imports[*]}")"

if [[ ${#regular_test_paths[@]} -gt 0 ]]; then
  import_hits=$(rg --with-filename --no-heading --line-number \
                  "\"($imp_pattern)" "${regular_test_paths[@]}" 2>/dev/null || true)
  if [[ -n "$import_hits" ]]; then
    echo "[forbidden-sqlite] SQLite imports in test files:" >&2
    echo "$import_hits" >&2
    failed=1
  fi

  # DSN-style patterns ("file::memory:", "sqlite://", "sqlite3://").
  dsn_hits=$(rg --with-filename --no-heading --line-number \
               '("file::memory:|sqlite3?://|:memory:")' "${regular_test_paths[@]}" 2>/dev/null || true)
  if [[ -n "$dsn_hits" ]]; then
    echo "[forbidden-sqlite] SQLite DSN patterns in test files:" >&2
    echo "$dsn_hits" >&2
    failed=1
  fi
fi

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Justification: ADR-0001 forbids SQLite for tests; use a real" >&2
  echo "PostgreSQL instance (Docker Compose / testcontainers)." >&2
  exit 1
fi

echo "[forbidden-sqlite] OK"
