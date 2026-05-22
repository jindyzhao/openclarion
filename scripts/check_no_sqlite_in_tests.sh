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
#   - No-op when no Go test files exist.

set -euo pipefail

cd "$(dirname "$0")/.."

# Skip when no tests exist yet.
if ! find . -path ./vendor -prune -o -name '*_test.go' -print -quit 2>/dev/null | grep -q .; then
  echo "[forbidden-sqlite] no _test.go files yet; skipping."
  exit 0
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

failed=0

import_hits=$(rg --no-heading --line-number \
                --glob '*_test.go' \
                --glob '!vendor/**' \
                "\"($imp_pattern)" . 2>/dev/null || true)
if [[ -n "$import_hits" ]]; then
  echo "[forbidden-sqlite] SQLite imports in test files:" >&2
  echo "$import_hits" >&2
  failed=1
fi

# DSN-style patterns ("file::memory:", "sqlite://", "sqlite3://").
dsn_hits=$(rg --no-heading --line-number \
             --glob '*_test.go' \
             --glob '!vendor/**' \
             '("file::memory:|sqlite3?://|:memory:")' . 2>/dev/null || true)
if [[ -n "$dsn_hits" ]]; then
  echo "[forbidden-sqlite] SQLite DSN patterns in test files:" >&2
  echo "$dsn_hits" >&2
  failed=1
fi

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Justification: ADR-0001 forbids SQLite for tests; use a real" >&2
  echo "PostgreSQL instance (Docker Compose / testcontainers)." >&2
  exit 1
fi

echo "[forbidden-sqlite] OK"
