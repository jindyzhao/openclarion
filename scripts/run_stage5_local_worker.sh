#!/usr/bin/env bash
# Start a local Stage 5 worker/API process from a private operator env file.
#
# The env file is sourced intentionally: it must be a developer-owned shell
# assignment file outside this repository, not an untrusted artifact.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

check_only=""
env_file="${OPENCLARION_STAGE5_WORKER_ENV_FILE:-}"
source_checkout="${OPENCLARION_STAGE5_WORKER_SOURCE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_stage5_local_worker.sh [--env-file PATH] [--check-only] [--source]

PATH must be a regular, non-symlink file owned by the current user, with no
group/other permissions, and outside the OpenClarion repository or under the
repo-local ignored .openclarion-private/ directory.

--source, or OPENCLARION_STAGE5_WORKER_SOURCE=1, ignores
OPENCLARION_STAGE5_WORKER_BINARY and runs the current checkout with go run.
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
    --check-only)
      check_only="1"
      shift
      ;;
    --source)
      source_checkout="1"
      shift
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
  printf '[stage5-local-worker] %s\n' "$1" >&2
  exit 2
}

require_other_permission() {
  local path="$1"
  local mask="$2"
  local subject="$3"
  local hint="$4"
  local mode
  local other_digit

  if ! mode="$(stat -c '%a' "$path" 2>/dev/null)"; then
    fail "$subject permission mode could not be read"
  fi
  other_digit="${mode: -1}"
  if [[ ! "$other_digit" =~ ^[0-7]$ ]]; then
    fail "$subject permission mode is invalid"
  fi
  if (( (10#$other_digit & mask) != mask )); then
    fail "$subject must be accessible by the sandbox user; $hint"
  fi
}

if [[ -z "$env_file" ]]; then
  fail "set OPENCLARION_STAGE5_WORKER_ENV_FILE or pass --env-file"
fi

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "stage5-local-worker" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

: "${DATABASE_URL:=postgres://openclarion:openclarion_dev@localhost:25432/openclarion?sslmode=disable}"
: "${TEMPORAL_HOST_PORT:=localhost:27233}"
: "${LISTEN_ADDR:=127.0.0.1:32101}"
: "${OPENCLARION_TEMPORAL_TASK_QUEUE:=openclarion-stage5}"
: "${OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS:=http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:3000,http://localhost:3000}"
: "${OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT:=$ROOT_DIR/.openclarion-private/agent-config}"
: "${OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL:=${OPENCLARION_IAM_OIDC_ISSUER:-${OIDC_ISSUER:-}}}"
: "${OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID:=${OPENCLARION_IAM_OIDC_CLIENT_ID:-${OIDC_CLIENT_ID:-}}}"

if [[ -z "${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}" ]]; then
  if [[ -n "${OPENCLARION_DIAGNOSIS_LDAP_URL:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_BASE_DN:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_BIND_DN:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES:-}" ||
        -n "${OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES:-}" ]]; then
    OPENCLARION_DIAGNOSIS_AUTH_MODE="ldap"
  elif [[ -n "${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}${OPENCLARION_DIAGNOSIS_STATIC_SUBJECT:-}${OPENCLARION_DIAGNOSIS_STATIC_ROLES:-}" ]]; then
    OPENCLARION_DIAGNOSIS_AUTH_MODE="static"
  elif [[ -n "${OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL:-}${OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID:-}" ]]; then
    OPENCLARION_DIAGNOSIS_AUTH_MODE="oidc"
  else
    OPENCLARION_DIAGNOSIS_AUTH_MODE="ldap"
  fi
fi
OPENCLARION_DIAGNOSIS_AUTH_MODE="${OPENCLARION_DIAGNOSIS_AUTH_MODE,,}"
case "$OPENCLARION_DIAGNOSIS_AUTH_MODE" in
  ldap)
    ;;
  oidc)
    : "${OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID:=openclarion-web}"
    ;;
  static)
    ;;
  *)
    fail "OPENCLARION_DIAGNOSIS_AUTH_MODE must be ldap, oidc, or static"
    ;;
esac

