#!/usr/bin/env bash
# Validate Go dependency licenses against the allowlist in docs/design/DEPENDENCIES.md.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO_LICENSES_VERSION="${GO_LICENSES_VERSION:-v1.6.0}"
POLICY_FILE="docs/design/DEPENDENCIES.md"
ALLOW_MARKER="go-license-allow:"
NON_GO_ALLOW_MARKER="go-license-non-go-allow:"
TODAY="${GO_LICENSES_REVIEW_TODAY:-$(date -u +%F)}"

reject_symlink_ancestors() {
  local file="$1"
  local dir=""
  local part=""
  local path_part=""
  local -a parts=()

  if [[ "$file" != */* ]]; then
    return 0
  fi
  dir="${file%/*}"

  IFS='/' read -r -a parts <<< "$dir"
  for part in "${parts[@]}"; do
    if [[ -z "$part" || "$part" == "." ]]; then
      continue
    fi
    if [[ -z "$path_part" ]]; then
      path_part="$part"
    else
      path_part="$path_part/$part"
    fi
    if [[ -L "$path_part" ]]; then
      echo "[go-licenses] $file parent directory $path_part must not be a symlink" >&2
      return 1
    fi
    if [[ -e "$path_part" && ! -d "$path_part" ]]; then
      echo "[go-licenses] $file parent directory $path_part must be a directory" >&2
      return 1
    fi
  done
  return 0
}

if ! reject_symlink_ancestors "$POLICY_FILE"; then
  exit 1
fi
if [[ -L "$POLICY_FILE" ]]; then
  echo "[go-licenses] $POLICY_FILE must be a regular file, not a symlink" >&2
  exit 1
fi
if [[ ! -e "$POLICY_FILE" ]]; then
  echo "[go-licenses] missing $POLICY_FILE" >&2
  exit 1
fi
if [[ ! -f "$POLICY_FILE" ]]; then
  echo "[go-licenses] $POLICY_FILE must be a regular file" >&2
  exit 1
fi

allow_line="$(grep -E "^${ALLOW_MARKER}[[:space:]]*" "$POLICY_FILE" | head -n 1 || true)"
if [[ -z "$allow_line" ]]; then
  echo "[go-licenses] $POLICY_FILE must contain '${ALLOW_MARKER} <SPDX>[,<SPDX>...]; owner: <owner>; reviewed: YYYY-MM-DD; reason: <reason>'." >&2
  exit 1
fi

validate_policy_metadata() {
  local line="$1"
  local marker="$2"
  local owner_re=';[[:space:]]*owner:[[:space:]]*[^;[:space:]][^;]*'
  local reviewed_re=';[[:space:]]*reviewed:[[:space:]]*([0-9]{4}-[0-9]{2}-[0-9]{2})([[:space:]]*;|$)'
  local reason_re=';[[:space:]]*reason:[[:space:]]*[^;[:space:]][^;]*'
  local reviewed_at=""

  if [[ ! "$line" =~ $owner_re ]]; then
    echo "[go-licenses] $POLICY_FILE ${marker} entry must include owner: <owner>." >&2
    return 1
  fi
  if [[ ! "$line" =~ $reviewed_re ]]; then
    echo "[go-licenses] $POLICY_FILE ${marker} entry must include reviewed: YYYY-MM-DD." >&2
    return 1
  fi
  reviewed_at="${BASH_REMATCH[1]}"
  if [[ "$(date -u -d "${reviewed_at}" +%F 2>/dev/null || true)" != "$reviewed_at" ]]; then
    echo "[go-licenses] $POLICY_FILE ${marker} reviewed date ${reviewed_at} is invalid." >&2
    return 1
  fi
  if [[ "$reviewed_at" > "$TODAY" ]]; then
    echo "[go-licenses] $POLICY_FILE ${marker} reviewed date ${reviewed_at} is in the future." >&2
    return 1
  fi
  if [[ ! "$line" =~ $reason_re ]]; then
    echo "[go-licenses] $POLICY_FILE ${marker} entry must include reason: <reason>." >&2
    return 1
  fi
}

validate_policy_metadata "$allow_line" "$ALLOW_MARKER"

allowed="${allow_line#${ALLOW_MARKER}}"
allowed="${allowed%%;*}"
allowed="$(printf '%s' "$allowed" | tr -d '[:space:]')"
if [[ -z "$allowed" ]]; then
  echo "[go-licenses] $POLICY_FILE ${ALLOW_MARKER} list is empty." >&2
  exit 1
fi

go_module_cache="$(go env GOMODCACHE)"
if [[ -z "$go_module_cache" || "$go_module_cache" != /* || "$go_module_cache" == "/" || "$go_module_cache" == *$'\n'* ]]; then
  echo "[go-licenses] go env GOMODCACHE must return one non-root absolute path." >&2
  exit 1
fi
go_module_cache="${go_module_cache%/}"

declare -A non_go_allow_sha256=()
while IFS= read -r non_go_line; do
  validate_policy_metadata "$non_go_line" "$NON_GO_ALLOW_MARKER"

  non_go_entry="${non_go_line#${NON_GO_ALLOW_MARKER}}"
  non_go_entry="${non_go_entry%%;*}"
  non_go_entry="$(printf '%s' "$non_go_entry" | tr -d '[:space:]')"
  IFS='|' read -r non_go_package non_go_path non_go_sha256 non_go_extra <<< "$non_go_entry"
  if [[ -z "$non_go_package" || -z "$non_go_path" || -z "$non_go_sha256" || -n "$non_go_extra" ]]; then
    echo "[go-licenses] $POLICY_FILE ${NON_GO_ALLOW_MARKER} entry must be <package>|<module-cache-relative-path>|<sha256>." >&2
    exit 1
  fi
  if [[ ! "$non_go_package" =~ ^[A-Za-z0-9._~/-]+$ ]]; then
    echo "[go-licenses] $POLICY_FILE ${NON_GO_ALLOW_MARKER} package is invalid: $non_go_package" >&2
    exit 1
  fi
  if [[ "$non_go_path" == /* || "/$non_go_path/" == *"/../"* || "$non_go_path" != *.s ]]; then
    echo "[go-licenses] $POLICY_FILE ${NON_GO_ALLOW_MARKER} path must identify a relative .s file: $non_go_path" >&2
    exit 1
  fi
  if [[ ! "$non_go_sha256" =~ ^[0-9a-f]{64}$ ]]; then
    echo "[go-licenses] $POLICY_FILE ${NON_GO_ALLOW_MARKER} SHA-256 is invalid for $non_go_package|$non_go_path" >&2
    exit 1
  fi
  non_go_key="$non_go_package|$non_go_path"
  if [[ -n "${non_go_allow_sha256[$non_go_key]+present}" ]]; then
    echo "[go-licenses] $POLICY_FILE contains duplicate ${NON_GO_ALLOW_MARKER} entry: $non_go_key" >&2
    exit 1
  fi
  non_go_allow_sha256["$non_go_key"]="$non_go_sha256"
done < <(grep -E "^${NON_GO_ALLOW_MARKER}[[:space:]]*" "$POLICY_FILE" || true)

go_licenses_tmp="$(mktemp -d)"
trap 'rm -rf "$go_licenses_tmp"' EXIT
non_go_observed="$go_licenses_tmp/non-go-observed"
: > "$non_go_observed"
go_licenses_run_index=0

validate_go_licenses_stderr() {
  local stderr_file="$1"
  local header_re="^W[0-9]{4}[[:space:]]+[0-9:.]+[[:space:]]+[0-9]+[[:space:]]+library\\.go:[0-9]+\\][[:space:]]+\"([^\"]+)\" contains non-Go code that can't be inspected for further dependencies:$"
  local current_package=""
  local current_has_file=0
  local line=""
  local relative_path=""
  local key=""
  local expected_sha256=""
  local actual_sha256=""
  local download_notice_count=0

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^go:[[:space:]]+downloading[[:space:]]+[^[:space:]]+([[:space:]]+[^[:space:]]+)*$ ]]; then
      download_notice_count=$((download_notice_count + 1))
      continue
    fi
    if [[ "$line" =~ $header_re ]]; then
      if [[ -n "$current_package" && "$current_has_file" -eq 0 ]]; then
        echo "[go-licenses] non-Go dependency notice for $current_package did not name a file" >&2
        return 1
      fi
      current_package="${BASH_REMATCH[1]}"
      current_has_file=0
      continue
    fi
    if [[ -n "$current_package" && "$line" == /* ]]; then
      if [[ "$line" != "$go_module_cache"/* ]]; then
        echo "[go-licenses] non-Go dependency file is outside the configured Go module cache: $line" >&2
        return 1
      fi
      relative_path="${line#"$go_module_cache"/}"
      key="$current_package|$relative_path"
      expected_sha256="${non_go_allow_sha256[$key]:-}"
      if [[ -z "$expected_sha256" ]]; then
        echo "[go-licenses] unreviewed non-Go dependency file: $key" >&2
        return 1
      fi
      if [[ -L "$line" || ! -f "$line" ]]; then
        echo "[go-licenses] reviewed non-Go dependency path must be a regular non-symlink file: $line" >&2
        return 1
      fi
      read -r actual_sha256 _ < <(sha256sum -- "$line")
      if [[ "$actual_sha256" != "$expected_sha256" ]]; then
        echo "[go-licenses] reviewed non-Go dependency content changed: $key" >&2
        return 1
      fi
      printf '%s\n' "$key" >> "$non_go_observed"
      current_has_file=1
      continue
    fi
    if [[ -n "$line" ]]; then
      echo "[go-licenses] unexpected tool stderr: $line" >&2
      return 1
    fi
  done < "$stderr_file"

  if [[ -n "$current_package" && "$current_has_file" -eq 0 ]]; then
    echo "[go-licenses] non-Go dependency notice for $current_package did not name a file" >&2
    return 1
  fi
  if (( download_notice_count > 0 )); then
    echo "[go-licenses] INFO ($download_notice_count Go module downloads completed)"
  fi
}

run_go_licenses() {
  local module_dir="$1"
  local ignore_prefix="$2"
  shift 2
  local stderr_file="$go_licenses_tmp/run-${go_licenses_run_index}.stderr"
  local status=0
  go_licenses_run_index=$((go_licenses_run_index + 1))

  echo "[go-licenses] $module_dir"
  (
    cd "$module_dir"
    go run "github.com/google/go-licenses@${GO_LICENSES_VERSION}" check \
      --include_tests \
      --ignore="$ignore_prefix" \
      --allowed_licenses="$allowed" \
      "$@"
  ) 2> "$stderr_file" || status=$?
  if (( status != 0 )); then
    cat "$stderr_file" >&2
    return "$status"
  fi
  validate_go_licenses_stderr "$stderr_file"
}

run_go_licenses "." "github.com/openclarion/openclarion" \
  ./cmd/openclarion ./api/... ./internal/... ./scripts/...

run_go_licenses "tools/openclarion-linter" "github.com/openclarion/openclarion/tools/openclarion-linter" \
  ./...

run_go_licenses "scripts/diagnosis_assistant_runner" "github.com/openclarion/openclarion" \
  ./...

for non_go_key in "${!non_go_allow_sha256[@]}"; do
  if ! grep -Fqx -- "$non_go_key" "$non_go_observed"; then
    echo "[go-licenses] stale ${NON_GO_ALLOW_MARKER} entry was not reported by the pinned tool: $non_go_key" >&2
    exit 1
  fi
done

if (( ${#non_go_allow_sha256[@]} > 0 )); then
  echo "[go-licenses] INFO (${#non_go_allow_sha256[@]} hash-pinned assembly files audited)"
fi

echo "[go-licenses] OK (allowed: $allowed)"
