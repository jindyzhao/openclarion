#!/usr/bin/env bash
# Run a live Alertmanager webhook -> auto_room -> AI notification proof through
# the OpenClarion HTTP API.
#
# This is intentionally NOT part of make ci: it requires a running backend,
# worker, Temporal, LLM provider, an enabled Alertmanager source profile, an
# enabled auto_room report workflow policy, and a configured notification
# channel with report, diagnosis_consultation, and diagnosis_close scopes.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_alertmanager_auto_diagnosis_live_smoke.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_API_BASE_URL
  ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID
    or OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID

Optional env:
  ALERTMANAGER_WEBHOOK_BEARER_TOKEN
  OPENCLARION_ALERTMANAGER_WEBHOOK_BEARER_TOKEN
  ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID
  ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND (default: assistant_message)
  ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS
    default: assistant_message
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ROOM_TIMEOUT
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_POLL_INTERVAL
  ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME

PATH must be a regular, non-symlink file owned by the current user, with no
group/other permissions, and outside the OpenClarion repository or under the
repo-local ignored .openclarion-private/ directory.

ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR and
ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT must live under the repo-local
ignored .openclarion-private/ directory.
EOF
}

while (($# > 0)); do
  case "$1" in
    --env-file)
      if (($# < 2)); then
        usage
        exit 2
      fi
      env_file="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

fail() {
  printf '[alertmanager-auto-diagnosis-live-smoke] %s\n' "$1" >&2
  exit 2
}

positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

repo_private_output_path() {
  local label="$1"
  local path="$2"

  case "$path" in
    "$ROOT_DIR"/.openclarion-private/*) ;;
    *)
      fail "$label must live under repo-local ignored .openclarion-private/"
      ;;
  esac
}

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "alertmanager-auto-diagnosis-live-smoke" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env OPENCLARION_LIVE_API_BASE_URL
if [[ -z "${ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID:-}${OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID:-}" ]]; then
  missing+=("ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID or OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID")
fi

if ((${#missing[@]} > 0)); then
  printf '[alertmanager-auto-diagnosis-live-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

source_profile_id="${ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID:-${OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID:-}}"
if [[ -n "${ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID:-}" && -n "${OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID:-}" &&
      "${ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID}" != "${OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID}" ]]; then
  fail "ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID and OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID must match when both are set"
fi
if ! positive_integer "$source_profile_id"; then
  fail "Alertmanager webhook source profile ID must be a positive integer"
fi

bearer_token="${ALERTMANAGER_WEBHOOK_BEARER_TOKEN:-${OPENCLARION_ALERTMANAGER_WEBHOOK_BEARER_TOKEN:-}}"
if [[ -n "$bearer_token" ]]; then
  bearer_token="${bearer_token#"${bearer_token%%[![:space:]]*}"}"
  bearer_token="${bearer_token%"${bearer_token##*[![:space:]]}"}"
  if [[ "$bearer_token" == Bearer\ * ]]; then
    bearer_token="${bearer_token#Bearer }"
  fi
  if [[ -z "$bearer_token" || "$bearer_token" =~ [[:space:]] ]]; then
    fail "Alertmanager webhook bearer token must be a single bearer token or Bearer header"
  fi
fi

expected_channel_profile_id="${ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID:-}"
if [[ -n "$expected_channel_profile_id" ]] && ! positive_integer "$expected_channel_profile_id"; then
  fail "Expected notification channel profile ID must be a positive integer"
fi

expected_content_kind="${ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND:-assistant_message}"
if [[ -n "$expected_content_kind" ]]; then
  expected_content_kind="${expected_content_kind#"${expected_content_kind%%[![:space:]]*}"}"
  expected_content_kind="${expected_content_kind%"${expected_content_kind##*[![:space:]]}"}"
  expected_content_kind="${expected_content_kind,,}"
  case "$expected_content_kind" in
    assistant_message|final_conclusion) ;;
    *) fail "Expected AI notification content kind must be assistant_message or final_conclusion" ;;
  esac
fi

required_content_kinds_raw="${ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS:-assistant_message}"
required_content_kinds=""
if [[ -n "$required_content_kinds_raw" ]]; then
  required_content_kinds_raw="${required_content_kinds_raw#"${required_content_kinds_raw%%[![:space:]]*}"}"
  required_content_kinds_raw="${required_content_kinds_raw%"${required_content_kinds_raw##*[![:space:]]}"}"
  if [[ -n "$required_content_kinds_raw" ]]; then
    if [[ "$required_content_kinds_raw" == ,* || "$required_content_kinds_raw" == *, || "$required_content_kinds_raw" == *,,* ]]; then
      fail "Required AI notification content kinds must not contain empty values"
    fi
    IFS=',' read -r -a required_content_kind_parts <<<"$required_content_kinds_raw"
    for part in "${required_content_kind_parts[@]}"; do
      kind="${part#"${part%%[![:space:]]*}"}"
      kind="${kind%"${kind##*[![:space:]]}"}"
      kind="${kind,,}"
      if [[ -z "$kind" ]]; then
        fail "Required AI notification content kinds must not contain empty values"
      fi
      case "$kind" in
        assistant_message|final_conclusion) ;;
        *) fail "Required AI notification content kinds must be assistant_message or final_conclusion" ;;
      esac
      if [[ ",$required_content_kinds," == *",$kind,"* ]]; then
        fail "Required AI notification content kinds must not contain duplicate values"
      fi
      required_content_kinds="${required_content_kinds:+$required_content_kinds,}$kind"
    done
  fi
fi

private_output_dir="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/alertmanager-auto-diagnosis-live-smoke}"
if [[ -z "$private_output_dir" || "$private_output_dir" == *$'\n'* || "$private_output_dir" == *$'\r'* ]]; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR must be a single-line path"
fi
if ! private_output_dir="$(realpath -m -- "$private_output_dir")"; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR could not be resolved"
fi
repo_private_output_path "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR" "$private_output_dir"
openclarion_private_output_path_allowed "alertmanager-auto-diagnosis-live-smoke" "$ROOT_DIR" "$private_output_dir" || exit $?
if [[ -L "$private_output_dir" ]]; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR must not be a symlink"
fi
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

private_output_file() {
  local name="$1"
  mktemp "$private_output_dir/${name}.XXXXXX.json"
}

output="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT:-$(private_output_file output)}"
if [[ -z "$output" || "$output" == *$'\n'* || "$output" == *$'\r'* ]]; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT must be a single-line path"
fi
if ! output="$(realpath -m -- "$output")"; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT could not be resolved"
fi
repo_private_output_path "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT" "$output"
openclarion_private_output_path_allowed "alertmanager-auto-diagnosis-live-smoke" "$ROOT_DIR" "$output" || exit $?
if [[ -L "$output" ]]; then
  fail "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT must not be a symlink"
fi
output_dir="$(dirname "$output")"
mkdir -p "$output_dir"
chmod 700 "$output_dir"
http_timeout="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT:-15s}"
room_timeout="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ROOM_TIMEOUT:-10m}"
poll_interval="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_POLL_INTERVAL:-5s}"
alert_name="${ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME:-OpenClarionAutoDiagnosisSmoke}"

args=(
  --api-base-url "$OPENCLARION_LIVE_API_BASE_URL"
  --source-profile-id "$source_profile_id"
  --output "$output"
  --http-timeout "$http_timeout"
  --room-timeout "$room_timeout"
  --poll-interval "$poll_interval"
  --alert-name "$alert_name"
)
if [[ -n "$bearer_token" ]]; then
  ALERTMANAGER_AUTO_DIAGNOSIS_EFFECTIVE_WEBHOOK_BEARER_TOKEN="$bearer_token"
  export ALERTMANAGER_AUTO_DIAGNOSIS_EFFECTIVE_WEBHOOK_BEARER_TOKEN
  args+=(--webhook-bearer-token-env ALERTMANAGER_AUTO_DIAGNOSIS_EFFECTIVE_WEBHOOK_BEARER_TOKEN)
fi
if [[ -n "$expected_channel_profile_id" ]]; then
  args+=(--expected-notification-channel-profile-id "$expected_channel_profile_id")
fi
if [[ -n "$expected_content_kind" ]]; then
  args+=(--expected-content-kind "$expected_content_kind")
fi
if [[ -n "$required_content_kinds" ]]; then
  args+=(--required-content-kinds "$required_content_kinds")
fi

echo "[alertmanager-auto-diagnosis-live-smoke] posting synthetic webhook and waiting for AI notification proof..." >&2
go run ./scripts/alertmanager_auto_diagnosis_live_smoke "${args[@]}"
