#!/usr/bin/env bash
# Reject `latest` version pins in dependency manifests.
#
# Background:
#   - docs/design/DEPENDENCIES.md: "CI must reject the literal string
#     `latest` in `go.mod` and `package.json` for first-party dependencies."
#   - ADR-0007: oapi-codegen-exp must be pinned by commit hash.
#
# Behavior:
#   - go.mod: rejects any `module v0.0.0-...latest` style or literal `latest`.
#     Go modules do not actually allow `latest` in go.mod (it gets resolved
#     at `go get` time), so this is a defense-in-depth check on text content.
#   - package.json (and pnpm/yarn lockfile manifests): rejects any value
#     that is exactly the string "latest" in dependencies / devDependencies /
#     peerDependencies / optionalDependencies.
#   - No-op when neither file exists yet.

set -euo pipefail

cd "$(dirname "$0")/.."

failed=0

# -------- go.mod --------
if [[ -f go.mod ]]; then
  if grep -nE '\blatest\b' go.mod; then
    echo "[forbidden-latest] go.mod must not reference 'latest'." >&2
    failed=1
  fi
fi

# -------- go.sum (defensive) --------
# go.sum does not include the literal 'latest', but a corrupt manual edit
# could; flag any occurrence.
if [[ -f go.sum ]]; then
  if grep -nE '\blatest\b' go.sum; then
    echo "[forbidden-latest] go.sum must not reference 'latest'." >&2
    failed=1
  fi
fi

# -------- package.json (any depth, excluding node_modules) --------
mapfile -t pkg_files < <(find . \
  -path ./node_modules -prune -o \
  -path ./vendor -prune -o \
  -path './**/node_modules' -prune -o \
  -name 'package.json' -print 2>/dev/null)

for pkg in "${pkg_files[@]}"; do
  # Match `"some-dep": "latest"` (allowing whitespace variants).
  if grep -nE '"[^"]+"[[:space:]]*:[[:space:]]*"latest"' "$pkg"; then
    echo "[forbidden-latest] $pkg must not pin any dependency to 'latest'." >&2
    failed=1
  fi
done

if [[ ${#pkg_files[@]} -eq 0 && ! -f go.mod ]]; then
  echo "[forbidden-latest] no go.mod or package.json yet; skipping."
  exit 0
fi

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Justification: docs/design/DEPENDENCIES.md forbids 'latest' for first-party deps." >&2
  echo "Fix: pin to a concrete version (semver tag or commit hash)." >&2
  exit 1
fi

echo "[forbidden-latest] OK"
