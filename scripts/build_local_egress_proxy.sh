#!/usr/bin/env bash
# Build the local sandbox egress proxy as a minimal scratch image.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

fail() {
  printf '[local-egress-proxy-build] %s\n' "$1" >&2
  exit 2
}

validate_image_ref() {
  local ref="$1"
  if [[ -z "$ref" || "$ref" == *$'\n'* || "$ref" == *$'\r'* || "$ref" =~ [[:space:]] ]]; then
    fail "image must be a non-empty single-line reference without whitespace"
  fi
  if [[ "$ref" == *://* || "$ref" == /* || "$ref" == */ || "$ref" == *"//"* || "$ref" == *@* ]]; then
    fail "image must be a tag reference without a URL scheme, digest, or empty path component"
  fi
  local last_component="${ref##*/}"
  if [[ "$last_component" != *:* ]]; then
    fail "image must include an explicit non-latest tag"
  fi
  local tag="${last_component##*:}"
  if [[ -z "$tag" || "$tag" == "latest" ]]; then
    fail "image tag must be explicit and must not be latest"
  fi
  if [[ ! "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
    fail "image tag contains invalid characters"
  fi
  local repository="${ref%:*}"
  if [[ "$repository" =~ [A-Z] || ! "$repository" =~ ^[a-z0-9._:/-]+$ ]]; then
    fail "image repository must use lowercase image-reference characters"
  fi
}

for tool in docker go; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool not found in PATH: $tool"
done

image="${OPENCLARION_LOCAL_EGRESS_PROXY_IMAGE:-openclarion/local-egress-proxy:dev}"
validate_image_ref "$image"

tmp_dir="$(mktemp -d -t openclarion-egress-proxy.XXXXXX)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

echo "[local-egress-proxy-build] building static proxy binary..." >&2
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go build -trimpath -ldflags='-s -w' \
  -o "$tmp_dir/openclarion-egress-proxy" ./cmd/openclarion-egress-proxy
chmod 0755 "$tmp_dir/openclarion-egress-proxy"
cp deploy/local-egress-proxy/Dockerfile "$tmp_dir/Dockerfile"
docker build --pull=false -t "$image" "$tmp_dir" >/dev/null
echo "[local-egress-proxy-build] image ready: $image" >&2
printf '%s\n' "$image"
