#!/usr/bin/env bash
# Build and optionally publish the OpenClarion service image.
#
# The image uses a scratch runtime and a statically linked service binary. The
# build context is created in a private temporary directory so private env files
# and retained live-proof artifacts cannot enter the image accidentally.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

image_ref="${OPENCLARION_SERVICE_IMAGE_REF:-}"
digest_ref_out="${OPENCLARION_SERVICE_IMAGE_DIGEST_REF_OUT:-}"
proof_out="${OPENCLARION_SERVICE_IMAGE_PROOF_OUT:-}"
goarch="${OPENCLARION_SERVICE_IMAGE_GOARCH:-}"
push=""

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_openclarion_service_image_build.sh --image-ref REF [--push] [--digest-ref-out PATH] [--proof-out PATH] [--goarch ARCH]

REF must be an explicit, non-latest tag such as:
  harbor.example.test/openclarion/openclarion:20260618-abcdef0

Use the registry host form, not a URL. For example, use
harbor.example.test/openclarion/openclarion:20260618-abcdef0 instead of
https://harbor.example.test/openclarion/openclarion:20260618-abcdef0.

Use --push only after docker login has been performed outside this script.
When --push is set, the script resolves and prints the immutable
repository@sha256:<digest> reference accepted by deployment manifests.
When --proof-out is set, the script also writes a strict, secret-free JSON proof
that binds the tag, digest ref, local RepoDigest, and remote manifest metadata.
EOF
}

fail() {
  printf '[openclarion-service-image] %s\n' "$1" >&2
  exit 2
}

