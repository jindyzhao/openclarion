#!/usr/bin/env bash
# Verify diagnosis browser-session promotion through the OpenClarion web BFF.
#
# This is intentionally NOT part of make ci: it requires a running web app,
# a backend with IAM OIDC or an explicit compatibility auth provider, and live
# operator credentials. Proof output is written under .openclarion-private/ by
# default and never stores the supplied bearer token, password, Authorization
# header, Set-Cookie value, or session token.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_DIAGNOSIS_AUTH_BFF_LIVE_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_diagnosis_auth_bff_live_smoke.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_WEB_BASE_URL
    or OPENCLARION_LIVE_WEB_PORT for http://127.0.0.1:<port>
  plus either:
    OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN
    or OPENCLARION_LIVE_BEARER_TOKEN
    or OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN for static compatibility auth
    or OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD for explicit LDAP compatibility auth

Optional env:
  OPENCLARION_LIVE_AUTH_MODE=bearer|ldap
  DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT
  DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR
  DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_TIMEOUT

PATH must be a regular, non-symlink file owned by the current user, with no
group/other permissions, and outside the OpenClarion repository or under the
repo-local ignored .openclarion-private/ directory.

DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT may point outside the repository. If it
points inside this repository, it must be under ignored .openclarion-private/.
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
  printf '[diagnosis-auth-bff-live-smoke] %s\n' "$1" >&2
  exit 2
}

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "diagnosis-auth-bff-live-smoke" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

if [[ -z "${OPENCLARION_LIVE_WEB_BASE_URL:-}" ]]; then
  web_port="${OPENCLARION_LIVE_WEB_PORT:-}"
  if [[ -z "$web_port" ]]; then
    fail "missing required env: OPENCLARION_LIVE_WEB_BASE_URL or OPENCLARION_LIVE_WEB_PORT"
  fi
  if [[ ! "$web_port" =~ ^[0-9]+$ || "$web_port" == "0" || "$web_port" -gt 65535 ]]; then
    fail "OPENCLARION_LIVE_WEB_PORT must be an integer from 1 to 65535"
  fi
  OPENCLARION_LIVE_WEB_BASE_URL="http://127.0.0.1:${web_port}"
fi

auth_mode="${OPENCLARION_LIVE_AUTH_MODE:-}"
auth_mode="${auth_mode,,}"
if [[ -z "$auth_mode" ]]; then
  if [[ -n "${OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN:-}${OPENCLARION_LIVE_BEARER_TOKEN:-}" ]]; then
    auth_mode="bearer"
  else
    diagnosis_auth_mode="${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}"
    diagnosis_auth_mode="${diagnosis_auth_mode,,}"
    if [[ -n "${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}" &&
          ( "$diagnosis_auth_mode" == "static" || -z "$diagnosis_auth_mode" ) ]]; then
      auth_mode="bearer"
    elif [[ -n "${OPENCLARION_LIVE_LDAP_USERNAME:-}${OPENCLARION_LIVE_LDAP_PASSWORD:-}" ||
            "$diagnosis_auth_mode" == "ldap" ]]; then
      auth_mode="ldap"
    fi
  fi
fi
case "$auth_mode" in
  bearer|ldap) ;;
  "") fail "set OPENCLARION_LIVE_AUTH_MODE or provide a bearer token / LDAP credentials" ;;
  *) fail "OPENCLARION_LIVE_AUTH_MODE must be bearer or ldap" ;;
esac

trim_single_line() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

args=(
  --web-base-url "$OPENCLARION_LIVE_WEB_BASE_URL"
  --auth-mode "$auth_mode"
)

case "$auth_mode" in
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
    bearer_token="$(trim_single_line "$bearer_token")"
    if [[ "$bearer_token" =~ ^[Bb][Ee][Aa][Rr][Ee][Rr][[:space:]]+(.+)$ ]]; then
      bearer_token="${BASH_REMATCH[1]}"
    fi
    if [[ -z "$bearer_token" || "$bearer_token" =~ [[:space:]] ]]; then
      fail "OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header"
    fi
    OPENCLARION_LIVE_DIAGNOSIS_AUTH_BFF_EFFECTIVE_BEARER_TOKEN="$bearer_token"
    export OPENCLARION_LIVE_DIAGNOSIS_AUTH_BFF_EFFECTIVE_BEARER_TOKEN
    args+=(--bearer-token-env OPENCLARION_LIVE_DIAGNOSIS_AUTH_BFF_EFFECTIVE_BEARER_TOKEN)
    ;;
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
    ;;
esac

private_output_dir="${DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/diagnosis-auth-bff-live-smoke}"
if [[ -z "$private_output_dir" || "$private_output_dir" == *$'\n'* || "$private_output_dir" == *$'\r'* ]]; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR must be a single-line path"
fi
if ! private_output_dir="$(realpath -m -- "$private_output_dir")"; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR could not be resolved"
fi
openclarion_private_output_path_allowed "diagnosis-auth-bff-live-smoke" "$ROOT_DIR" "$private_output_dir" || exit $?
if [[ -L "$private_output_dir" ]]; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR must not be a symlink"
fi
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

private_output_file() {
  local name="$1"
  mktemp "$private_output_dir/${name}.XXXXXX.json"
}

output="${DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT:-$(private_output_file output)}"
if [[ -z "$output" || "$output" == *$'\n'* || "$output" == *$'\r'* ]]; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT must be a single-line path"
fi
if ! output="$(realpath -m -- "$output")"; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT could not be resolved"
fi
openclarion_private_output_path_allowed "diagnosis-auth-bff-live-smoke" "$ROOT_DIR" "$output" || exit $?
if [[ -L "$output" ]]; then
  fail "DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT must not be a symlink"
fi
output_dir="$(dirname "$output")"
mkdir -p "$output_dir"
chmod 700 "$output_dir"

timeout="${DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_TIMEOUT:-15s}"
args+=(--output "$output" --timeout "$timeout")

echo "[diagnosis-auth-bff-live-smoke] checking ${auth_mode} browser-session promotion through the web BFF..." >&2
go run ./scripts/diagnosis_auth_bff_live_smoke "${args[@]}"
