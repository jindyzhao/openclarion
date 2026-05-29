#!/usr/bin/env bash
# Run the manual M4 Docker ContainerProvider smoke against a real local Docker daemon.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[container-provider-smoke] required tool not found in PATH: $1" >&2
    exit 2
  }
}

require_tool docker
require_tool go

run_id="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_RUN_ID:-$$-${RANDOM:-0}}"
invocation_id="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_INVOCATION_ID:-container-provider-smoke-${run_id}}"
export OPENCLARION_CONTAINER_PROVIDER_SMOKE_INVOCATION_ID="$invocation_id"

image="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE:-}"
source_image="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_SOURCE_IMAGE:-busybox:1.36.1}"
pull_policy="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL:-missing}"

case "$pull_policy" in
  always|missing|never) ;;
  *)
    echo "[container-provider-smoke] OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL must be one of: always, missing, never" >&2
    exit 2
    ;;
esac

reject_latest() {
  local ref="$1"
  if [[ "$ref" =~ (^|[:/])latest($|@) ]]; then
    echo "[container-provider-smoke] image must not use latest: $ref" >&2
    exit 2
  fi
}

resolve_digest_ref() {
  local ref="$1"
  if [[ "$ref" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
    printf '%s\n' "$ref"
    return
  fi
  reject_latest "$ref"
  case "$pull_policy" in
    always)
      docker pull "$ref" >/dev/null
      ;;
    missing)
      docker image inspect "$ref" >/dev/null 2>&1 || docker pull "$ref" >/dev/null
      ;;
    never)
      docker image inspect "$ref" >/dev/null
      ;;
  esac
  local digest
  digest="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$ref" | sed -n '1p')"
  if [[ -z "$digest" ]]; then
    echo "[container-provider-smoke] could not resolve a repo digest for $ref" >&2
    exit 2
  fi
  printf '%s\n' "$digest"
}

if [[ -n "$image" ]]; then
  reject_latest "$image"
  if [[ ! "$image" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
    echo "[container-provider-smoke] OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE must be pinned by sha256 digest: $image" >&2
    exit 2
  fi
else
  echo "[container-provider-smoke] resolving default source image $source_image to a digest..." >&2
  image="$(resolve_digest_ref "$source_image")"
  export OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE="$image"
  if [[ -z "${OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON:-}" ]]; then
    export OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON='["sh","-c","test -r /workspace/evidence.json && test -r /workspace/conversation.json && test -r /workspace/message.json && test -r /workspace/agent_config/agent.yaml && cat /workspace/evidence.json > /workspace/out/output.json"]'
  fi
fi

echo "[container-provider-smoke] running Provider.Run with $image..." >&2
go run ./scripts/container_provider_smoke

leaked="$(docker ps -a --filter "label=openclarion.invocation_id=${invocation_id}" --format '{{.ID}}')"
if [[ -n "$leaked" ]]; then
  echo "[container-provider-smoke] leaked container(s) for invocation $invocation_id:" >&2
  printf '%s\n' "$leaked" >&2
  docker rm -f -v $leaked >/dev/null 2>&1 || true
  exit 1
fi

echo "[container-provider-smoke] OK - no containers leaked for invocation $invocation_id." >&2