: "${OPENCLARION_LLM_BASE_URL:=${OPENCLARION_DIAGNOSIS_LLM_BASE_URL:-}}"
: "${OPENCLARION_LLM_API_KEY:=${OPENCLARION_DIAGNOSIS_LLM_API_KEY:-}}"
: "${OPENCLARION_LLM_MODEL:=${OPENCLARION_DIAGNOSIS_LLM_MODEL:-}}"
: "${OPENCLARION_DIAGNOSIS_LLM_BASE_URL:=${OPENCLARION_LLM_BASE_URL:-}}"
: "${OPENCLARION_DIAGNOSIS_LLM_API_KEY:=${OPENCLARION_LLM_API_KEY:-}}"
: "${OPENCLARION_DIAGNOSIS_LLM_MODEL:=${OPENCLARION_LLM_MODEL:-}}"
if [[ -n "${OPENCLARION_SANDBOX_EGRESS_ALLOWED:-}" ]]; then
  : "${OPENCLARION_SANDBOX_EGRESS_NETWORK:=openclarion-sandbox-allowlist}"
fi

export DATABASE_URL
export TEMPORAL_HOST_PORT
export LISTEN_ADDR
export OPENCLARION_TEMPORAL_TASK_QUEUE
export OPENCLARION_DIAGNOSIS_AUTH_MODE
export OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL
export OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID
export OPENCLARION_DIAGNOSIS_LDAP_URL
export OPENCLARION_DIAGNOSIS_LDAP_BASE_DN
export OPENCLARION_DIAGNOSIS_LDAP_BIND_DN
export OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD
export OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER
export OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE
export OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE
export OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES
export OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES
export OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES
export OPENCLARION_DIAGNOSIS_LDAP_START_TLS
export OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT
export OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN
export OPENCLARION_DIAGNOSIS_STATIC_SUBJECT
export OPENCLARION_DIAGNOSIS_STATIC_ROLES
export OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY
export OPENCLARION_WECOM_CORP_ID
export OPENCLARION_WECOM_CALLBACK_TOKEN
export OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY
export OPENCLARION_WECOM_CALLBACK_RECEIVE_ID
export OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS
export OPENCLARION_SANDBOX_IMAGE_REF
export OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT
export OPENCLARION_SANDBOX_EGRESS_ALLOWED
export OPENCLARION_SANDBOX_EGRESS_NETWORK
export OPENCLARION_LLM_BASE_URL
export OPENCLARION_LLM_API_KEY
export OPENCLARION_LLM_MODEL
export OPENCLARION_DIAGNOSIS_LLM_BASE_URL
export OPENCLARION_DIAGNOSIS_LLM_API_KEY
export OPENCLARION_DIAGNOSIS_LLM_MODEL
export OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS
export OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE
export OPENCLARION_IM_WEBHOOK_URL
export OPENCLARION_IM_WEBHOOK_FORMAT
export OPENCLARION_PROMETHEUS_URL
export OPENCLARION_PROMETHEUS_BEARER_TOKEN
export OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON
export OPENCLARION_NOTIFICATION_CHANNEL_WECOM_SECRET_REFS
export OPENCLARION_STAGE5_WORKER_BINARY

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env DATABASE_URL
require_env TEMPORAL_HOST_PORT
require_env LISTEN_ADDR
require_env OPENCLARION_TEMPORAL_TASK_QUEUE
require_env OPENCLARION_DIAGNOSIS_AUTH_MODE
case "$OPENCLARION_DIAGNOSIS_AUTH_MODE" in
  ldap)
    require_env OPENCLARION_DIAGNOSIS_LDAP_URL
    require_env OPENCLARION_DIAGNOSIS_LDAP_BASE_DN
    if [[ -z "${OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES:-}${OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES:-}${OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES:-}" ]]; then
      missing+=("OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES or OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES or OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES")
    fi
    if [[ (-n "${OPENCLARION_DIAGNOSIS_LDAP_BIND_DN:-}" && -z "${OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD:-}") ||
          (-z "${OPENCLARION_DIAGNOSIS_LDAP_BIND_DN:-}" && -n "${OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD:-}") ]]; then
      fail "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN and OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD must be configured together"
    fi
    ldap_start_tls="${OPENCLARION_DIAGNOSIS_LDAP_START_TLS:-}"
    ldap_start_tls="${ldap_start_tls,,}"
    ldap_allow_plaintext="${OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT:-}"
    ldap_allow_plaintext="${ldap_allow_plaintext,,}"
    if [[ -n "$ldap_start_tls" && "$ldap_start_tls" != "true" && "$ldap_start_tls" != "false" ]]; then
      fail "OPENCLARION_DIAGNOSIS_LDAP_START_TLS must be true or false"
    fi
    if [[ -n "$ldap_allow_plaintext" && "$ldap_allow_plaintext" != "true" && "$ldap_allow_plaintext" != "false" ]]; then
      fail "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT must be true or false"
    fi
    if [[ "${OPENCLARION_DIAGNOSIS_LDAP_URL:-}" == [Ll][Dd][Aa][Pp]://* &&
          "$ldap_start_tls" != "true" &&
          "$ldap_allow_plaintext" != "true" ]]; then
      fail "OPENCLARION_DIAGNOSIS_LDAP_URL uses ldap://; set OPENCLARION_DIAGNOSIS_LDAP_START_TLS=true or OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT=true"
    fi
    ;;
  oidc)
    require_env OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL
    require_env OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID
    ;;
  static)
    require_env OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN
    require_env OPENCLARION_DIAGNOSIS_STATIC_SUBJECT
    require_env OPENCLARION_DIAGNOSIS_STATIC_ROLES
    ;;
