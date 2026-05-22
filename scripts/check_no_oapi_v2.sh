#!/usr/bin/env bash
# Reject oapi-codegen v2 usage and the OpenAPI 3.0 compatibility bridge.
#
# Background:
#   - ADR-0007: OpenClarion uses oapi-codegen-exp (V3, OpenAPI 3.1 native).
#     The legacy v2 generator and the `openapi.compat.yaml` 3.0 bridge are
#     downgrade fallbacks that require a superseding ADR to activate.
#
# Behavior:
#   - Rejects imports of `github.com/oapi-codegen/oapi-codegen/v2` in code.
#   - Rejects the string `oapi-codegen/v2` in go.mod and tools.go.
#   - Rejects existence of api/openapi.compat.yaml.
#   - No-op when no targets exist yet.

set -euo pipefail

cd "$(dirname "$0")/.."

failed=0

# -------- compat bridge file must not exist --------
if [[ -f api/openapi.compat.yaml ]]; then
  echo "[forbidden-oapi-v2] api/openapi.compat.yaml exists." >&2
  echo "  ADR-0007 requires native OpenAPI 3.1; the 3.0 compat bridge is a" >&2
  echo "  downgrade fallback gated by a superseding ADR." >&2
  failed=1
fi

# -------- go.mod must not pull in v2 generator --------
if [[ -f go.mod ]]; then
  if grep -nE 'oapi-codegen/oapi-codegen/v2' go.mod; then
    echo "[forbidden-oapi-v2] go.mod references oapi-codegen v2." >&2
    failed=1
  fi
fi

# -------- Go sources / tools.go must not import v2 --------
hits=$(rg --type go --no-heading --line-number \
        --glob '!vendor/**' \
        --glob '!**/testdata/**' \
        '"github.com/oapi-codegen/oapi-codegen/v2' . 2>/dev/null || true)
if [[ -n "$hits" ]]; then
  echo "[forbidden-oapi-v2] forbidden v2 imports detected:" >&2
  echo "$hits" >&2
  failed=1
fi

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Fix: use oapi-codegen-exp (V3) per ADR-0007." >&2
  exit 1
fi

echo "[forbidden-oapi-v2] OK"
