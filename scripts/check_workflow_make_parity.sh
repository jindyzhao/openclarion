#!/usr/bin/env bash
# scripts/check_workflow_make_parity.sh
#
# Workflow / Makefile parity gate.
#
# Enforces docs/design/ci/README.md "Workflow Policy":
#   - GitHub Actions step `run:` lines must call a repository-owned
#     `make <target>` only; no inline pipelines, no multi-command shell
#     blocks, no `run: |` block scalars.
#   - Every referenced make target must actually be declared in
#     `Makefile` (as `.PHONY:` or a normal target rule).
#   - Third-party `uses:` actions must be pinned to a full commit SHA.
#   - Every job must declare `permissions:` and `timeout-minutes:`.
#
# This is an anti-drift gate. It is intentionally strict: first-party
# CI workflows must stay aligned with the local entrypoint so that
# `make pr` is a faithful mirror of CI.
#
# Allowed exceptions: local actions (`uses: ./...`) are not checked for
# commit-SHA pinning because they are part of the same repository tree.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

WORKFLOWS_DIR=".github/workflows"
MAKEFILE="Makefile"

if [[ ! -d "$WORKFLOWS_DIR" ]]; then
  echo "[workflow-parity] no $WORKFLOWS_DIR; skipping."
  exit 0
fi
if [[ ! -f "$MAKEFILE" ]]; then
  echo "[workflow-parity] no Makefile at repo root; cannot validate."
  exit 1
fi

# Collect declared make targets (from .PHONY lines and from
# 'name:' rule heads). The set is the union, deduplicated.
declared_targets="$(awk '
  /^\.PHONY:/ {
    sub(/^\.PHONY:[[:space:]]*/, "")
    sub(/[[:space:]]*#.*$/, "")
    n = split($0, arr, /[[:space:]]+/)
    for (i = 1; i <= n; i++) if (arr[i] != "") print arr[i]
    next
  }
  /^[a-zA-Z][a-zA-Z0-9_.-]*:/ {
    name = $1
    sub(/:.*$/, "", name)
    print name
  }
' "$MAKEFILE" | sort -u)"

if [[ -z "$declared_targets" ]]; then
  echo "[workflow-parity] no make targets parsed from Makefile; refusing to continue."
  exit 1
fi

err_file="$(mktemp)"
trap 'rm -f "$err_file"' EXIT

shopt -s nullglob
workflow_files=("$WORKFLOWS_DIR"/*.yml "$WORKFLOWS_DIR"/*.yaml)
if [[ ${#workflow_files[@]} -eq 0 ]]; then
  echo "[workflow-parity] no workflow files; skipping."
  exit 0
fi

for wf in "${workflow_files[@]}"; do
  # Reject multi-line block scalars: `run: |` or `run: >` (with optional
  # block chomping indicators like |- |+ >- >+). Account for both
  # list-item form (`  - run: |`) and bare form (`  run: |`).
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf '[workflow-parity] %s: multi-line `run:` block forbidden — %s\n' \
      "$wf" "$line" >> "$err_file"
  done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*[|>][+-]?[[:space:]]*$' "$wf" || true)

  # Reject inline run lines that are not exactly `make <target>`.
  while IFS= read -r raw; do
    # raw looks like: "17:      - run: make pr"
    line_no="${raw%%:*}"
    rest="${raw#*:}"
    # Strip leading whitespace, optional list-item dash, then `run:`.
    cmd="$(printf '%s\n' "$rest" \
      | sed -E 's/^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*//' \
      | sed -E "s/^['\"]//; s/['\"]$//")"

    # Skip empty (defensive; should not happen because of grep filter).
    [[ -z "$cmd" ]] && continue

    if [[ ! "$cmd" =~ ^make[[:space:]]+([a-zA-Z][a-zA-Z0-9_.-]*)$ ]]; then
      printf '[workflow-parity] %s:%s: `run:` must be `make <target>` only — got: %s\n' \
        "$wf" "$line_no" "$cmd" >> "$err_file"
      continue
    fi
    target="${BASH_REMATCH[1]}"
    if ! grep -qFx -- "$target" <<<"$declared_targets"; then
      printf '[workflow-parity] %s:%s: make target `%s` not declared in Makefile\n' \
        "$wf" "$line_no" "$target" >> "$err_file"
    fi
  done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]+[^|>]' "$wf" || true)

  # Third-party actions must be pinned to a full commit SHA. Mutable
  # tags (`@v4`, `@main`) and branch names are forbidden.
  while IFS= read -r raw; do
    line_no="${raw%%:*}"
    rest="${raw#*:}"
    action="$(printf '%s\n' "$rest" \
      | sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*//' \
      | sed -E 's/[[:space:]]+#.*$//' \
      | sed -E "s/^['\"]//; s/['\"]$//; s/[[:space:]]+$//")"

    [[ -z "$action" ]] && continue
    [[ "$action" == ./* ]] && continue

    if [[ ! "$action" =~ @[0-9a-fA-F]{40}$ ]]; then
      printf '[workflow-parity] %s:%s: third-party action must be pinned to a full commit SHA — got: %s\n' \
        "$wf" "$line_no" "$action" >> "$err_file"
    fi
  done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+' "$wf" || true)

  # Each job must have explicit permissions and timeout. Keep this
  # line-oriented: workflow files in this repository use two-space
  # indentation for job IDs and four-space indentation for job fields.
  awk -v wf="$wf" -v err="$err_file" '
    function flush_job() {
      if (job != "") {
        if (!has_permissions) {
          printf "[workflow-parity] %s:%s: job `%s` missing explicit permissions:\n", wf, job_line, job >> err
        }
        if (!has_timeout) {
          printf "[workflow-parity] %s:%s: job `%s` missing timeout-minutes:\n", wf, job_line, job >> err
        }
      }
    }
    /^jobs:[[:space:]]*$/ {
      in_jobs = 1
      next
    }
    in_jobs && /^[^[:space:]][^:]*:/ {
      flush_job()
      in_jobs = 0
      job = ""
      next
    }
    in_jobs && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ {
      flush_job()
      job = $1
      sub(/:$/, "", job)
      job_line = NR
      has_permissions = 0
      has_timeout = 0
      next
    }
    in_jobs && job != "" && /^    permissions:[[:space:]]*$/ {
      has_permissions = 1
      next
    }
    in_jobs && job != "" && /^    timeout-minutes:[[:space:]]*[0-9]+[[:space:]]*$/ {
      has_timeout = 1
      next
    }
    END {
      flush_job()
    }
  ' "$wf"
done

if [[ -s "$err_file" ]]; then
  cat "$err_file"
  count=$(wc -l < "$err_file")
  echo ""
  echo "[workflow-parity] $count violation(s)."
  echo "[workflow-parity] Fix: move inline shell into scripts/<name>.sh and expose via Makefile."
  exit 1
fi

echo "[workflow-parity] OK (workflows: ${#workflow_files[@]})"
