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
#   - Third-party `uses:` action pins must carry a human version comment.
#   - PR-triggered workflows must declare top-level concurrency with
#     cancel-in-progress: true.
#   - `pull_request` workflows must not reference GitHub secrets. A workflow
#     that deliberately uses `pull_request_target` must carry an explicit
#     reviewer policy marker before it can be committed.
#   - Workflows must declare top-level defaults.run.shell: bash.
#   - Workflow files must use the registered `ci.yml` / `<gate>.yml`
#     naming convention and appear in docs/design/ci/README.md.
#   - Top-level workflow `name:` values must be present and unique.
#   - Workflow and job `permissions:` blocks must stay at `contents:
#     read`, unless a broader entry carries an inline `parity-allow`
#     justification.
#   - Every job must declare `runs-on:`, `permissions:`, and
#     `timeout-minutes:`. Runner labels are fixed to Ubuntu LTS labels,
#     and M0-M2 job timeouts are capped at 15 minutes.
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
CI_README="docs/design/ci/README.md"
MAX_TIMEOUT_MINUTES=15
ALLOWED_UBUNTU_RUNNER_RE='^ubuntu-(24\.04|22\.04)$'
ACTION_VERSION_COMMENT_RE='#[[:space:]]*v[0-9]+(\.[0-9]+){1,2}([[:space:]][^[:space:]].*)?[[:space:]]*$'
PULL_REQUEST_TARGET_REVIEW_POLICY_RE='^[[:space:]]*#[[:space:]]*pull-request-target-review-policy:[[:space:]]+[^[:space:]]'

require_regular_file() {
  local path="$1"
  if [[ -L "$path" ]]; then
    echo "[workflow-parity] $path must be a regular file, not a symlink." >&2
    exit 1
  fi
  if [[ ! -e "$path" ]]; then
    echo "[workflow-parity] missing $path." >&2
    exit 1
  fi
  if [[ ! -f "$path" ]]; then
    echo "[workflow-parity] $path must be a regular file." >&2
    exit 1
  fi
}

if [[ -L "$WORKFLOWS_DIR" ]]; then
  echo "[workflow-parity] $WORKFLOWS_DIR must be a directory, not a symlink." >&2
  exit 1
fi
if [[ ! -e "$WORKFLOWS_DIR" ]]; then
  echo "[workflow-parity] no $WORKFLOWS_DIR; skipping."
  exit 0
fi
if [[ ! -d "$WORKFLOWS_DIR" ]]; then
  echo "[workflow-parity] $WORKFLOWS_DIR must be a directory." >&2
  exit 1
fi
require_regular_file "$MAKEFILE"
require_regular_file "$CI_README"

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

declare -A workflow_names=()
declare -A workflow_name_files=()

