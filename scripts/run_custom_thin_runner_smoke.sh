#!/usr/bin/env bash
# Build and smoke-test the local custom thin runner runtime candidate.
#
# The image is pushed to an ephemeral localhost registry so the existing
# runtime/provider smoke harnesses exercise a real repo@sha256 reference.
# Set OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR to also collect the five
# canonical M4 runtime-smoke artifacts before the ephemeral registry is removed.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[custom-thin-runner-smoke] required tool not found in PATH: $1" >&2
    exit 2
  }
}

fail() {
  echo "[custom-thin-runner-smoke] $1" >&2
  exit 2
}

validate_single_line() {
  local label="$1"
  local value="$2"
  if [[ -z "$value" || "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
    fail "$label must be a non-empty single-line value"
  fi
}

validate_ignored_repo_output_path() {
  local label="$1"
  local path="$2"
  local path_abs=""
  local rel=""
  local tracked=""

  validate_single_line "$label" "$path"
  if ! path_abs="$(realpath -m -- "$path")"; then
    fail "$label path must be resolvable"
  fi

  case "$path_abs" in
    "$ROOT_DIR"/*|"$ROOT_DIR")
      rel="${path_abs#"$ROOT_DIR"/}"
      if ! git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        fail "$label repo-local output requires git ignore verification"
      fi
      tracked="$(git -C "$ROOT_DIR" ls-files -- "$rel" "$rel/" 2>/dev/null || true)"
      if [[ -n "$tracked" ]]; then
        fail "$label repo-local output must not overlap tracked files"
      fi
      if ! git -C "$ROOT_DIR" check-ignore -q -- "$rel"; then
        fail "$label repo-local output must be ignored by git"
      fi
      ;;
  esac
}

validate_ignored_repo_output_file() {
  local label="$1"
  local path="$2"

  validate_ignored_repo_output_path "$label" "$path"
  if [[ "$path" == */ || "$(basename "$path")" == "." || "$(basename "$path")" == ".." ]]; then
    fail "$label must name a file path"
  fi
}

require_tool docker
require_tool git
require_tool go
require_tool realpath

run_id="${OPENCLARION_CUSTOM_THIN_RUNNER_SMOKE_RUN_ID:-$$-${RANDOM:-0}}"
tmp_dir="$(mktemp -d -t openclarion-custom-thin-runner.XXXXXX)"
registry_image="${OPENCLARION_CUSTOM_THIN_RUNNER_REGISTRY_IMAGE:-registry:2@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373}"
registry_name="openclarion-custom-thin-runner-registry-${run_id}"
local_tag="openclarion/custom-thin-runner:smoke-${run_id}"
digest_ref_out="${OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT:-}"
artifacts_dir="${OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR:-}"
registry_cid=""

if [[ -n "$digest_ref_out" ]]; then
  validate_ignored_repo_output_file "OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT" "$digest_ref_out"
fi
if [[ -n "$artifacts_dir" ]]; then
  validate_ignored_repo_output_path "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR" "$artifacts_dir"
fi

cleanup() {
  if [[ -n "$registry_cid" ]]; then
    docker rm -f -v "$registry_cid" >/dev/null 2>&1 || true
  else
    docker rm -f -v "$registry_name" >/dev/null 2>&1 || true
  fi
  if [[ -n "${remote_tag:-}" ]]; then
    docker image rm -f "$remote_tag" >/dev/null 2>&1 || true
  fi
  docker image rm -f "$local_tag" >/dev/null 2>&1 || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

echo "[custom-thin-runner-smoke] building static runner binary..." >&2
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go build -trimpath -ldflags="-s -w" \
  -o "$tmp_dir/custom-thin-runner" ./scripts/custom_thin_runner
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go build -trimpath -ldflags="-s -w" \
  -o "$tmp_dir/agent_tool_metric_query" ./scripts/agent_tool_metric_query
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go build -trimpath -ldflags="-s -w" \
  -o "$tmp_dir/agent_tool_topology_lookup" ./scripts/agent_tool_topology_lookup
cp scripts/custom_thin_runner/Dockerfile "$tmp_dir/Dockerfile"

echo "[custom-thin-runner-smoke] building scratch candidate image..." >&2
docker build --pull=false -t "$local_tag" "$tmp_dir" >/dev/null

echo "[custom-thin-runner-smoke] starting ephemeral localhost registry..." >&2
registry_cid="$(docker run -d --name "$registry_name" -p 127.0.0.1::5000 "$registry_image")"
host_port="$(docker port "$registry_cid" 5000/tcp | sed -n 's/.*://p' | sed -n '1p')"
if [[ -z "$host_port" ]]; then
  echo "[custom-thin-runner-smoke] could not determine registry host port" >&2
  exit 1
fi

repository="localhost:${host_port}/openclarion/custom-thin-runner"
remote_tag="${repository}:smoke-${run_id}"
docker tag "$local_tag" "$remote_tag"

echo "[custom-thin-runner-smoke] pushing candidate image to $repository..." >&2
docker push "$remote_tag" >/dev/null

digest_ref=""
while IFS= read -r candidate; do
  case "$candidate" in
    "$repository"@sha256:*)
      digest_ref="$candidate"
      break
      ;;
  esac
