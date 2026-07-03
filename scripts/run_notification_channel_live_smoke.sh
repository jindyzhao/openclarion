#!/usr/bin/env bash
# Run a live notification-channel delivery test through the OpenClarion HTTP API.
#
# This is intentionally NOT part of make ci: it requires a running backend,
# persisted notification channel profile, and server-side secret resolver
# configuration for the profile secret_ref.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_NOTIFICATION_CHANNEL_LIVE_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_notification_channel_live_smoke.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_API_BASE_URL
  NOTIFICATION_CHANNEL_PROFILE_ID
    or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID

Optional env:
  OPENCLARION_LIVE_BEARER_TOKEN
  NOTIFICATION_CHANNEL_EXPECTED_KIND
    or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND
    diagnosis samples default this expectation to wecom
  NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND
    or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND
  NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS
    or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS
    comma-separated; useful for ai_diagnosis_sample,diagnosis_close_sample
  NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true|false
    or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true|false
    true requires both ai_diagnosis_sample and diagnosis_close_sample
  NOTIFICATION_CHANNEL_LIVE_SMOKE_OUTPUT
  NOTIFICATION_CHANNEL_LIVE_SMOKE_WORKDIR
  NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT

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
  printf '[notification-channel-live-smoke] %s\n' "$1" >&2
  exit 2
}

positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "notification-channel-live-smoke" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env OPENCLARION_LIVE_API_BASE_URL
if [[ -z "${NOTIFICATION_CHANNEL_PROFILE_ID:-}${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}" ]]; then
  missing+=("NOTIFICATION_CHANNEL_PROFILE_ID or OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID")
fi

if ((${#missing[@]} > 0)); then
  printf '[notification-channel-live-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

channel_id="${NOTIFICATION_CHANNEL_PROFILE_ID:-${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}}"
if [[ -n "${NOTIFICATION_CHANNEL_PROFILE_ID:-}" && -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}" &&
      "${NOTIFICATION_CHANNEL_PROFILE_ID}" != "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID}" ]]; then
  fail "NOTIFICATION_CHANNEL_PROFILE_ID and OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID must match when both are set"
fi
if ! positive_integer "$channel_id"; then
  fail "Notification channel profile ID must be a positive integer"
fi

expected_kind="${NOTIFICATION_CHANNEL_EXPECTED_KIND:-${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND:-}}"
if [[ -n "${NOTIFICATION_CHANNEL_EXPECTED_KIND:-}" && -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND:-}" &&
      "${NOTIFICATION_CHANNEL_EXPECTED_KIND}" != "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND}" ]]; then
  fail "NOTIFICATION_CHANNEL_EXPECTED_KIND and OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND must match when both are set"
fi
if [[ -n "$expected_kind" ]]; then
  expected_kind="${expected_kind#"${expected_kind%%[![:space:]]*}"}"
  expected_kind="${expected_kind%"${expected_kind##*[![:space:]]}"}"
  expected_kind="${expected_kind,,}"
  if [[ "$expected_kind" != "webhook" && "$expected_kind" != "wecom" ]]; then
    fail "Notification channel expected kind must be webhook or wecom"
  fi
fi

expected_content_kind="${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND:-${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND:-}}"
if [[ -n "${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND:-}" && -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND:-}" &&
      "${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND}" != "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND}" ]]; then
  fail "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND and OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND must match when both are set"
fi
expected_content_kinds="${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS:-${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS:-}}"
if [[ -n "${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS:-}" && -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS:-}" &&
      "${NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS}" != "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS}" ]]; then
  fail "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS and OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS must match when both are set"
fi
if [[ -n "$expected_content_kind" && -n "$expected_content_kinds" ]]; then
  fail "Set only one of NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND or NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS"
fi
if [[ -n "$expected_content_kind" ]]; then
  expected_content_kind="${expected_content_kind#"${expected_content_kind%%[![:space:]]*}"}"
  expected_content_kind="${expected_content_kind%"${expected_content_kind##*[![:space:]]}"}"
  expected_content_kind="${expected_content_kind,,}"
  case "$expected_content_kind" in
    transport_sample|ai_diagnosis_sample|diagnosis_close_sample) ;;
    *) fail "Notification channel expected content kind must be transport_sample, ai_diagnosis_sample, or diagnosis_close_sample" ;;
  esac
fi
expected_has_diagnosis_content=false
case "$expected_content_kind" in
  ai_diagnosis_sample|diagnosis_close_sample) expected_has_diagnosis_content=true ;;
