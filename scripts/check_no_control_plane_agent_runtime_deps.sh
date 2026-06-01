#!/usr/bin/env bash
# Reject agent-framework dependencies and hard-coded runtime-family names in
# first-party control-plane source until docs/design/agent-runtime-selection.md
# records an accepted sandbox baseline and the policy is updated intentionally.

set -euo pipefail

cd "$(dirname "$0")/.."

policy_file="${OPENCLARION_AGENT_RUNTIME_POLICY_FILE:-docs/design/ci/agent-runtime-forbidden.tsv}"

if [[ ! -f "$policy_file" ]]; then
  echo "[forbidden-agent-runtime] missing policy file: $policy_file" >&2
  exit 1
fi

blocked_patterns=()
blocked_code_patterns=()
declare -A seen_policy_rows=()

line_no=0
while IFS=$'\t' read -r scope pattern extra || [[ -n "${scope:-}" ]]; do
  line_no=$((line_no + 1))
  scope=${scope%$'\r'}
  pattern=${pattern%$'\r'}

  if [[ -z "$scope" || "$scope" == \#* ]]; then
    continue
  fi
  if [[ -n "${extra:-}" || -z "${pattern:-}" ]]; then
    echo "[forbidden-agent-runtime] invalid policy row $policy_file:$line_no; expected '<manifest|code><TAB><pattern>'" >&2
    exit 1
  fi
  if [[ "$scope" =~ ^[[:space:]]|[[:space:]]$ || "$pattern" =~ ^[[:space:]]|[[:space:]]$ ]]; then
    echo "[forbidden-agent-runtime] invalid policy row $policy_file:$line_no; scope and pattern must not contain leading or trailing whitespace" >&2
    exit 1
  fi
  row_key="${scope}"$'\t'"${pattern}"
  if [[ -n "${seen_policy_rows[$row_key]:-}" ]]; then
    echo "[forbidden-agent-runtime] duplicate policy row $policy_file:$line_no: $scope	$pattern" >&2
    exit 1
  fi
  seen_policy_rows[$row_key]=1

  case "$scope" in
    manifest)
      blocked_patterns+=("$pattern")
      ;;
    code)
      blocked_code_patterns+=("$pattern")
      ;;
    *)
      echo "[forbidden-agent-runtime] invalid policy scope $policy_file:$line_no: $scope" >&2
      exit 1
      ;;
  esac
done <"$policy_file"

if [[ ${#blocked_patterns[@]} -eq 0 || ${#blocked_code_patterns[@]} -eq 0 ]]; then
  echo "[forbidden-agent-runtime] policy file must define at least one manifest and one code pattern: $policy_file" >&2
  exit 1
fi

mapfile -t manifests < <(find . \
  -path ./node_modules -prune -o \
  -path './*/node_modules' -prune -o \
  -path ./vendor -prune -o \
  -path './*/vendor' -prune -o \
  \( -name 'go.mod' -o -name 'package.json' \) -print 2>/dev/null)

source_roots=()
for root in cmd internal scripts web/src; do
  if [[ -d "$root" ]]; then
    source_roots+=("$root")
  fi
done

source_files=()
if [[ ${#source_roots[@]} -gt 0 ]]; then
  mapfile -t source_files < <(find "${source_roots[@]}" \
    -path '*/vendor' -prune -o \
    -path '*/node_modules' -prune -o \
    -path '*/internal/persistence/ent' -prune -o \
    -type f \
    \( -name '*.go' -o -name '*.sh' -o -name '*.bash' -o -name '*.js' -o -name '*.jsx' -o -name '*.ts' -o -name '*.tsx' -o -name '*.mjs' -o -name '*.cjs' \) \
    ! -name '*_test.go' \
    ! -name '*.test.js' \
    ! -name '*.test.jsx' \
    ! -name '*.test.ts' \
    ! -name '*.test.tsx' \
    ! -name '*.spec.js' \
    ! -name '*.spec.jsx' \
    ! -name '*.spec.ts' \
    ! -name '*.spec.tsx' \
    -print 2>/dev/null)
fi

failed=0

for manifest in "${manifests[@]}"; do
  for pattern in "${blocked_patterns[@]}"; do
    if grep -niF "$pattern" "$manifest"; then
      echo "[forbidden-agent-runtime] $manifest must not add agent runtime dependency '$pattern' before the runtime selection gate accepts a baseline." >&2
      failed=1
    fi
  done
done

for source_file in "${source_files[@]}"; do
  for pattern in "${blocked_code_patterns[@]}"; do
    if grep -niF "$pattern" "$source_file"; then
      echo "[forbidden-agent-runtime] $source_file must not hard-code agent runtime family '$pattern' in first-party control-plane source before the runtime selection gate accepts a baseline." >&2
      failed=1
    fi
  done
done

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Justification: docs/design/agent-runtime-selection.md keeps agent-runtime dependencies and runtime-specific logic inside candidate sandbox images until M4 proves the runtime baseline." >&2
  echo "Fix: remove the control-plane dependency or hard-coded runtime name, keep candidate names in evidence/docs/sandbox images, or update the runtime selection gate and $policy_file in the same change." >&2
  exit 1
fi

echo "[forbidden-agent-runtime] OK"
