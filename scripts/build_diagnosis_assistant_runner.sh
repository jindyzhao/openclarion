#!/usr/bin/env bash
# Build the local diagnosis assistant runtime and leave an immutable digest
# reference in the Docker daemon. No Kubernetes or external runtime is needed.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

fail() {
  echo "[diagnosis-assistant-runner-build] $1" >&2
  exit 2
}

target_arch="${OPENCLARION_DIAGNOSIS_RUNNER_TARGET_ARCH:-amd64}"
case "$target_arch" in
  amd64 | arm64) ;;
  *) fail "target architecture must be amd64 or arm64" ;;
esac
target_platform="linux/${target_arch}"
registry_ready_timeout_seconds="${OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS:-30}"
if [[ ! "$registry_ready_timeout_seconds" =~ ^([1-9]|[1-9][0-9]|1[01][0-9]|120)$ ]]; then
  fail "registry readiness timeout must be an integer from 1 to 120 seconds"
fi

validate_output_file() {
  local path="$1"
  local path_abs=""
  local rel=""
  local tracked=""

  if [[ -z "$path" || "$path" == *$'\n'* || "$path" == *$'\r'* ]]; then
    fail "digest ref output must be a non-empty single-line path"
  fi
  if [[ "$path" == */ || "$(basename -- "$path")" == "." || "$(basename -- "$path")" == ".." ]]; then
    fail "digest ref output must name a file path"
  fi
  path_abs="$(realpath -m -- "$path")" || fail "digest ref output path must be resolvable"
  case "$path_abs" in
    "$ROOT_DIR"/*)
      rel="${path_abs#"$ROOT_DIR"/}"
      tracked="$(git ls-files -- "$rel" "$rel/" 2>/dev/null || true)"
      [[ -z "$tracked" ]] || fail "repo-local digest ref output must not overlap tracked files"
      git check-ignore -q -- "$rel" || fail "repo-local digest ref output must be ignored by git"
      ;;
  esac
  if [[ -L "$path" ]]; then
    fail "digest ref output must not be a symlink"
  fi
  if [[ -e "$path" && ! -f "$path" ]]; then
    fail "digest ref output must be a regular file when it already exists"
  fi
}

for tool in curl docker git go grep realpath sed; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool not found in PATH: $tool"
done

output_file="${OPENCLARION_DIAGNOSIS_RUNNER_DIGEST_REF_OUT:-$ROOT_DIR/.openclarion-private/diagnosis-assistant-runner.digest-ref}"
validate_output_file "$output_file"

ca_bundle=""
for candidate in \
  /etc/ssl/certs/ca-certificates.crt \
  /etc/pki/tls/certs/ca-bundle.crt \
  /etc/ssl/ca-bundle.pem; do
  if [[ -f "$candidate" && ! -L "$candidate" && -s "$candidate" ]]; then
    ca_bundle="$candidate"
    break
  fi
done
[[ -n "$ca_bundle" ]] || fail "a direct non-empty system CA bundle is required"

run_id="${OPENCLARION_DIAGNOSIS_RUNNER_BUILD_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
if [[ ! "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$ ]]; then
  fail "build id must match [A-Za-z0-9][A-Za-z0-9._-]{0,79}"
fi

registry_name="openclarion-diagnosis-runner-registry-${run_id}"
local_tag="openclarion/diagnosis-assistant-runner:local-${run_id}"
docker info >/dev/null 2>&1 || fail "Docker daemon is unavailable"
if docker container inspect "$registry_name" >/dev/null 2>&1; then
  fail "build id already names an existing container: $registry_name"
fi
if docker image inspect "$local_tag" >/dev/null 2>&1; then
  fail "build id already names an existing image tag: $local_tag"
fi

tmp_dir="$(mktemp -d -t openclarion-diagnosis-runner.XXXXXX)"
registry_image="${OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_IMAGE:-registry:2@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373}"
registry_cid=""
remote_tag=""
tmp_output=""
local_image_created=0

cleanup() {
  if [[ -n "$registry_cid" ]]; then
    docker rm -f -v "$registry_cid" >/dev/null 2>&1 || true
  fi
  if ((local_image_created == 1)); then
    docker image rm "$local_tag" >/dev/null 2>&1 || true
  fi
  if [[ -n "$tmp_output" ]]; then
    rm -f "$tmp_output"
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

echo "[diagnosis-assistant-runner-build] building static binaries for ${target_platform}..." >&2
CGO_ENABLED=0 GOOS=linux GOARCH="$target_arch" \
  GOWORK=off go -C scripts/diagnosis_assistant_runner build \
  -trimpath -ldflags='-s -w' -o "$tmp_dir/diagnosis-assistant-runner" .
chmod 0755 "$tmp_dir/diagnosis-assistant-runner"
echo "[diagnosis-assistant-runner-build] collecting third-party license files..." >&2
(
  cd scripts/diagnosis_assistant_runner
  GOWORK=off go run github.com/google/go-licenses@v1.6.0 save \
    --ignore=github.com/openclarion/openclarion \
    --save_path="$tmp_dir/third_party_licenses" .
)
cp scripts/diagnosis_assistant_runner/Dockerfile "$tmp_dir/Dockerfile"
cp scripts/diagnosis_assistant_runner/THIRD_PARTY_NOTICES.txt "$tmp_dir/THIRD_PARTY_NOTICES.txt"
cp LICENSE "$tmp_dir/Apache-2.0.txt"
cp "$ca_bundle" "$tmp_dir/ca-certificates.crt"
chmod 0644 "$tmp_dir/Apache-2.0.txt" "$tmp_dir/THIRD_PARTY_NOTICES.txt"
chmod 0644 "$tmp_dir/ca-certificates.crt"

docker build --pull=false --platform "$target_platform" -t "$local_tag" "$tmp_dir" >/dev/null
local_image_created=1
local_platform="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$local_tag")"
[[ "$local_platform" == "$target_platform" ]] || \
  fail "local image platform ${local_platform:-unknown} does not match ${target_platform}"

echo "[diagnosis-assistant-runner-build] resolving immutable repository digest..." >&2
registry_cid="$(docker run -d --name "$registry_name" -p 127.0.0.1::5000 "$registry_image")"
host_port="$(docker port "$registry_cid" 5000/tcp | sed -n 's/.*://p' | sed -n '1p')"
[[ "$host_port" =~ ^[0-9]+$ ]] || fail "could not determine temporary registry port"
registry_headers="$tmp_dir/registry-headers"
registry_ready=0
registry_ready_deadline=$((SECONDS + registry_ready_timeout_seconds))
while ((SECONDS < registry_ready_deadline)); do
  : >"$registry_headers"
  if curl --proto '=http' --noproxy '*' --fail --silent --show-error \
    --connect-timeout 1 --max-time 1 --dump-header "$registry_headers" \
    --output /dev/null "http://127.0.0.1:${host_port}/v2/" 2>/dev/null && \
    grep -Eiq '^Docker-Distribution-Api-Version:[[:space:]]*registry/2\.0[[:space:]]*$' "$registry_headers"; then
    registry_ready=1
    break
  fi
  sleep 1
done
if ((registry_ready != 1)); then
  fail "temporary registry did not become ready within ${registry_ready_timeout_seconds}s"
fi
repository="localhost:${host_port}/openclarion/diagnosis-assistant-runner"
remote_tag="${repository}:local-${run_id}"
docker tag "$local_tag" "$remote_tag"
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

if [[ ! "$digest_ref" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]]; then
  fail "could not resolve a lowercase repository digest"
fi
digest_platform="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$digest_ref")"
[[ "$digest_platform" == "$target_platform" ]] || \
  fail "digest image platform ${digest_platform:-unknown} does not match ${target_platform}"

output_dir="$(dirname -- "$output_file")"
mkdir -p "$output_dir"
tmp_output="$(mktemp "$output_dir/.diagnosis-assistant-runner.digest-ref.XXXXXX")"
chmod 0600 "$tmp_output"
printf '%s\n' "$digest_ref" >"$tmp_output"
mv -f "$tmp_output" "$output_file"
tmp_output=""

echo "[diagnosis-assistant-runner-build] local immutable image is ready." >&2
echo "[diagnosis-assistant-runner-build] image platform: $target_platform" >&2
echo "[diagnosis-assistant-runner-build] digest ref file: $output_file" >&2
printf '%s\n' "$digest_ref"
