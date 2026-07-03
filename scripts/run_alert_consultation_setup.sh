#!/usr/bin/env bash
# Create or replace live Alertmanager -> auto_room -> Enterprise WeChat
# consultation configuration through the OpenClarion HTTP API.
#
# This is intentionally NOT part of make ci: it requires a running backend,
# database, server-side secret resolver configuration, and operator-owned live
# endpoints.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_ALERT_CONSULTATION_SETUP_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_alert_consultation_setup.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_API_BASE_URL
  OPENCLARION_LIVE_ALERTMANAGER_BASE_URL

Optional env:
  OPENCLARION_LIVE_BEARER_TOKEN
  OPENCLARION_LIVE_ALERTMANAGER_AUTH_MODE       default: none
  OPENCLARION_LIVE_ALERTMANAGER_SECRET_REF      required when auth mode is bearer
  OPENCLARION_LIVE_NOTIFICATION_CHANNEL_SECRET_REF
                                                default: secret/openclarion/ops-wecom
  OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID
  OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID
  OPENCLARION_LIVE_GROUPING_POLICY_ID
  OPENCLARION_LIVE_REPORT_WORKFLOW_POLICY_ID
  OPENCLARION_ALERT_CONSULTATION_SETUP_OUTPUT
  OPENCLARION_ALERT_CONSULTATION_SETUP_ENV_OUTPUT
  OPENCLARION_ALERT_CONSULTATION_SETUP_WORKDIR
  OPENCLARION_ALERT_CONSULTATION_SETUP_TIMEOUT  default: 30s
  OPENCLARION_ALERT_CONSULTATION_SETUP_ENABLE_POLICY
                                                default: true
  OPENCLARION_ALERT_CONSULTATION_SETUP_REPORT_SCENARIO
                                                default: cascade

When OPENCLARION_ALERT_CONSULTATION_SETUP_ENABLE_POLICY=true, the setup sends
Enterprise WeChat AI diagnosis and diagnosis close sample notifications before
enabling the report workflow policy.

PATH must be a regular, non-symlink file owned by the current user, with no
group/other permissions, and outside the OpenClarion repository or under the
repo-local ignored .openclarion-private/ directory.
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
  printf '[alert-consultation-setup] %s\n' "$1" >&2
  exit 2
}

positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "alert-consultation-setup" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env OPENCLARION_LIVE_API_BASE_URL
require_env OPENCLARION_LIVE_ALERTMANAGER_BASE_URL

if ((${#missing[@]} > 0)); then
  printf '[alert-consultation-setup] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

bearer_token="${OPENCLARION_LIVE_BEARER_TOKEN:-}"
if [[ -n "$bearer_token" ]]; then
  bearer_token="$(trim "$bearer_token")"
  if [[ "$bearer_token" == Bearer\ * ]]; then
    bearer_token="${bearer_token#Bearer }"
  fi
  if [[ -z "$bearer_token" || "$bearer_token" =~ [[:space:]] ]]; then
    fail "OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header"
  fi
fi

auth_mode="${OPENCLARION_LIVE_ALERTMANAGER_AUTH_MODE:-none}"
auth_mode="$(trim "$auth_mode")"
auth_mode="${auth_mode,,}"
case "$auth_mode" in
  none|bearer) ;;
  *) fail "OPENCLARION_LIVE_ALERTMANAGER_AUTH_MODE must be none or bearer" ;;
esac

alertmanager_secret_ref="$(trim "${OPENCLARION_LIVE_ALERTMANAGER_SECRET_REF:-}")"
if [[ "$auth_mode" == "bearer" && -z "$alertmanager_secret_ref" ]]; then
  fail "OPENCLARION_LIVE_ALERTMANAGER_SECRET_REF is required when Alertmanager auth mode is bearer"
fi
if [[ "$auth_mode" == "none" && -n "$alertmanager_secret_ref" ]]; then
  fail "OPENCLARION_LIVE_ALERTMANAGER_SECRET_REF requires OPENCLARION_LIVE_ALERTMANAGER_AUTH_MODE=bearer"
fi
if [[ -n "$alertmanager_secret_ref" && "$alertmanager_secret_ref" =~ [[:space:]] ]]; then
  fail "OPENCLARION_LIVE_ALERTMANAGER_SECRET_REF must not contain whitespace"
fi

notification_secret_ref="$(trim "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_SECRET_REF:-secret/openclarion/ops-wecom}")"
if [[ -z "$notification_secret_ref" || "$notification_secret_ref" =~ [[:space:]] ]]; then
  fail "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_SECRET_REF must be a single non-empty secret reference"
