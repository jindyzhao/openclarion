#!/usr/bin/env bash
# Run the manual M4 agent-runtime adapter smoke against a candidate sandbox image.
#
# This is intentionally NOT part of make ci: it requires a digest-pinned
# candidate image that already implements the ADR-0013 entrypoint. Runtime
# family names belong in operator evidence and sandbox image contexts, not in
# this harness.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

image="${OPENCLARION_AGENT_RUNTIME_IMAGE:-}"
if [[ -z "$image" ]]; then
  cat >&2 <<'EOF'
[agent-runtime-smoke] missing OPENCLARION_AGENT_RUNTIME_IMAGE.
Set it to a digest-pinned candidate image, for example:
  OPENCLARION_AGENT_RUNTIME_IMAGE='registry.example.com/openclarion/agent@sha256:<64-hex-digest>' make agent-runtime-smoke

The image entrypoint must implement the ADR-0013 contract:
  read  /workspace/evidence.json
  read  /workspace/conversation.json
  read  /workspace/message.json
  read  /workspace/agent_config/
  write /workspace/out/output.json
EOF
  exit 2
fi

if [[ ! "$image" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
  echo "[agent-runtime-smoke] image must be pinned by sha256 digest: $image" >&2
  exit 2
fi

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[agent-runtime-smoke] required tool not found in PATH: $1" >&2
    exit 2
  }
}

require_tool docker
require_tool go
require_tool timeout

run_id="${OPENCLARION_AGENT_RUNTIME_RUN_ID:-$$-${RANDOM:-0}}"
container_name="openclarion-agent-runtime-smoke-${run_id}"
workdir="${OPENCLARION_AGENT_RUNTIME_WORKDIR:-$(mktemp -d -t openclarion-agent-runtime-smoke.XXXXXX)}"
timeout_seconds="${OPENCLARION_AGENT_RUNTIME_TIMEOUT_SECONDS:-60}"
memory="${OPENCLARION_AGENT_RUNTIME_MEMORY:-512m}"
cpus="${OPENCLARION_AGENT_RUNTIME_CPUS:-1}"
pids_limit="${OPENCLARION_AGENT_RUNTIME_PIDS_LIMIT:-256}"
output_max_bytes="${OPENCLARION_AGENT_RUNTIME_OUTPUT_MAX_BYTES:-10485760}"
pull_policy="${OPENCLARION_AGENT_RUNTIME_PULL:-missing}"
shell_command="${OPENCLARION_AGENT_RUNTIME_SHELL_COMMAND:-}"
command_args=()
if [[ -n "$shell_command" ]]; then
  command_args=(sh -c "$shell_command")
fi

case "$pull_policy" in
  always|missing|never) ;;
  *)
    echo "[agent-runtime-smoke] OPENCLARION_AGENT_RUNTIME_PULL must be one of: always, missing, never" >&2
    exit 2
    ;;
esac

if ! [[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]]; then
  echo "[agent-runtime-smoke] OPENCLARION_AGENT_RUNTIME_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 2
fi
if ! [[ "$output_max_bytes" =~ ^[1-9][0-9]*$ ]]; then
  echo "[agent-runtime-smoke] OPENCLARION_AGENT_RUNTIME_OUTPUT_MAX_BYTES must be a positive integer" >&2
  exit 2
fi

mkdir -p "$workdir"/agent_config "$workdir"/out
chmod 0777 "$workdir"/out

evidence_path="${OPENCLARION_AGENT_RUNTIME_EVIDENCE_PATH:-$workdir/evidence.json}"
conversation_path="${OPENCLARION_AGENT_RUNTIME_CONVERSATION_PATH:-$workdir/conversation.json}"
message_path="${OPENCLARION_AGENT_RUNTIME_MESSAGE_PATH:-$workdir/message.json}"
agent_config_dir="${OPENCLARION_AGENT_RUNTIME_AGENT_CONFIG_DIR:-$workdir/agent_config}"
output_path="${OPENCLARION_AGENT_RUNTIME_OUTPUT_PATH:-$workdir/output.json}"
proof_path="${OPENCLARION_AGENT_RUNTIME_PROOF_PATH:-$workdir/agent-runtime-smoke-proof.json}"
output_mount_dir="$workdir/out"

if [[ -z "${OPENCLARION_AGENT_RUNTIME_EVIDENCE_PATH:-}" ]]; then
  cat >"$evidence_path" <<'EOF'
{"snapshot_id":1,"alerts":[{"labels":{"alertname":"RuntimeSmoke"},"annotations":{"summary":"runtime adapter smoke"}}]}
EOF
fi
if [[ -z "${OPENCLARION_AGENT_RUNTIME_CONVERSATION_PATH:-}" ]]; then
  printf '[]\n' >"$conversation_path"
