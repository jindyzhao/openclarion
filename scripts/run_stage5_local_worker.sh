#!/usr/bin/env bash
# Start a local Stage 5 worker/API process from a private operator env file.
#
# The env file is sourced intentionally: it must be a developer-owned shell
# assignment file outside this repository, not an untrusted artifact.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

check_only=""
env_file="${OPENCLARION_STAGE5_WORKER_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_stage5_local_worker.sh [--env-file PATH] [--check-only]

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
    --check-only)
      check_only="1"
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
: "${OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID:=openclarion-web}"
: "${OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS:=http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:3000,http://localhost:3000}"
: "${OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT:=$ROOT_DIR/.openclarion-private/agent-config}"

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
export OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL
export OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID
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
export OPENCLARION_IM_WEBHOOK_URL
export OPENCLARION_IM_WEBHOOK_FORMAT
export OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON
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
require_env OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL
require_env OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID
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
require_env OPENCLARION_IM_WEBHOOK_URL

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

if [[ -n "${OPENCLARION_IM_WEBHOOK_URL:-}" &&
      "${OPENCLARION_IM_WEBHOOK_URL}" == https://qyapi.weixin.qq.com/cgi-bin/webhook/send* &&
      "${OPENCLARION_IM_WEBHOOK_FORMAT:-}" != "wecom" ]]; then
  fail "WeCom webhook endpoints require OPENCLARION_IM_WEBHOOK_FORMAT=wecom"
fi

worker_cmd=()
if [[ -n "${OPENCLARION_STAGE5_WORKER_BINARY:-}" ]]; then
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
else
  if ! command -v go >/dev/null 2>&1; then
    fail "go must be available when OPENCLARION_STAGE5_WORKER_BINARY is not set"
  fi
  worker_cmd=(go run ./cmd/openclarion serve)
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

echo "[stage5-local-worker] starting current OpenClarion with Stage 5 worker wiring..." >&2
exec "${worker_cmd[@]}"
