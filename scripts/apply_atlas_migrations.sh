#!/usr/bin/env bash
# Validate and apply the committed Atlas migration directory to one database.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

ATLAS_IMAGE="${ATLAS_IMAGE:-arigaio/atlas:1.2.0}"
database_url="${OPENCLARION_ATLAS_DATABASE_URL:-${DATABASE_URL:-}}"
docker_network="${OPENCLARION_ATLAS_DOCKER_NETWORK:-host}"
timeout_seconds="${OPENCLARION_ATLAS_TIMEOUT_SECONDS:-300}"

fail() {
  printf '[atlas-apply] %s\n' "$1" >&2
  exit 2
}

for tool in docker timeout; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done

[[ -n "$database_url" ]] || fail "set DATABASE_URL or OPENCLARION_ATLAS_DATABASE_URL"
[[ "$database_url" != *[[:space:]]* ]] || fail "database URL must not contain whitespace"
case "$database_url" in
  postgres://* | postgresql://*) ;;
  *) fail "database URL must use the postgres or postgresql scheme" ;;
esac
[[ "$docker_network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || \
  fail "Docker network name is invalid"
[[ "$docker_network" != "none" ]] || fail "Docker network must permit database access"
[[ "$ATLAS_IMAGE" =~ ^[a-z0-9][a-z0-9._:/-]*:[0-9]+\.[0-9]+\.[0-9]+$ ]] || \
  fail "ATLAS_IMAGE must use a lowercase repository and concrete semantic-version tag"
[[ "$timeout_seconds" =~ ^[0-9]+$ && ${#timeout_seconds} -le 4 ]] || \
  fail "OPENCLARION_ATLAS_TIMEOUT_SECONDS must be numeric"
timeout_value=$((10#$timeout_seconds))
((timeout_value >= 1 && timeout_value <= 1800)) || \
  fail "OPENCLARION_ATLAS_TIMEOUT_SECONDS must be between 1 and 1800"

docker network inspect "$docker_network" >/dev/null 2>&1 || fail "Docker network does not exist"

atlas() {
  ATLAS_DATABASE_URL="$database_url" timeout --foreground "${timeout_value}s" \
    docker run --rm \
    --network "$docker_network" \
    --read-only \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --tmpfs /tmp:rw,noexec,nosuid,nodev,size=64m,mode=1777 \
    --mount "type=bind,src=$ROOT_DIR,dst=/workspace,readonly" \
    --workdir /workspace \
    --env HOME=/tmp \
    --env ATLAS_DATABASE_URL \
    "$ATLAS_IMAGE" \
    "$@"
}

echo "[atlas-apply] validating migration directory integrity..." >&2
atlas migrate validate --env runtime

echo "[atlas-apply] applying pending migrations..." >&2
atlas migrate apply --env runtime

echo "[atlas-apply] checking final migration status..." >&2
atlas migrate status --env runtime
echo "[atlas-apply] OK - committed migrations are applied." >&2
