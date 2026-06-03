#!/usr/bin/env bash
# Validate Go dependency licenses against the allowlist in docs/design/DEPENDENCIES.md.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO_LICENSES_VERSION="${GO_LICENSES_VERSION:-v1.6.0}"
POLICY_FILE="docs/design/DEPENDENCIES.md"
ALLOW_MARKER="go-license-allow:"
TODAY="${GO_LICENSES_REVIEW_TODAY:-$(date -u +%F)}"

if [[ ! -f "$POLICY_FILE" ]]; then
  echo "[go-licenses] missing $POLICY_FILE" >&2
  exit 1
fi

allow_line="$(grep -E "^${ALLOW_MARKER}[[:space:]]*" "$POLICY_FILE" | head -n 1 || true)"
if [[ -z "$allow_line" ]]; then
  echo "[go-licenses] $POLICY_FILE must contain '${ALLOW_MARKER} <SPDX>[,<SPDX>...]; owner: <owner>; reviewed: YYYY-MM-DD; reason: <reason>'." >&2
  exit 1
fi

owner_re=';[[:space:]]*owner:[[:space:]]*[^;[:space:]][^;]*'
reviewed_re=';[[:space:]]*reviewed:[[:space:]]*([0-9]{4}-[0-9]{2}-[0-9]{2})([[:space:]]*;|$)'
reason_re=';[[:space:]]*reason:[[:space:]]*[^;[:space:]][^;]*'
if [[ ! "$allow_line" =~ $owner_re ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} entry must include owner: <owner>." >&2
  exit 1
fi
if [[ ! "$allow_line" =~ $reviewed_re ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} entry must include reviewed: YYYY-MM-DD." >&2
  exit 1
fi
reviewed_at="${BASH_REMATCH[1]}"
if [[ "$(date -u -d "${reviewed_at}" +%F 2>/dev/null || true)" != "$reviewed_at" ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} reviewed date ${reviewed_at} is invalid." >&2
  exit 1
fi
if [[ "$reviewed_at" > "$TODAY" ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} reviewed date ${reviewed_at} is in the future." >&2
  exit 1
fi
if [[ ! "$allow_line" =~ $reason_re ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} entry must include reason: <reason>." >&2
  exit 1
fi

allowed="${allow_line#${ALLOW_MARKER}}"
allowed="${allowed%%;*}"
allowed="$(printf '%s' "$allowed" | tr -d '[:space:]')"
if [[ -z "$allowed" ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} list is empty." >&2
  exit 1
fi

run_go_licenses() {
  local module_dir="$1"
  local ignore_prefix="$2"
  shift 2

  echo "[go-licenses] $module_dir"
  (
    cd "$module_dir"
    go run "github.com/google/go-licenses@${GO_LICENSES_VERSION}" check \
      --include_tests \
      --ignore="$ignore_prefix" \
      --allowed_licenses="$allowed" \
      "$@"
  )
}

run_go_licenses "." "github.com/openclarion/openclarion" \
  ./cmd/openclarion ./api/... ./internal/... ./scripts/...

run_go_licenses "tools/openclarion-linter" "github.com/openclarion/openclarion/tools/openclarion-linter" \
  ./...

echo "[go-licenses] OK (allowed: $allowed)"