esac
require_env OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS
require_env OPENCLARION_LLM_BASE_URL
require_env OPENCLARION_LLM_API_KEY
require_env OPENCLARION_LLM_MODEL
require_env OPENCLARION_DIAGNOSIS_LLM_BASE_URL
require_env OPENCLARION_DIAGNOSIS_LLM_API_KEY
require_env OPENCLARION_DIAGNOSIS_LLM_MODEL
require_env OPENCLARION_SANDBOX_IMAGE_REF
require_env OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT
require_env OPENCLARION_SANDBOX_EGRESS_ALLOWED
require_env OPENCLARION_SANDBOX_EGRESS_NETWORK
if [[ -z "${OPENCLARION_IM_WEBHOOK_URL:-}" && -z "${OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON:-}" ]]; then
  missing+=("OPENCLARION_IM_WEBHOOK_URL or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON")
fi

if ((${#missing[@]} > 0)); then
  printf '[stage5-local-worker] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

if [[ ! "$OPENCLARION_SANDBOX_IMAGE_REF" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]]; then
  fail "OPENCLARION_SANDBOX_IMAGE_REF must be an immutable name@sha256:<64 lowercase hex digest> reference"
fi

if [[ -L "$OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT" || ! -d "$OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT" ]]; then
  fail "OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT must be a direct directory"
fi

diagnosis_agent_config_dir="$OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT/diagnosis-assistant"
diagnosis_agent_instructions="$diagnosis_agent_config_dir/instructions.md"
if [[ -L "$diagnosis_agent_config_dir" || ! -d "$diagnosis_agent_config_dir" ]]; then
  fail "OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT must contain a direct diagnosis-assistant directory"
fi
if [[ -L "$diagnosis_agent_instructions" || ! -f "$diagnosis_agent_instructions" ]]; then
  fail "diagnosis-assistant instructions.md must be a direct regular file"
fi
if [[ ! -s "$diagnosis_agent_instructions" ]]; then
  fail "diagnosis-assistant instructions.md must not be empty"
fi
diagnosis_agent_instructions_size="$(wc -c <"$diagnosis_agent_instructions")"
diagnosis_agent_instructions_size="${diagnosis_agent_instructions_size//[[:space:]]/}"
if [[ ! "$diagnosis_agent_instructions_size" =~ ^[0-9]+$ ]]; then
  fail "diagnosis-assistant instructions.md size could not be read"
fi
if (( diagnosis_agent_instructions_size > 65536 )); then
  fail "diagnosis-assistant instructions.md must be 64 KiB or smaller"
fi
require_other_permission "$diagnosis_agent_config_dir" 5 \
  "diagnosis-assistant agent config directory" \
  "set mode 0755 or grant other read/execute permission"
require_other_permission "$diagnosis_agent_instructions" 4 \
  "diagnosis-assistant instructions.md" \
  "set mode 0644 or grant other-read permission"

wecom_webhook_host="qyapi.weixin.qq.com"
wecom_webhook_path="/cgi-bin/webhook/send"
wecom_webhook_prefix="https://${wecom_webhook_host}${wecom_webhook_path}"
if [[ -n "${OPENCLARION_IM_WEBHOOK_URL:-}" &&
      "${OPENCLARION_IM_WEBHOOK_URL}" == "$wecom_webhook_prefix"* &&
      "${OPENCLARION_IM_WEBHOOK_FORMAT:-}" != "wecom" ]]; then
  fail "WeCom webhook endpoints require OPENCLARION_IM_WEBHOOK_FORMAT=wecom"
fi

truthy() {
  case "${1:-}" in
    1 | true | TRUE | yes | YES)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

worker_cmd=()
worker_mode=""
if truthy "$source_checkout"; then
  if ! command -v go >/dev/null 2>&1; then
    fail "go must be available when --source or OPENCLARION_STAGE5_WORKER_SOURCE=1 is used"
  fi
  worker_cmd=(go run ./cmd/openclarion serve)
  worker_mode="current checkout"
elif [[ -n "${OPENCLARION_STAGE5_WORKER_BINARY:-}" ]]; then
  if [[ "$OPENCLARION_STAGE5_WORKER_BINARY" == *$'\n'* ||
        "$OPENCLARION_STAGE5_WORKER_BINARY" == *$'\r'* ]]; then
    fail "OPENCLARION_STAGE5_WORKER_BINARY must be a single-line path"
  fi
  if [[ -L "$OPENCLARION_STAGE5_WORKER_BINARY" ||
        ! -f "$OPENCLARION_STAGE5_WORKER_BINARY" ||
        ! -x "$OPENCLARION_STAGE5_WORKER_BINARY" ]]; then
    fail "OPENCLARION_STAGE5_WORKER_BINARY must be a direct executable file"
  fi
  worker_cmd=("$OPENCLARION_STAGE5_WORKER_BINARY" serve)
  worker_mode="configured binary"
else
  if ! command -v go >/dev/null 2>&1; then
    fail "go must be available when OPENCLARION_STAGE5_WORKER_BINARY is not set"
  fi
  worker_cmd=(go run ./cmd/openclarion serve)
  worker_mode="current checkout"
fi

if [[ "$worker_mode" == "current checkout" &&
      -f "$ROOT_DIR/go.mod" &&
      -f "$ROOT_DIR/scripts/diagnosis_auth_env_check/main.go" ]]; then
  if ! go run ./scripts/diagnosis_auth_env_check >/dev/null; then
    fail "diagnosis auth provider environment validation failed"
  fi
fi
if [[ "$worker_mode" == "current checkout" &&
      -f "$ROOT_DIR/go.mod" &&
      -f "$ROOT_DIR/scripts/notification_channel_secret_refs_env_check/main.go" ]]; then
  if ! go run ./scripts/notification_channel_secret_refs_env_check >/dev/null; then
    fail "notification channel secret reference environment validation failed"
  fi
fi

if ! command -v docker >/dev/null 2>&1; then
  fail "docker must be available before starting a Stage 5 sandbox-capable worker"
fi
if ! docker network inspect "$OPENCLARION_SANDBOX_EGRESS_NETWORK" >/dev/null 2>&1; then
  fail "OPENCLARION_SANDBOX_EGRESS_NETWORK must name an existing Docker network for Stage 5 sandbox egress"
fi
if ! docker image inspect "$OPENCLARION_SANDBOX_IMAGE_REF" >/dev/null 2>&1; then
  echo "[stage5-local-worker] pulling digest-pinned sandbox image..." >&2
  if ! docker pull "$OPENCLARION_SANDBOX_IMAGE_REF" >/dev/null 2>&1; then
    fail "OPENCLARION_SANDBOX_IMAGE_REF must be present locally or pullable by Docker before starting a Stage 5 sandbox-capable worker"
  fi
fi

if [[ -n "$check_only" ]]; then
  echo "[stage5-local-worker] OK - env file and runtime prerequisites are ready." >&2
  exit 0
fi

echo "[stage5-local-worker] starting OpenClarion from ${worker_mode} with Stage 5 worker wiring..." >&2
if [[ "$worker_mode" == "configured binary" ]]; then
  echo "[stage5-local-worker] note: configured binary mode ignores uncommitted checkout changes; pass --source to run current checkout." >&2
fi
exec "${worker_cmd[@]}"