fi
case "$notification_secret_ref" in
  http://*|https://*)
    fail "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_SECRET_REF must be a secret reference, not a webhook URL"
    ;;
esac

source_id="${OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID:-}"
channel_id="${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}"
grouping_policy_id="${OPENCLARION_LIVE_GROUPING_POLICY_ID:-}"
workflow_policy_id="${OPENCLARION_LIVE_REPORT_WORKFLOW_POLICY_ID:-}"

for pair in \
  "OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID:$source_id" \
  "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:$channel_id" \
  "OPENCLARION_LIVE_GROUPING_POLICY_ID:$grouping_policy_id" \
  "OPENCLARION_LIVE_REPORT_WORKFLOW_POLICY_ID:$workflow_policy_id"; do
  key="${pair%%:*}"
  value="${pair#*:}"
  if [[ -n "$value" ]] && ! positive_integer "$value"; then
    fail "$key must be a positive integer when set"
  fi
done

enable_policy="${OPENCLARION_ALERT_CONSULTATION_SETUP_ENABLE_POLICY:-true}"
enable_policy="$(trim "$enable_policy")"
enable_policy="${enable_policy,,}"
case "$enable_policy" in
  true|false) ;;
  *) fail "OPENCLARION_ALERT_CONSULTATION_SETUP_ENABLE_POLICY must be true or false" ;;
esac

report_scenario="${OPENCLARION_ALERT_CONSULTATION_SETUP_REPORT_SCENARIO:-cascade}"
report_scenario="$(trim "$report_scenario")"
report_scenario="${report_scenario,,}"
case "$report_scenario" in
  single_alert|cascade|alert_storm) ;;
  *) fail "OPENCLARION_ALERT_CONSULTATION_SETUP_REPORT_SCENARIO must be single_alert, cascade, or alert_storm" ;;
esac

private_output_dir="${OPENCLARION_ALERT_CONSULTATION_SETUP_WORKDIR:-$ROOT_DIR/.openclarion-private/alert-consultation-setup}"
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

output="${OPENCLARION_ALERT_CONSULTATION_SETUP_OUTPUT:-$private_output_dir/latest.json}"
env_output="${OPENCLARION_ALERT_CONSULTATION_SETUP_ENV_OUTPUT:-$private_output_dir/live-ids.env}"
timeout="${OPENCLARION_ALERT_CONSULTATION_SETUP_TIMEOUT:-30s}"

args=(
  --api-base-url "$OPENCLARION_LIVE_API_BASE_URL"
  --alertmanager-base-url "$OPENCLARION_LIVE_ALERTMANAGER_BASE_URL"
  --alertmanager-auth-mode "$auth_mode"
  --notification-secret-ref "$notification_secret_ref"
  --report-scenario "$report_scenario"
  --enable-policy="$enable_policy"
  --output "$output"
  --env-output "$env_output"
  --timeout "$timeout"
)
if [[ -n "$bearer_token" ]]; then
  OPENCLARION_ALERT_CONSULTATION_SETUP_EFFECTIVE_BEARER_TOKEN="$bearer_token"
  export OPENCLARION_ALERT_CONSULTATION_SETUP_EFFECTIVE_BEARER_TOKEN
  args+=(--bearer-token-env OPENCLARION_ALERT_CONSULTATION_SETUP_EFFECTIVE_BEARER_TOKEN)
fi
if [[ -n "$alertmanager_secret_ref" ]]; then
  args+=(--alertmanager-secret-ref "$alertmanager_secret_ref")
fi
if [[ -n "$source_id" ]]; then
  args+=(--source-id "$source_id")
fi
if [[ -n "$channel_id" ]]; then
  args+=(--channel-id "$channel_id")
fi
if [[ -n "$grouping_policy_id" ]]; then
  args+=(--grouping-policy-id "$grouping_policy_id")
fi
if [[ -n "$workflow_policy_id" ]]; then
  args+=(--report-workflow-policy-id "$workflow_policy_id")
fi

echo "[alert-consultation-setup] creating or updating live consultation configuration..." >&2
go run ./scripts/alert_consultation_setup "${args[@]}"