workflow_has_trigger() {
  local wf="$1"
  local trigger="$2"
  awk -v trigger="$trigger" '
    function value_has_trigger(value, arr, n, i) {
      n = split(value, arr, /[^A-Za-z0-9_-]+/)
      for (i = 1; i <= n; i++) {
        if (arr[i] == trigger) return 1
      }
      return 0
    }
    {
      line = $0
      sub(/[[:space:]]+#.*$/, "", line)
    }
    line ~ /^on:[[:space:]]*/ {
      value = line
      sub(/^on:[[:space:]]*/, "", value)
      if (value == "") {
        in_on = 1
      } else if (value_has_trigger(value)) {
        found = 1
      }
      next
    }
    in_on && line ~ /^[^[:space:]][^:]*:/ {
      in_on = 0
    }
    in_on && line ~ "^  " trigger ":[[:space:]]*" {
      found = 1
    }
    in_on && line ~ "^[[:space:]]*-[[:space:]]*" trigger "[[:space:]]*$" {
      found = 1
    }
    END {
      exit(found ? 0 : 1)
    }
  ' "$wf"
}

for wf in "${workflow_files[@]}"; do
  require_regular_file "$wf"

  wf_base="$(basename "$wf")"
  if [[ "$wf_base" != "ci.yml" && ! "$wf_base" =~ ^[a-z0-9][a-z0-9-]*\.yml$ ]]; then
    printf '[workflow-parity] %s: workflow filename must be `ci.yml` or `<gate>.yml`\n' \
      "$wf" >> "$err_file"
  fi
  if ! grep -qF "| \`$wf\` |" "$CI_README"; then
    printf '[workflow-parity] %s: workflow file must be listed in %s Workflow File Registry\n' \
      "$wf" "$CI_README" >> "$err_file"
  fi
  workflow_name="$(awk '
    /^name:[[:space:]]*[^[:space:]]/ {
      value = $0
      sub(/^name:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*$/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^["'\''"]|["'\''"]$/, "", value)
      print value
      exit
    }
  ' "$wf")"
  if [[ -z "$workflow_name" ]]; then
    printf '[workflow-parity] %s: missing top-level workflow `name:`\n' \
      "$wf" >> "$err_file"
  elif [[ -v "workflow_names[$workflow_name]" ]]; then
    printf '[workflow-parity] %s: duplicate workflow name `%s` also used by %s\n' \
      "$wf" "$workflow_name" "${workflow_name_files[$workflow_name]}" >> "$err_file"
  else
    workflow_names[$workflow_name]=1
    workflow_name_files[$workflow_name]="$wf"
  fi

  awk -v wf="$wf" -v err="$err_file" '
    function leading_spaces(s, stripped) {
      stripped = s
      sub(/[^ ].*$/, "", stripped)
      return length(stripped)
    }
    function finish_permissions() {
      if (in_permissions && !has_permission_entry) {
        printf "[workflow-parity] %s:%s: permissions block is empty\n", wf, permissions_line >> err
      }
      in_permissions = 0
      permissions_indent = -1
      permissions_line = 0
      has_permission_entry = 0
    }
    /^[[:space:]]*$/ || /^[[:space:]]*#/ {
      next
    }
    {
      indent = leading_spaces($0)
      if (in_permissions && indent <= permissions_indent) {
        finish_permissions()
      }
    }
    /^[[:space:]]*permissions:[[:space:]]+[^[:space:]#]/ {
      printf "[workflow-parity] %s:%s: permissions must be a block with explicit entries\n", wf, NR >> err
      next
    }
    /^[[:space:]]*permissions:[[:space:]]*(#.*)?$/ {
      finish_permissions()
      in_permissions = 1
      permissions_indent = leading_spaces($0)
      permissions_line = NR
      has_permission_entry = 0
      next
    }
    in_permissions {
      indent = leading_spaces($0)
      if (indent != permissions_indent + 2) {
        next
      }
      line = $0
      sub(/^[[:space:]]*/, "", line)
      if (line !~ /^[A-Za-z0-9_-]+:[[:space:]]*[A-Za-z-]+/) {
        next
      }
      has_permission_entry = 1
      entry = line
      sub(/[[:space:]]+#.*$/, "", entry)
      sub(/[[:space:]]+$/, "", entry)
      if (entry == "contents: read") {
        next
      }
      if (line ~ /#[[:space:]]*parity-allow:[[:space:]][^[:space:]]/) {
        next
      }
      printf "[workflow-parity] %s:%s: permission `%s` exceeds `contents: read` without `# parity-allow: <reason>`\n", wf, NR, entry >> err
    }
    END {
      finish_permissions()
    }
  ' "$wf"

  if ! awk '
    /^defaults:[[:space:]]*$/ {
      in_defaults = 1
      next
    }
    in_defaults && /^[^[:space:]][^:]*:/ {
      in_defaults = 0
      in_run = 0
    }
    in_defaults && /^  run:[[:space:]]*$/ {
      in_run = 1
      next
    }
    in_defaults && in_run && /^    shell:[[:space:]]*bash[[:space:]]*$/ {
      found = 1
    }
    END {
      exit(found ? 0 : 1)
    }
  ' "$wf"; then
    printf '[workflow-parity] %s: missing top-level defaults.run.shell: bash\n' \
      "$wf" >> "$err_file"
  fi

  if workflow_has_trigger "$wf" "pull_request"; then
    if ! awk '
      /^concurrency:[[:space:]]*$/ {
        in_concurrency = 1
        next
      }
      in_concurrency && /^[^[:space:]][^:]*:/ {
        in_concurrency = 0
      }
      in_concurrency && /^  group:[[:space:]]*[^[:space:]]/ {
        has_group = 1
      }
      in_concurrency && /^  cancel-in-progress:[[:space:]]*true[[:space:]]*$/ {
        has_cancel = 1
      }
      END {
        exit(has_group && has_cancel ? 0 : 1)
      }
    ' "$wf"; then
      printf '[workflow-parity] %s: pull_request workflow missing top-level concurrency.group and cancel-in-progress: true\n' \
        "$wf" >> "$err_file"
    fi

    while IFS= read -r raw; do
      line_no="${raw%%:*}"
      rest="${raw#*:}"
      printf '[workflow-parity] %s:%s: pull_request workflow must not reference GitHub secrets; use pull_request_target only with an explicit reviewer policy — %s\n' \
        "$wf" "$line_no" "$rest" >> "$err_file"
    done < <(grep -nE '\$\{\{[[:space:]]*secrets[.]' "$wf" || true)
  fi

  if workflow_has_trigger "$wf" "pull_request_target"; then
    if ! grep -qE "$PULL_REQUEST_TARGET_REVIEW_POLICY_RE" "$wf"; then
      printf '[workflow-parity] %s: pull_request_target workflow must include `# pull-request-target-review-policy: <review process>` before any PR secrets boundary is accepted\n' \
        "$wf" >> "$err_file"
    fi
  fi

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
      continue
    fi
    if [[ ! "$rest" =~ $ACTION_VERSION_COMMENT_RE ]]; then
      printf '[workflow-parity] %s:%s: third-party action SHA pin must include an inline version comment like `# vX.Y.Z`\n' \
        "$wf" "$line_no" >> "$err_file"
    fi
  done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+' "$wf" || true)

  # Each job must have explicit runner, permissions, and timeout. Keep
  # this line-oriented: workflow files in this repository use two-space
  # indentation for job IDs and four-space indentation for job fields.
  awk -v wf="$wf" -v err="$err_file" \
    -v max_timeout="$MAX_TIMEOUT_MINUTES" \
    -v allowed_runner="$ALLOWED_UBUNTU_RUNNER_RE" '
    function flush_job() {
      if (job != "") {
        if (!has_runner) {
          printf "[workflow-parity] %s:%s: job `%s` missing runs-on:\n", wf, job_line, job >> err
        } else if (bad_runner != "") {
          printf "[workflow-parity] %s:%s: job `%s` uses unsupported runner `%s` (allowed: ubuntu-24.04, ubuntu-22.04)\n", wf, runner_line, job, bad_runner >> err
        }
        if (!has_permissions) {
          printf "[workflow-parity] %s:%s: job `%s` missing explicit permissions:\n", wf, job_line, job >> err
        }
        if (!has_timeout) {
          printf "[workflow-parity] %s:%s: job `%s` missing timeout-minutes:\n", wf, job_line, job >> err
        } else if (timeout_value > max_timeout) {
          printf "[workflow-parity] %s:%s: job `%s` timeout-minutes %s exceeds cap %s\n", wf, timeout_line, job, timeout_value, max_timeout >> err
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
      has_runner = 0
      has_permissions = 0
      has_timeout = 0
      bad_runner = ""
      timeout_value = 0
      next
    }
    in_jobs && job != "" && /^    runs-on:[[:space:]]*[^[:space:]]+[[:space:]]*$/ {
      has_runner = 1
      runner_line = NR
      runner = $0
      sub(/^    runs-on:[[:space:]]*/, "", runner)
      gsub(/["'\'']/, "", runner)
      sub(/[[:space:]]+#.*$/, "", runner)
      sub(/[[:space:]]+$/, "", runner)
      if (runner !~ allowed_runner) {
        bad_runner = runner
      }
      next
    }
    in_jobs && job != "" && /^    permissions:[[:space:]]*$/ {
      has_permissions = 1
      next
    }
    in_jobs && job != "" && /^    timeout-minutes:[[:space:]]*[0-9]+[[:space:]]*$/ {
      has_timeout = 1
      timeout_line = NR
      timeout_value = $0
      sub(/^    timeout-minutes:[[:space:]]*/, "", timeout_value)
      sub(/[[:space:]]+$/, "", timeout_value)
      timeout_value += 0
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
