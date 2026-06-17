#!/usr/bin/env bash
# Enforce a package-level coverage floor for handwritten Go packages.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO_COVERAGE_MIN="${GO_COVERAGE_MIN:-40.0}"

if [[ ! "$GO_COVERAGE_MIN" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "[go-coverage] GO_COVERAGE_MIN must be a non-negative number, got $GO_COVERAGE_MIN" >&2
  exit 2
fi

mapfile -t packages < <(
  go list ./api/... ./cmd/... ./internal/... ./scripts/... |
    while IFS= read -r pkg; do
      case "$pkg" in
        */api) continue ;;
        */internal/persistence/ent|*/internal/persistence/ent/*) continue ;;
        */scripts) continue ;;
      esac
      printf '%s\n' "$pkg"
    done
)

if [[ ${#packages[@]} -eq 0 ]]; then
  echo "[go-coverage] no packages selected" >&2
  exit 1
fi

parallel_packages=()
serialized_packages=()
for pkg in "${packages[@]}"; do
  case "$pkg" in
    */internal/e2e|*/internal/orchestrator/temporal)
      serialized_packages+=("$pkg")
      ;;
    *)
      parallel_packages+=("$pkg")
      ;;
  esac
done

outputs=()
run_coverage() {
  local chunk_output
  chunk_output="$(
    go test -count=1 -cover "$@" 2>&1
  )" || {
    printf '%s\n' "$chunk_output"
    exit 1
  }
  outputs+=("$chunk_output")
}

if [[ ${#parallel_packages[@]} -gt 0 ]]; then
  run_coverage "${parallel_packages[@]}"
fi

# These packages cold-start Temporal dev servers; serialize them so coverage does
# not run competing CLI downloads or server startups on clean CI runners.
for pkg in "${serialized_packages[@]}"; do
  run_coverage "$pkg"
done

output="$(printf '%s\n' "${outputs[@]}")"
printf '%s\n' "$output"

failures=()
checked=0
while IFS= read -r line; do
  if [[ "$line" =~ coverage:[[:space:]]+([0-9]+[.][0-9]+)%[[:space:]]+of[[:space:]]+statements ]]; then
    fields=($line)
    if [[ "${fields[0]}" == "ok" ]]; then
      pkg="${fields[1]}"
    else
      pkg="${fields[0]}"
    fi
    coverage="${BASH_REMATCH[1]}"
    checked=$((checked + 1))
    if awk -v got="$coverage" -v min="$GO_COVERAGE_MIN" 'BEGIN { exit !(got + 0 < min + 0) }'; then
      failures+=("$pkg coverage ${coverage}% < ${GO_COVERAGE_MIN}%")
    fi
  fi
done <<<"$output"

if [[ $checked -eq 0 ]]; then
  echo "[go-coverage] FAIL: no package coverage lines found." >&2
  exit 1
fi

if [[ ${#failures[@]} -gt 0 ]]; then
  echo "[go-coverage] FAIL: package coverage below ${GO_COVERAGE_MIN}%." >&2
  printf '  %s\n' "${failures[@]}" >&2
  exit 1
fi

echo "[go-coverage] OK (${checked} packages, min ${GO_COVERAGE_MIN}%)"
