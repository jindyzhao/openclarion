#!/usr/bin/env bash
# Run a live diagnosis auth status/check proof through the OpenClarion HTTP API.
#
# This is intentionally NOT part of make ci: it requires a running backend and
# live operator credentials. Proof output is written under .openclarion-private/
# by default and never stores the supplied password or token.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_DIAGNOSIS_AUTH_LIVE_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_diagnosis_auth_live_smoke.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_API_BASE_URL
  plus either:
    OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD
    or OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN
    or OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN for static diagnosis auth
    or OPENCLARION_LIVE_BEARER_TOKEN

Optional env:
  OPENCLARION_LIVE_AUTH_MODE=ldap|bearer
  OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE=ldap|static|oidc|unknown|none
  OPENCLARION_LIVE_DIAGNOSIS_AUTH_REQUIRED_SUPPORTED_MODES=oidc,ldap
  OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION=true|false
  DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT
  DIAGNOSIS_AUTH_LIVE_SMOKE_WORKDIR
  DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT

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
  printf '[diagnosis-auth-live-smoke] %s\n' "$1" >&2
  exit 2
}

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "diagnosis-auth-live-smoke" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

if [[ -z "${OPENCLARION_LIVE_API_BASE_URL:-}" ]]; then
  fail "missing required env: OPENCLARION_LIVE_API_BASE_URL"
fi

auth_mode="${OPENCLARION_LIVE_AUTH_MODE:-}"
auth_mode="${auth_mode,,}"
if [[ -z "$auth_mode" ]]; then
  if [[ -n "${OPENCLARION_LIVE_LDAP_USERNAME:-}${OPENCLARION_LIVE_LDAP_PASSWORD:-}" ]]; then
    auth_mode="ldap"
  else
    diagnosis_auth_mode="${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}"
    diagnosis_auth_mode="${diagnosis_auth_mode,,}"
    if [[ "$diagnosis_auth_mode" == "ldap" ]]; then
      auth_mode="ldap"
    fi
  fi
  if [[ -z "$auth_mode" &&
        -n "${OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN:-}${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}${OPENCLARION_LIVE_BEARER_TOKEN:-}" ]]; then
    auth_mode="bearer"
  fi
fi
case "$auth_mode" in
  ldap|bearer) ;;
  "") fail "set OPENCLARION_LIVE_AUTH_MODE or provide LDAP credentials / bearer token" ;;
  *) fail "OPENCLARION_LIVE_AUTH_MODE must be ldap or bearer" ;;
esac

args=(
  --api-base-url "$OPENCLARION_LIVE_API_BASE_URL"
  --auth-mode "$auth_mode"
)

case "$auth_mode" in
  ldap)
    if [[ -z "${OPENCLARION_LIVE_LDAP_USERNAME:-}" || -z "${OPENCLARION_LIVE_LDAP_PASSWORD:-}" ]]; then
      fail "OPENCLARION_LIVE_LDAP_USERNAME and OPENCLARION_LIVE_LDAP_PASSWORD are required for ldap auth mode"
    fi
    if [[ "${OPENCLARION_LIVE_LDAP_USERNAME}" =~ [[:space:]] ]]; then
      fail "OPENCLARION_LIVE_LDAP_USERNAME must not contain whitespace"
    fi
    if [[ "$OPENCLARION_LIVE_LDAP_PASSWORD" == *$'\n'* || "$OPENCLARION_LIVE_LDAP_PASSWORD" == *$'\r'* ]]; then
      fail "OPENCLARION_LIVE_LDAP_PASSWORD must not contain CR or LF"
    fi
    args+=(--ldap-username-env OPENCLARION_LIVE_LDAP_USERNAME)
    args+=(--ldap-password-env OPENCLARION_LIVE_LDAP_PASSWORD)
    : "${OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE:=ldap}"
    ;;
  bearer)
    bearer_token="${OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN:-}"
    diagnosis_auth_mode="${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}"
    diagnosis_auth_mode="${diagnosis_auth_mode,,}"
    if [[ -z "$bearer_token" &&
          -n "${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}" &&
          ( "$diagnosis_auth_mode" == "static" || -z "$diagnosis_auth_mode" ) ]]; then
      bearer_token="$OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN"
    fi
    if [[ -z "$bearer_token" ]]; then
      bearer_token="${OPENCLARION_LIVE_BEARER_TOKEN:-}"
    fi
    bearer_token="${bearer_token#"${bearer_token%%[![:space:]]*}"}"
    bearer_token="${bearer_token%"${bearer_token##*[![:space:]]}"}"
    if [[ "$bearer_token" == Bearer\ * ]]; then
      bearer_token="${bearer_token#Bearer }"
    fi
    if [[ -z "$bearer_token" || "$bearer_token" =~ [[:space:]] ]]; then
      fail "OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header"
    fi
    OPENCLARION_LIVE_DIAGNOSIS_AUTH_EFFECTIVE_BEARER_TOKEN="$bearer_token"
    export OPENCLARION_LIVE_DIAGNOSIS_AUTH_EFFECTIVE_BEARER_TOKEN
    args+=(--bearer-token-env OPENCLARION_LIVE_DIAGNOSIS_AUTH_EFFECTIVE_BEARER_TOKEN)
    if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE:-}" &&
          -n "${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}" ]]; then
      OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE="${OPENCLARION_DIAGNOSIS_AUTH_MODE,,}"
    fi
    ;;
esac

expected_mode="${OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE:-}"
expected_mode="${expected_mode,,}"
if [[ -n "$expected_mode" ]]; then
  case "$expected_mode" in
    ldap|static|oidc|unknown|none) ;;
    *) fail "OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE must be ldap, static, oidc, unknown, or none" ;;
  esac
  args+=(--expected-backend-mode "$expected_mode")
fi

required_supported_modes="${OPENCLARION_LIVE_DIAGNOSIS_AUTH_REQUIRED_SUPPORTED_MODES:-}"
required_supported_modes="${required_supported_modes,,}"
if [[ -n "$required_supported_modes" ]]; then
  IFS=',' read -r -a required_supported_mode_parts <<< "$required_supported_modes"
  for mode in "${required_supported_mode_parts[@]}"; do
    mode="${mode#"${mode%%[![:space:]]*}"}"
    mode="${mode%"${mode##*[![:space:]]}"}"
    case "$mode" in
      ldap|static|oidc|unknown) ;;
      *) fail "OPENCLARION_LIVE_DIAGNOSIS_AUTH_REQUIRED_SUPPORTED_MODES must contain only ldap, static, oidc, or unknown" ;;
    esac
  done
  args+=(--required-supported-modes "$required_supported_modes")
fi

issue_session="${OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION:-}"
issue_session="${issue_session,,}"
case "$issue_session" in
  ""|false|0|no) ;;
  true|1|yes)
    args+=(--issue-session)
    ;;
  *) fail "OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION must be true or false" ;;
esac

private_output_dir="${DIAGNOSIS_AUTH_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/diagnosis-auth-live-smoke}"
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

private_output_file() {
  local name="$1"
  mktemp "$private_output_dir/${name}.XXXXXX.json"
}

output="${DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT:-$(private_output_file output)}"
timeout="${DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT:-15s}"
args+=(--output "$output" --timeout "$timeout")

echo "[diagnosis-auth-live-smoke] checking diagnosis auth wiring..." >&2
go run ./scripts/diagnosis_auth_live_smoke "${args[@]}"