while (($# > 0)); do
  case "$1" in
    --image-ref)
      if (($# < 2)); then
        usage
        exit 2
      fi
      image_ref="$2"
      shift 2
      ;;
    --push)
      push="1"
      shift
      ;;
    --digest-ref-out)
      if (($# < 2)); then
        usage
        exit 2
      fi
      digest_ref_out="$2"
      shift 2
      ;;
    --proof-out)
      if (($# < 2)); then
        usage
        exit 2
      fi
      proof_out="$2"
      shift 2
      ;;
    --goarch)
      if (($# < 2)); then
        usage
        exit 2
      fi
      goarch="$2"
      shift 2
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

require_tool() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool not found in PATH: $1"
}

validate_single_line() {
  local label="$1"
  local value="$2"
  if [[ -z "$value" || "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
    fail "$label must be a non-empty single-line value"
  fi
}

validate_image_ref() {
  validate_single_line "OPENCLARION_SERVICE_IMAGE_REF" "$image_ref"
  if [[ "$image_ref" =~ [[:space:]] ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF must not contain whitespace"
  fi
  if [[ "$image_ref" == *://* ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF must be an image reference, not a URL; omit schemes such as https://"
  fi
  if [[ "$image_ref" == /* || "$image_ref" == */ || "$image_ref" == *"//"* ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF must not start with '/', end with '/', or contain empty path components"
  fi
  if [[ "$image_ref" == *@* ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF must be a tag reference for build/push, not a digest reference"
  fi
  local last_component="${image_ref##*/}"
  if [[ "$last_component" != *:* ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF must include an explicit non-latest tag"
  fi
  local tag="${last_component##*:}"
  if [[ -z "$tag" || "$tag" == "latest" ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF tag must be explicit and must not be latest"
  fi
  if [[ ! "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF tag contains invalid characters"
  fi
  local repository="${image_ref%:*}"
  if [[ "$repository" =~ [A-Z] || ! "$repository" =~ ^[a-z0-9._:/-]+$ ]]; then
    fail "OPENCLARION_SERVICE_IMAGE_REF repository must use lowercase image-reference characters"
  fi
  if [[ -n "$push" ]]; then
    local registry="${image_ref%%/*}"
    if [[ "$registry" == "$image_ref" ||
          ( "$registry" != *.* && "$registry" != *:* && "$registry" != "localhost" ) ]]; then
      fail "pushed OPENCLARION_SERVICE_IMAGE_REF must include an explicit registry host"
    fi
  fi
}

validate_goarch() {
  if [[ -z "$goarch" ]]; then
    goarch="$(go env GOARCH)"
  fi
  validate_single_line "OPENCLARION_SERVICE_IMAGE_GOARCH" "$goarch"
  case "$goarch" in
    amd64|arm64)
      ;;
    *)
      fail "OPENCLARION_SERVICE_IMAGE_GOARCH must be amd64 or arm64"
      ;;
  esac
}

resolve_repository_from_tag() {
  local ref="$1"
  printf '%s\n' "${ref%:*}"
}

resolve_pushed_digest_ref() {
  local ref="$1"
  local repository="$2"
  local digest_ref=""
  local digest=""

  while IFS= read -r candidate; do
    case "$candidate" in
      "$repository"@sha256:*)
        digest_ref="$candidate"
        break
        ;;
    esac
  done < <(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$ref" 2>/dev/null || true)

  if [[ -z "$digest_ref" ]]; then
    digest="$(docker buildx imagetools inspect "$ref" --format '{{.Digest}}' 2>/dev/null || true)"
    if [[ "$digest" == sha256:* ]]; then
      digest_ref="${repository}@${digest}"
    fi
  fi

  if [[ ! "$digest_ref" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
    fail "could not resolve pushed image digest reference"
  fi
  printf '%s\n' "$digest_ref"
}

write_digest_ref() {
  local path="$1"
  local value="$2"
  validate_private_output_path "OPENCLARION_SERVICE_IMAGE_DIGEST_REF_OUT" "$path"
  if [[ -e "$path" ]]; then
    fail "digest ref output path already exists"
  fi
  mkdir -p "$(dirname "$path")"
  (umask 077 && set -o noclobber && printf '%s\n' "$value" >"$path")
}

json_string() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\t'/\\t}"
  printf '"%s"' "$value"
}

write_push_proof() {
  local path="$1"
  local tag_ref="$2"
  local digest_ref="$3"
  local digest_file="$4"
  local raw_manifest=""
  local repo_digests=""
  local checked_at=""

  validate_private_output_path "OPENCLARION_SERVICE_IMAGE_PROOF_OUT" "$path"
  if [[ -e "$path" ]]; then
    fail "proof output path already exists"
  fi

  raw_manifest="$(docker buildx imagetools inspect --raw "$digest_ref" 2>/dev/null)" ||
    fail "could not inspect pushed image manifest"
  repo_digests="$(docker image inspect --format '{{json .RepoDigests}}' "$tag_ref" 2>/dev/null)" ||
    fail "could not inspect local image repo digests"
  checked_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  mkdir -p "$(dirname "$path")"
  (
    umask 077
    set -o noclobber
    {
      printf '{\n'
      printf '  "proof_type": "openclarion_service_image_push",\n'
      printf '  "status": "pass",\n'
      printf '  "checked_at": %s,\n' "$(json_string "$checked_at")"
      printf '  "image_tag": %s,\n' "$(json_string "$tag_ref")"
      printf '  "digest_ref": %s,\n' "$(json_string "$digest_ref")"
      if [[ -n "$digest_file" ]]; then
        printf '  "digest_ref_file": %s,\n' "$(json_string "$digest_file")"
      else
        printf '  "digest_ref_file": null,\n'
      fi
      printf '  "git_revision": %s,\n' "$(json_string "$revision")"
      printf '  "goos": "linux",\n'
      printf '  "goarch": %s,\n' "$(json_string "$goarch")"
      printf '  "remote_manifest": %s,\n' "$raw_manifest"
      printf '  "local_repo_digests": %s,\n' "$repo_digests"
      printf '  "secret_values_retained": false\n'
      printf '}\n'
    } >"$path"
  )
}

validate_private_output_path() {
  local label="$1"
  local path="$2"
  local path_abs=""
  local rel=""

  validate_single_line "$label" "$path"
  if [[ "$path" == */ || "$(basename "$path")" == "." || "$(basename "$path")" == ".." ]]; then
    fail "$label must name a file path"
  fi
  if ! path_abs="$(realpath -m -- "$path")"; then
    fail "$label path must be resolvable"
  fi

  case "$path_abs" in
    "$ROOT_DIR"/*|"$ROOT_DIR")
      rel="${path_abs#"$ROOT_DIR"/}"
      case "$rel" in
        .openclarion-private/*)
          ;;
        *)
          fail "$label must live outside the repository or under .openclarion-private/"
          ;;
      esac
      if ! git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        fail "$label repo-local output requires git ignore verification"
      fi
      if git -C "$ROOT_DIR" ls-files --error-unmatch -- "$rel" >/dev/null 2>&1; then
        fail "$label repo-local output must not be tracked by git"
      fi
      if ! git -C "$ROOT_DIR" check-ignore -q -- "$rel"; then
        fail "$label repo-local output must be ignored by git"
      fi
      ;;
  esac
}

validate_image_ref
require_tool docker
require_tool go
require_tool realpath
validate_goarch

if [[ -n "$digest_ref_out" && -z "$push" ]]; then
  fail "--digest-ref-out requires --push"
fi
if [[ -n "$proof_out" && -z "$push" ]]; then
  fail "--proof-out requires --push"
fi

ca_cert_file="${OPENCLARION_SERVICE_IMAGE_CA_CERT_FILE:-/etc/ssl/certs/ca-certificates.crt}"
if [[ ! -f "$ca_cert_file" || ! -r "$ca_cert_file" ]]; then
  fail "OPENCLARION_SERVICE_IMAGE_CA_CERT_FILE must point at a readable CA bundle file"
fi

tmp_dir="$(mktemp -d -t openclarion-service-image.XXXXXX)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

revision="$(git rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')"

echo "[openclarion-service-image] building static OpenClarion binary for linux/${goarch}..." >&2
CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" \
  go build -trimpath -ldflags="-s -w" \
  -o "$tmp_dir/openclarion" ./cmd/openclarion
chmod 0755 "$tmp_dir/openclarion"
cp "$ca_cert_file" "$tmp_dir/ca-certificates.crt"

cat >"$tmp_dir/Dockerfile" <<'EOF'
FROM scratch

COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY openclarion /openclarion

USER 65532:65532
ENTRYPOINT ["/openclarion"]
CMD ["serve"]
EOF

echo "[openclarion-service-image] building scratch service image..." >&2
docker build \
  --pull=false \
  --label "org.opencontainers.image.title=OpenClarion" \
  --label "org.opencontainers.image.revision=${revision}" \
  --label "org.opencontainers.image.source=https://github.com/openclarion/openclarion" \
  -t "$image_ref" \
  "$tmp_dir" >/dev/null

if [[ -z "$push" ]]; then
  echo "[openclarion-service-image] OK - built $image_ref" >&2
  exit 0
fi

repository="$(resolve_repository_from_tag "$image_ref")"
echo "[openclarion-service-image] pushing service image to $repository..." >&2
docker push "$image_ref" >/dev/null
digest_ref="$(resolve_pushed_digest_ref "$image_ref" "$repository")"

if [[ -n "$digest_ref_out" ]]; then
  write_digest_ref "$digest_ref_out" "$digest_ref"
fi
if [[ -n "$proof_out" ]]; then
  write_push_proof "$proof_out" "$image_ref" "$digest_ref" "$digest_ref_out"
fi

echo "[openclarion-service-image] OK - pushed $digest_ref" >&2
