#!/usr/bin/env bash
# Start the complete non-Kubernetes local OpenClarion product stack.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_LOCAL_PRODUCT_ENV_FILE:-${OPENCLARION_STAGE5_WORKER_ENV_FILE:-}}"
check_only=""

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_local_product.sh [--env-file PATH] [--check-only]

Starts the complete local product without Kubernetes. PATH follows the same
private-file ownership and permission rules as run_stage5_local_worker.sh.

--check-only starts local dependencies, applies migrations, and validates the
runtime configurations without starting the API/worker or frontend processes.
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
    -h | --help)
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
  printf '[local-product] %s\n' "$1" >&2
  exit 2
}

validate_port() {
  local name="$1"
  local value="$2"
  [[ "$value" =~ ^[0-9]+$ && ${#value} -le 5 ]] || fail "$name must be numeric"
  local numeric=$((10#$value))
  ((numeric >= 1 && numeric <= 65535)) || fail "$name must be between 1 and 65535"
  printf -v "$name" '%d' "$numeric"
}

validate_bounded_seconds() {
  local name="$1"
  local value="$2"
  local maximum="$3"
  [[ "$value" =~ ^[0-9]+$ && ${#value} -le 4 ]] || fail "$name must be numeric"
  local numeric=$((10#$value))
  ((numeric >= 1 && numeric <= maximum)) || fail "$name must be between 1 and $maximum"
  printf -v "$name" '%d' "$numeric"
}

version_at_least() {
  local actual="${1#v}"
  local required_major="$2"
  local required_minor="$3"
  local required_patch="$4"
  actual="${actual%%-*}"
  local major=""
  local minor=""
  local patch=""
  local extra=""
  IFS=. read -r major minor patch extra <<<"$actual"
  [[ -z "$extra" && "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ && "$patch" =~ ^[0-9]+$ ]] || return 1
  major=$((10#$major))
  minor=$((10#$minor))
  patch=$((10#$patch))
  ((major > required_major)) ||
    ((major == required_major && minor > required_minor)) ||
    ((major == required_major && minor == required_minor && patch >= required_patch))
}

database_url_uses_loopback_host() {
  local url="$1"
  url="${url,,}"
  [[ "$url" =~ ^postgres(ql)?://([^/@[:space:]]+@)?(localhost|127\.[0-9]+\.[0-9]+\.[0-9]+|\[::1\])([:/?]|$) ]]
}

csv_has_value() {
  local compact="${1//,/}"
  compact="${compact//[[:space:]]/}"
  [[ -n "$compact" ]]
}

[[ -n "$env_file" ]] || fail "set OPENCLARION_LOCAL_PRODUCT_ENV_FILE or pass --env-file"
openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "local-product" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

session_signing_key="${OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY:-}"
[[ -n "${session_signing_key//[[:space:]]/}" ]] || \
  fail "OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY is required for the complete browser console"

for tool in curl docker go npm setsid; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool not found in PATH: $tool"
done
compose_version="$(docker compose version --short 2>/dev/null)" || fail "Docker Compose V2 is required"
version_at_least "$compose_version" 2 33 1 || fail "Docker Compose 2.33.1 or newer is required"

: "${POSTGRES_PORT:=25432}"
: "${TEMPORAL_PORT:=27233}"
: "${TEMPORAL_UI_PORT:=28233}"
: "${OPENCLARION_LOCAL_API_PORT:=32101}"
: "${OPENCLARION_LOCAL_WEB_HOST:=127.0.0.1}"
: "${OPENCLARION_LOCAL_WEB_PORT:=3000}"
: "${OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME:=openclarion-local-product}"
: "${OPENCLARION_LOCAL_COMPOSE_WAIT_SECONDS:=180}"
: "${OPENCLARION_LOCAL_PROCESS_WAIT_SECONDS:=120}"
: "${OPENCLARION_LOCAL_EGRESS_PROXY_IMAGE:=openclarion/local-egress-proxy:dev}"
: "${OPENCLARION_LOCAL_DATABASE_NAME:=openclarion_local_product}"
: "${OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE:=0}"
: "${OPENCLARION_LOCAL_REUSE_DIAGNOSIS_RUNNER:=0}"
: "${OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT:=$ROOT_DIR/config/agents}"

validate_port POSTGRES_PORT "$POSTGRES_PORT"
validate_port TEMPORAL_PORT "$TEMPORAL_PORT"
validate_port TEMPORAL_UI_PORT "$TEMPORAL_UI_PORT"
validate_port OPENCLARION_LOCAL_API_PORT "$OPENCLARION_LOCAL_API_PORT"
validate_port OPENCLARION_LOCAL_WEB_PORT "$OPENCLARION_LOCAL_WEB_PORT"
validate_bounded_seconds OPENCLARION_LOCAL_COMPOSE_WAIT_SECONDS "$OPENCLARION_LOCAL_COMPOSE_WAIT_SECONDS" 600
validate_bounded_seconds OPENCLARION_LOCAL_PROCESS_WAIT_SECONDS "$OPENCLARION_LOCAL_PROCESS_WAIT_SECONDS" 600
[[ "$OPENCLARION_LOCAL_WEB_HOST" == "127.0.0.1" || "$OPENCLARION_LOCAL_WEB_HOST" == "localhost" ]] || \
  fail "OPENCLARION_LOCAL_WEB_HOST must be 127.0.0.1 or localhost"
[[ "$OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME" =~ ^[a-z0-9][a-z0-9_-]{0,62}$ ]] || \
  fail "OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME must be a lowercase Compose project name"
[[ "$OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE" == "0" || "$OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE" == "1" ]] || \
  fail "OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE must be 0 or 1"
[[ "$OPENCLARION_LOCAL_REUSE_DIAGNOSIS_RUNNER" == "0" || "$OPENCLARION_LOCAL_REUSE_DIAGNOSIS_RUNNER" == "1" ]] || \
  fail "OPENCLARION_LOCAL_REUSE_DIAGNOSIS_RUNNER must be 0 or 1"

OPENCLARION_SANDBOX_EGRESS_NETWORK="${OPENCLARION_LOCAL_SANDBOX_EGRESS_NETWORK:-${OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME}-sandbox-allowlist}"
OPENCLARION_SANDBOX_EGRESS_PROXY_URL="${OPENCLARION_LOCAL_SANDBOX_EGRESS_PROXY_URL:-http://openclarion-egress-proxy:18080}"
[[ "$OPENCLARION_SANDBOX_EGRESS_NETWORK" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || \
  fail "OPENCLARION_LOCAL_SANDBOX_EGRESS_NETWORK is invalid"
[[ -n "${OPENCLARION_SANDBOX_EGRESS_ALLOWED:-}" ]] || fail "OPENCLARION_SANDBOX_EGRESS_ALLOWED is required"

TEMPORAL_HOST_PORT="127.0.0.1:${TEMPORAL_PORT}"
LISTEN_ADDR="127.0.0.1:${OPENCLARION_LOCAL_API_PORT}"
OPENCLARION_LOCAL_API_BASE_URL="http://${LISTEN_ADDR}"
web_url="http://${OPENCLARION_LOCAL_WEB_HOST}:${OPENCLARION_LOCAL_WEB_PORT}"
: "${OPENCLARION_PUBLIC_BASE_URL:=$web_url}"
case ",${OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS:-}," in
  *,"$web_url",*) ;;
  ,,) OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS="$web_url" ;;
  *) OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS="${OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS},${web_url}" ;;
esac

atlas_database_url=""
atlas_network=""
if [[ "$OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE" == "0" ]]; then
  [[ "$OPENCLARION_LOCAL_DATABASE_NAME" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || \
    fail "OPENCLARION_LOCAL_DATABASE_NAME must be a lowercase PostgreSQL identifier"
  DATABASE_URL="postgres://openclarion:openclarion_dev@127.0.0.1:${POSTGRES_PORT}/${OPENCLARION_LOCAL_DATABASE_NAME}?sslmode=disable"
  atlas_database_url="postgres://openclarion:openclarion_dev@postgres:5432/${OPENCLARION_LOCAL_DATABASE_NAME}?sslmode=disable"
  atlas_network="${OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME}_default"
else
  [[ -n "${DATABASE_URL:-}" ]] || fail "DATABASE_URL is required when OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE=1"
  atlas_database_url="${OPENCLARION_LOCAL_ATLAS_DATABASE_URL:-$DATABASE_URL}"
  atlas_network="${OPENCLARION_LOCAL_ATLAS_DOCKER_NETWORK:-${OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME}_default}"
  if database_url_uses_loopback_host "$atlas_database_url" && [[ "$atlas_network" != "host" ]]; then
    fail "Atlas cannot reach a loopback database from a bridge network; set OPENCLARION_LOCAL_ATLAS_DATABASE_URL to a container-reachable URL or OPENCLARION_LOCAL_ATLAS_DOCKER_NETWORK=host where supported"
  fi
fi

export POSTGRES_PORT TEMPORAL_PORT TEMPORAL_UI_PORT
export DATABASE_URL TEMPORAL_HOST_PORT LISTEN_ADDR
export OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS OPENCLARION_PUBLIC_BASE_URL
export OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT OPENCLARION_SANDBOX_EGRESS_NETWORK OPENCLARION_SANDBOX_EGRESS_PROXY_URL
export OPENCLARION_LOCAL_EGRESS_PROXY_IMAGE

compose=(docker compose --project-name "$OPENCLARION_LOCAL_COMPOSE_PROJECT_NAME")

echo "[local-product] validating frontend prerequisites..." >&2
[[ -f web/package-lock.json && -x web/node_modules/.bin/next ]] || \
  fail "run npm --prefix web ci before starting the local product"

bash scripts/build_local_egress_proxy.sh >/dev/null

if [[ "$OPENCLARION_LOCAL_REUSE_DIAGNOSIS_RUNNER" == "0" ]]; then
  OPENCLARION_SANDBOX_IMAGE_REF="$(bash scripts/build_diagnosis_assistant_runner.sh)"
  [[ "$OPENCLARION_SANDBOX_IMAGE_REF" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || \
    fail "diagnosis runner build did not return one immutable image reference"
  export OPENCLARION_SANDBOX_IMAGE_REF
fi

echo "[local-product] starting PostgreSQL, Temporal, and isolated egress proxy..." >&2
"${compose[@]}" --profile sandbox-egress up -d --wait \
  --wait-timeout "$OPENCLARION_LOCAL_COMPOSE_WAIT_SECONDS" \
  postgres temporal temporal-ui openclarion-egress-proxy

echo "[local-product] validating worker prerequisites..." >&2
OPENCLARION_STAGE5_WORKER_ENV_FILE="$env_file" \
  bash scripts/run_stage5_local_worker.sh --env-file "$env_file" --source --check-only

if [[ "$OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE" == "0" ]]; then
  database_exists="$("${compose[@]}" exec -T postgres \
    psql -U openclarion -d postgres -Atc \
    "SELECT 1 FROM pg_database WHERE datname = '$OPENCLARION_LOCAL_DATABASE_NAME'")"
  if [[ "$database_exists" != "1" ]]; then
    "${compose[@]}" exec -T postgres \
      createdb -U openclarion -O openclarion "$OPENCLARION_LOCAL_DATABASE_NAME"
  fi
  "${compose[@]}" exec -T postgres \
    psql -U openclarion -d "$OPENCLARION_LOCAL_DATABASE_NAME" \
    -v ON_ERROR_STOP=1 -c 'CREATE EXTENSION IF NOT EXISTS vector' >/dev/null
fi

echo "[local-product] applying committed database migrations..." >&2
OPENCLARION_ATLAS_DATABASE_URL="$atlas_database_url" \
OPENCLARION_ATLAS_DOCKER_NETWORK="$atlas_network" \
  bash scripts/apply_atlas_migrations.sh

if [[ "$OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE" == "0" ]] && \
  ! csv_has_value "${OPENCLARION_RBAC_BOOTSTRAP_ADMIN_SUBJECTS:-}"; then
  if ! rbac_admin_count="$("${compose[@]}" exec -T postgres \
    psql -U openclarion -d "$OPENCLARION_LOCAL_DATABASE_NAME" \
    -v ON_ERROR_STOP=1 -Atc \
    "SELECT COUNT(*) FROM rbac_assignments AS assignment JOIN tenants AS tenant ON tenant.id = assignment.tenant_id WHERE assignment.enabled AND assignment.role = 'admin' AND assignment.scope_kind = 'global' AND assignment.scope_key = '' AND tenant.key = 'default' AND tenant.status = 'active'")"; then
    fail "could not inspect enabled global admin RBAC assignments in the active default workspace"
  fi
  [[ "$rbac_admin_count" =~ ^[0-9]+$ ]] || \
    fail "could not inspect enabled global admin RBAC assignments in the active default workspace"
  [[ "$rbac_admin_count" != "0" ]] || \
    fail "active default workspace has no enabled global admin RBAC assignments; set OPENCLARION_RBAC_BOOTSTRAP_ADMIN_SUBJECTS to the authenticated operator subject for initial setup"
fi

if [[ -n "$check_only" ]]; then
  echo "[local-product] OK - non-Kubernetes local product prerequisites are ready." >&2
  exit 0
fi

backend_pid=""
frontend_pid=""
# stop_process_group is invoked indirectly by the EXIT trap cleanup below.
# shellcheck disable=SC2317
stop_process_group() {
  local pid="$1"
  [[ -n "$pid" ]] || return 0

  if kill -0 -- "-$pid" >/dev/null 2>&1; then
    kill -TERM -- "-$pid" >/dev/null 2>&1 || true
    (
      sleep 5
      kill -KILL -- "-$pid" >/dev/null 2>&1 || true
    ) &
    local watchdog_pid="$!"
    wait "$pid" >/dev/null 2>&1 || true
    kill "$watchdog_pid" >/dev/null 2>&1 || true
    wait "$watchdog_pid" >/dev/null 2>&1 || true
    sleep 0.1
    kill -KILL -- "-$pid" >/dev/null 2>&1 || true
    return 0
  fi

  wait "$pid" >/dev/null 2>&1 || true
}

# cleanup is invoked indirectly by the EXIT trap below.
# shellcheck disable=SC2317
cleanup() {
  trap - EXIT INT TERM
  stop_process_group "$frontend_pid"
  stop_process_group "$backend_pid"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

OPENCLARION_STAGE5_WORKER_ENV_FILE="$env_file" \
  setsid bash scripts/run_stage5_local_worker.sh --env-file "$env_file" --source &
backend_pid="$!"

wait_for_url() {
  local label="$1"
  local url="$2"
  local pid="$3"
  local attempt
  for ((attempt = 1; attempt <= OPENCLARION_LOCAL_PROCESS_WAIT_SECONDS; attempt++)); do
    if curl --proto '=http' --fail --silent --show-error --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      fail "$label process exited before becoming ready"
    fi
    sleep 1
  done
  fail "$label did not become ready at $url"
}

wait_for_url "backend" "$OPENCLARION_LOCAL_API_BASE_URL/healthz" "$backend_pid"

frontend_env=(
  "HOME=${HOME:-/tmp}"
  "PATH=$PATH"
  "PWD=$ROOT_DIR/web"
  "OPENCLARION_API_BASE_URL=$OPENCLARION_LOCAL_API_BASE_URL"
  "OPENCLARION_BROWSER_WS_BASE_URL=$OPENCLARION_LOCAL_API_BASE_URL"
  "NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL=$OPENCLARION_LOCAL_API_BASE_URL"
)
for name in \
  LANG LC_ALL TERM TZ FORCE_COLOR \
  NODE_OPTIONS NODE_EXTRA_CA_CERTS NEXT_TELEMETRY_DISABLED \
  SSL_CERT_FILE SSL_CERT_DIR HTTP_PROXY HTTPS_PROXY NO_PROXY \
  http_proxy https_proxy no_proxy \
  OIDC_ISSUER OIDC_CLIENT_ID OIDC_CLIENT_SECRET OIDC_CLIENT_AUTH_METHOD \
  OIDC_REDIRECT_URL OIDC_SCOPES OIDC_USE_PKCE OIDC_STATE_SIGNING_KEY \
  OPENCLARION_IAM_OIDC_ISSUER OPENCLARION_IAM_OIDC_CLIENT_ID \
  OPENCLARION_IAM_OIDC_CLIENT_SECRET OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD \
  OPENCLARION_IAM_OIDC_REDIRECT_URL OPENCLARION_IAM_OIDC_SCOPES \
  OPENCLARION_IAM_OIDC_USE_PKCE OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY \
  OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID \
  OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY; do
  if [[ -v "$name" ]]; then
    frontend_env+=("$name=${!name}")
  fi
done

(
  cd web
  exec setsid env -i "${frontend_env[@]}" \
    npm run dev -- --hostname "$OPENCLARION_LOCAL_WEB_HOST" --port "$OPENCLARION_LOCAL_WEB_PORT"
) &
frontend_pid="$!"

wait_for_url "frontend" "$web_url" "$frontend_pid"
echo "[local-product] frontend: $web_url" >&2
echo "[local-product] API: $OPENCLARION_LOCAL_API_BASE_URL" >&2
echo "[local-product] Temporal UI: http://127.0.0.1:${TEMPORAL_UI_PORT}" >&2

set +e
wait -n "$backend_pid" "$frontend_pid"
status="$?"
set -e
exit "$status"