esac
if [[ -n "$expected_content_kinds" ]]; then
  IFS=',' read -r -a expected_content_kind_items <<< "$expected_content_kinds"
  normalized_expected_content_kinds=()
  seen_expected_content_kinds=","
  for item in "${expected_content_kind_items[@]}"; do
    item="${item#"${item%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    item="${item,,}"
    case "$item" in
      transport_sample|ai_diagnosis_sample|diagnosis_close_sample) ;;
      "") fail "Notification channel expected content kinds must not contain empty entries" ;;
      *) fail "Notification channel expected content kinds must be comma-separated transport_sample, ai_diagnosis_sample, or diagnosis_close_sample" ;;
    esac
    if [[ "$seen_expected_content_kinds" == *",$item,"* ]]; then
      fail "Notification channel expected content kinds must not contain duplicates"
    fi
    seen_expected_content_kinds+="$item,"
    normalized_expected_content_kinds+=("$item")
    case "$item" in
      ai_diagnosis_sample|diagnosis_close_sample) expected_has_diagnosis_content=true ;;
    esac
  done
  expected_content_kinds="$(IFS=','; echo "${normalized_expected_content_kinds[*]}")"
fi
if [[ "$expected_has_diagnosis_content" == true ]]; then
  if [[ -z "$expected_kind" ]]; then
    expected_kind="wecom"
  elif [[ "$expected_kind" != "wecom" ]]; then
    fail "Notification channel diagnosis content requires expected kind wecom"
  fi
fi

require_ai_proof="${NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF:-${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF:-}}"
if [[ -n "${NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF:-}" && -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF:-}" &&
      "${NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF}" != "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF}" ]]; then
  fail "NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF and OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF must match when both are set"
fi
if [[ -n "$require_ai_proof" ]]; then
  require_ai_proof="${require_ai_proof#"${require_ai_proof%%[![:space:]]*}"}"
  require_ai_proof="${require_ai_proof%"${require_ai_proof##*[![:space:]]}"}"
  require_ai_proof="${require_ai_proof,,}"
  case "$require_ai_proof" in
    true|false) ;;
    *) fail "Notification channel require AI proof must be true or false" ;;
  esac
fi
if [[ "$require_ai_proof" == "true" ]]; then
  if [[ -n "$expected_content_kind" || -n "$expected_content_kinds" ]]; then
    fail "Notification channel require AI proof cannot be combined with explicit expected content kinds"
  fi
  expected_kind="${expected_kind:-wecom}"
  if [[ "$expected_kind" != "wecom" ]]; then
    fail "Notification channel AI proof requires expected kind wecom"
  fi
fi

bearer_token="${OPENCLARION_LIVE_BEARER_TOKEN:-}"
if [[ -n "$bearer_token" ]]; then
  bearer_token="${bearer_token#"${bearer_token%%[![:space:]]*}"}"
  bearer_token="${bearer_token%"${bearer_token##*[![:space:]]}"}"
  if [[ "$bearer_token" == Bearer\ * ]]; then
    bearer_token="${bearer_token#Bearer }"
  fi
  if [[ -z "$bearer_token" || "$bearer_token" =~ [[:space:]] ]]; then
    fail "OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header"
  fi
fi

private_output_dir="${NOTIFICATION_CHANNEL_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/notification-channel-live-smoke}"
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

private_output_file() {
  local name="$1"
  mktemp "$private_output_dir/${name}.XXXXXX.json"
}

output="${NOTIFICATION_CHANNEL_LIVE_SMOKE_OUTPUT:-$(private_output_file output)}"
timeout="${NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT:-15s}"

args=(
  --api-base-url "$OPENCLARION_LIVE_API_BASE_URL"
  --channel-id "$channel_id"
  --output "$output"
  --timeout "$timeout"
)
if [[ -n "$expected_kind" ]]; then
  args+=(--expected-kind "$expected_kind")
fi
if [[ -n "$expected_content_kind" ]]; then
  args+=(--expected-content-kind "$expected_content_kind")
fi
if [[ -n "$expected_content_kinds" ]]; then
  args+=(--expected-content-kinds "$expected_content_kinds")
fi
if [[ "$require_ai_proof" == "true" ]]; then
  args+=(--require-ai-proof)
fi
if [[ -n "$bearer_token" ]]; then
  OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EFFECTIVE_BEARER_TOKEN="$bearer_token"
  export OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EFFECTIVE_BEARER_TOKEN
  args+=(--bearer-token-env OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EFFECTIVE_BEARER_TOKEN)
fi

echo "[notification-channel-live-smoke] testing configured notification channel..." >&2
go run ./scripts/notification_channel_live_smoke "${args[@]}"