fi
if [[ -z "${OPENCLARION_AGENT_RUNTIME_MESSAGE_PATH:-}" ]]; then
  cat >"$message_path" <<'EOF'
{"role":"user","content":"Return a compact JSON object proving the runtime adapter can read the mounted inputs."}
EOF
fi
if [[ -z "${OPENCLARION_AGENT_RUNTIME_AGENT_CONFIG_DIR:-}" ]]; then
  cat >"$agent_config_dir/agent.yaml" <<'EOF'
name: runtime-smoke
mode: file-contract-smoke
output_path: /workspace/out/output.json
EOF
fi

evidence_path="$(realpath "$evidence_path")"
conversation_path="$(realpath "$conversation_path")"
message_path="$(realpath "$message_path")"
agent_config_dir="$(realpath "$agent_config_dir")"
output_path="$(realpath -m "$output_path")"
proof_path="$(realpath -m "$proof_path")"

for path in "$evidence_path" "$conversation_path" "$message_path"; do
  if [[ ! -f "$path" ]]; then
    echo "[agent-runtime-smoke] required input file does not exist: $path" >&2
    exit 2
  fi
done
if [[ ! -d "$agent_config_dir" ]]; then
  echo "[agent-runtime-smoke] agent config directory does not exist: $agent_config_dir" >&2
  exit 2
fi

cid=""
cleanup() {
  if [[ -n "$cid" ]]; then
    docker rm -f -v "$cid" >/dev/null 2>&1 || true
  else
    docker rm -f -v "$container_name" >/dev/null 2>&1 || true
  fi
  if [[ -z "${OPENCLARION_AGENT_RUNTIME_WORKDIR:-}" ]]; then
    rm -rf "$workdir"
  fi
}
trap cleanup EXIT

echo "[agent-runtime-smoke] creating candidate container from $image..." >&2
cid="$(docker create \
  --name "$container_name" \
  --label openclarion.smoke=agent-runtime \
  --pull "$pull_policy" \
  --network none \
  --user 65532:65532 \
  --workdir /workspace \
  --read-only \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --memory "$memory" \
  --cpus "$cpus" \
  --pids-limit "$pids_limit" \
  --ulimit "fsize=${output_max_bytes}:${output_max_bytes}" \
  --mount "type=bind,source=${output_mount_dir},target=/workspace/out" \
  --mount "type=bind,source=${evidence_path},target=/workspace/evidence.json,readonly" \
  --mount "type=bind,source=${conversation_path},target=/workspace/conversation.json,readonly" \
  --mount "type=bind,source=${message_path},target=/workspace/message.json,readonly" \
  --mount "type=bind,source=${agent_config_dir},target=/workspace/agent_config,readonly" \
  "$image" "${command_args[@]}")"

echo "[agent-runtime-smoke] starting $cid..." >&2
docker start "$cid" >/dev/null

set +e
wait_output="$(timeout "${timeout_seconds}s" docker wait "$cid" 2>&1)"
wait_status=$?
set -e

if [[ $wait_status -eq 124 ]]; then
  echo "[agent-runtime-smoke] timeout after ${timeout_seconds}s; stopping candidate container..." >&2
  docker stop --time 2 "$cid" >/dev/null 2>&1 || docker kill "$cid" >/dev/null 2>&1 || true
  exit 1
fi
if [[ $wait_status -ne 0 ]]; then
  echo "[agent-runtime-smoke] docker wait failed: $wait_output" >&2
  exit 1
fi

exit_code="$(printf '%s\n' "$wait_output" | tail -n 1)"
if [[ "$exit_code" != "0" ]]; then
  echo "[agent-runtime-smoke] candidate exited with code $exit_code" >&2
  if [[ "${OPENCLARION_AGENT_RUNTIME_SHOW_LOGS:-}" == "1" ]]; then
    docker logs --tail 200 "$cid" >&2 || true
  else
    echo "[agent-runtime-smoke] logs are suppressed by default to avoid leaking candidate credentials." >&2
    echo "[agent-runtime-smoke] set OPENCLARION_AGENT_RUNTIME_SHOW_LOGS=1 only in a controlled shell." >&2
  fi
  exit 1
fi

mkdir -p "$(dirname "$output_path")"
mkdir -p "$(dirname "$proof_path")"
docker cp "$cid:/workspace/out/output.json" "$output_path" >/dev/null
go run ./scripts/agent_runtime_smoke_output \
  --runtime-candidate "$image" \
  --source "make agent-runtime-smoke" \
  --output-max-bytes "$output_max_bytes" \
  --proof "$proof_path" \
  "$output_path"

echo "[agent-runtime-smoke] OK - candidate output: $output_path" >&2
echo "[agent-runtime-smoke] OK - smoke proof: $proof_path" >&2
