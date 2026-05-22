#!/usr/bin/env bash
# Reject forbidden imports in Go source code.
#
# Background:
#   - ADR-0012 forbids third-party HTTP frameworks (Gin, Echo, Fiber).
#   - ADR-0001 keeps PostgreSQL as the single source of truth; Redis,
#     MongoDB, and external vector databases are not part of the MVP runtime.
#   - docs/design/DEPENDENCIES.md restates these constraints and references this gate.
#
# Behavior:
#   - Scans all .go files (excluding vendor/, **/testdata/, third_party/).
#   - Matches `import` lines that pull in known forbidden modules.
#   - No-op when no .go files exist yet (M0 bootstrap).
#   - Exits 1 on any match.
#
# Future relaxation requires a superseding ADR.

set -euo pipefail

cd "$(dirname "$0")/.."

# Skip silently when there are no Go sources yet.
if ! find . -path ./vendor -prune -o -path './**/testdata' -prune \
       -o -name '*.go' -print -quit 2>/dev/null | grep -q .; then
  echo "[forbidden-imports] no .go files yet; skipping (will activate when Go code lands)."
  exit 0
fi

# Forbidden module path prefixes. Use full module roots so we do not
# accidentally match unrelated packages whose names contain the substring.
forbidden=(
  'github.com/gin-gonic/gin'
  'github.com/labstack/echo'
  'github.com/gofiber/fiber'
  'github.com/go-redis/redis'
  'github.com/redis/go-redis'
  'github.com/gomodule/redigo'
  'go.mongodb.org/mongo-driver'
  'github.com/qdrant/go-client'
  'github.com/milvus-io/milvus-sdk-go'
  'github.com/weaviate/weaviate-go-client'
)

pattern="$(IFS='|'; echo "${forbidden[*]}")"

# rg with --type go honors the ignore file but we explicitly exclude
# vendor/testdata to be safe.
hits=$(rg --type go --no-heading --line-number \
        --glob '!vendor/**' \
        --glob '!**/testdata/**' \
        --glob '!third_party/**' \
        "\"($pattern)" . 2>/dev/null || true)

if [[ -n "$hits" ]]; then
  echo "[forbidden-imports] forbidden module imports detected:" >&2
  echo "$hits" >&2
  echo "" >&2
  echo "Forbidden modules: ${forbidden[*]}" >&2
  echo "Justification: ADR-0001 (no Redis/Mongo/vector DB), ADR-0012 (no Gin/Echo/Fiber)." >&2
  echo "If this constraint must change, supersede the relevant ADR first." >&2
  exit 1
fi

echo "[forbidden-imports] OK"