done < <(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$remote_tag")

if [[ -z "$digest_ref" ]]; then
  digest="$(docker buildx imagetools inspect "$remote_tag" --format '{{.Digest}}')"
  digest_ref="${repository}@${digest}"
fi
if [[ ! "$digest_ref" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
  echo "[custom-thin-runner-smoke] could not resolve digest-pinned image ref: $digest_ref" >&2
  exit 1
fi
echo "[custom-thin-runner-smoke] resolved candidate image: $digest_ref" >&2

topology_file="$tmp_dir/topology.json"
cat >"$topology_file" <<'JSON'
{"services":[{"name":"payments","owner":"payments-team","tier":"backend","dependencies":["postgres"],"dependents":["checkout"]},{"name":"postgres","owner":"platform","tier":"data"},{"name":"checkout","owner":"checkout-team","tier":"edge"}]}
JSON

echo "[custom-thin-runner-smoke] proving packaged topology helper in $digest_ref..." >&2
docker run --rm \
  --pull never \
  --network none \
  --user 65532:65532 \
  --read-only \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --memory 128m \
  --cpus 1 \
  --pids-limit 64 \
  --mount "type=bind,source=${topology_file},target=/workspace/topology.json,readonly" \
  --entrypoint /tools/agent_tool_topology_lookup \
  "$digest_ref" \
  --topology-file /workspace/topology.json \
  --service payments \
  >"$tmp_dir/topology-output.json"
go run ./scripts/agent_tool_topology_lookup --topology-file "$topology_file" --service payments >/dev/null
go run ./scripts/agent_runtime_smoke_output "$tmp_dir/topology-output.json" >/dev/null

echo "[custom-thin-runner-smoke] running agent-runtime-smoke with $digest_ref..." >&2
env -u OPENCLARION_AGENT_RUNTIME_SHELL_COMMAND \
  OPENCLARION_AGENT_RUNTIME_IMAGE="$digest_ref" \
  OPENCLARION_AGENT_RUNTIME_PULL=missing \
  make agent-runtime-smoke

echo "[custom-thin-runner-smoke] running container-provider-smoke with $digest_ref..." >&2
env -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON \
  OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE="$digest_ref" \
  OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL=missing \
  make container-provider-smoke

if [[ -n "$artifacts_dir" ]]; then
  echo "[custom-thin-runner-smoke] collecting runtime-smoke artifacts in $artifacts_dir..." >&2
  env -u OPENCLARION_AGENT_RUNTIME_SHELL_COMMAND \
    -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON \
    OPENCLARION_AGENT_RUNTIME_IMAGE="$digest_ref" \
    OPENCLARION_M4_RUNTIME_SMOKE_PULL=missing \
    OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR="$artifacts_dir" \
    make sandbox-m4-runtime-smoke-artifacts
fi

if [[ -n "$digest_ref_out" ]]; then
  if [[ -e "$digest_ref_out" ]]; then
    echo "[custom-thin-runner-smoke] digest ref output path already exists: $digest_ref_out" >&2
    exit 2
  fi
  mkdir -p "$(dirname "$digest_ref_out")"
  (umask 077 && set -o noclobber && printf '%s\n' "$digest_ref" >"$digest_ref_out")
  echo "[custom-thin-runner-smoke] wrote digest ref: $digest_ref_out" >&2
fi

echo "[custom-thin-runner-smoke] OK - custom thin runner passed both digest-pinned smoke harnesses." >&2
