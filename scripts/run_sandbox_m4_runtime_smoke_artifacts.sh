#!/usr/bin/env bash
# Collect the retained runtime-smoke artifacts required by the M4 review
# evidence and packet helpers.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[sandbox-m4-runtime-smoke-artifacts] required tool not found in PATH: $1" >&2
    exit 2
  }
}

require_tool docker
require_tool go
require_tool make

artifacts_dir="${OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR:-}"
runtime_image="${OPENCLARION_AGENT_RUNTIME_IMAGE:-}"
runtime_pull="${OPENCLARION_M4_RUNTIME_SMOKE_PULL:-${OPENCLARION_AGENT_RUNTIME_PULL:-missing}}"

if [[ -z "$artifacts_dir" || -z "$runtime_image" ]]; then
  cat >&2 <<'EOF'
[sandbox-m4-runtime-smoke-artifacts] missing required configuration.
Set:
  OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=<empty-artifact-dir>
  OPENCLARION_AGENT_RUNTIME_IMAGE=<candidate-image@sha256:64-lowercase-hex-digest>

Optional:
  OPENCLARION_M4_RUNTIME_SMOKE_PULL=always|missing|never
  OPENCLARION_AGENT_RUNTIME_SHELL_COMMAND=... # only for generic shell-capable smoke images
  OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON='[...]' # only for generic shell-capable smoke images
EOF
  exit 2
fi

case "$runtime_pull" in
  always|missing|never) ;;
  *)
    echo "[sandbox-m4-runtime-smoke-artifacts] OPENCLARION_M4_RUNTIME_SMOKE_PULL must be one of: always, missing, never" >&2
    exit 2
    ;;
esac

if [[ ! "$runtime_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]]; then
  echo "[sandbox-m4-runtime-smoke-artifacts] OPENCLARION_AGENT_RUNTIME_IMAGE must be pinned by lowercase sha256 digest: $runtime_image" >&2
  exit 2
fi

if [[ -e "$artifacts_dir" ]]; then
  if [[ -L "$artifacts_dir" || ! -d "$artifacts_dir" ]]; then
    echo "[sandbox-m4-runtime-smoke-artifacts] artifact output path must be a direct directory: $artifacts_dir" >&2
    exit 2
  fi
  if [[ -n "$(find "$artifacts_dir" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    echo "[sandbox-m4-runtime-smoke-artifacts] artifact output directory must be empty: $artifacts_dir" >&2
    exit 2
  fi
else
  mkdir -p "$artifacts_dir"
fi

agent_runtime_artifact="$artifacts_dir/agent-runtime-smoke.json"
provider_lifecycle_artifact="$artifacts_dir/container-provider-smoke.json"
provider_timeout_artifact="$artifacts_dir/container-provider-timeout-smoke.json"
provider_output_cap_artifact="$artifacts_dir/container-provider-output-cap-smoke.json"
egress_artifact="$artifacts_dir/egress-allowdeny-smoke.json"

echo "[sandbox-m4-runtime-smoke-artifacts] running agent-runtime-smoke..." >&2
OPENCLARION_AGENT_RUNTIME_IMAGE="$runtime_image" \
OPENCLARION_AGENT_RUNTIME_PULL="$runtime_pull" \
OPENCLARION_AGENT_RUNTIME_PROOF_PATH="$agent_runtime_artifact" \
  make agent-runtime-smoke

echo "[sandbox-m4-runtime-smoke-artifacts] running container-provider-smoke..." >&2
OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE="$runtime_image" \
OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL="$runtime_pull" \
OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_PATH="$provider_lifecycle_artifact" \
  make container-provider-smoke

echo "[sandbox-m4-runtime-smoke-artifacts] running container-provider-timeout-smoke..." >&2
env -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_EXPECT_ERROR_CONTAINS \
  OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_PATH="$provider_timeout_artifact" \
  make container-provider-timeout-smoke

echo "[sandbox-m4-runtime-smoke-artifacts] running container-provider-output-cap-smoke..." >&2
env -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_PULL \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON \
  -u OPENCLARION_CONTAINER_PROVIDER_SMOKE_EXPECT_ERROR_CONTAINS \
  OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_PATH="$provider_output_cap_artifact" \
  make container-provider-output-cap-smoke

echo "[sandbox-m4-runtime-smoke-artifacts] running egress-allowdeny-smoke..." >&2
OPENCLARION_EGRESS_SMOKE_PROOF_PATH="$egress_artifact" \
  make egress-allowdeny-smoke

expected_artifacts=(
  "$agent_runtime_artifact"
  "$provider_lifecycle_artifact"
  "$provider_timeout_artifact"
  "$provider_output_cap_artifact"
  "$egress_artifact"
)

for artifact in "${expected_artifacts[@]}"; do
  if [[ ! -s "$artifact" ]]; then
    echo "[sandbox-m4-runtime-smoke-artifacts] expected artifact was not written or is empty: $artifact" >&2
    exit 1
  fi
done

echo "[sandbox-m4-runtime-smoke-artifacts] OK - retained runtime-smoke artifacts:" >&2
printf '[sandbox-m4-runtime-smoke-artifacts] %s\n' "${expected_artifacts[@]}" >&2
