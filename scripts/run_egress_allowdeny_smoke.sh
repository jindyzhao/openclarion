#!/usr/bin/env bash
# Run the manual M4 Docker egress allow/deny smoke.
#
# This is intentionally NOT part of make ci: it requires a local Docker daemon
# and proves a concrete proxy-style topology rather than a pure unit contract.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[egress-allowdeny-smoke] required tool not found in PATH: $1" >&2
    exit 2
  }
}

require_tool docker
require_tool go

image="${OPENCLARION_EGRESS_SMOKE_IMAGE:-busybox:1.36.1}"
pull_policy="${OPENCLARION_EGRESS_SMOKE_PULL:-missing}"
run_id="${OPENCLARION_EGRESS_SMOKE_RUN_ID:-$$-${RANDOM:-0}}"
timeout_seconds="${OPENCLARION_EGRESS_SMOKE_TIMEOUT_SECONDS:-8}"
proof_path="${OPENCLARION_EGRESS_SMOKE_PROOF_PATH:-}"

case "$pull_policy" in
  always|missing|never) ;;
  *)
    echo "[egress-allowdeny-smoke] OPENCLARION_EGRESS_SMOKE_PULL must be one of: always, missing, never" >&2
    exit 2
    ;;
esac

if [[ "$image" =~ (^|:)latest(@|$) ]]; then
  echo "[egress-allowdeny-smoke] image must not use latest: $image" >&2
  exit 2
fi
if [[ -n "$image" && "$image" != *@sha256:* && "$pull_policy" == "never" ]]; then
  echo "[egress-allowdeny-smoke] OPENCLARION_EGRESS_SMOKE_IMAGE must be digest-pinned when pull policy is never: $image" >&2
  exit 2
fi
if ! [[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]]; then
  echo "[egress-allowdeny-smoke] OPENCLARION_EGRESS_SMOKE_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 2
fi

workdir="$(mktemp -d -t openclarion-egress-allowdeny.XXXXXX)"
helper="${workdir}/egress-allowdeny-smoke"
sandbox_net="openclarion-egress-sandbox-${run_id}"
upstream_net="openclarion-egress-upstream-${run_id}"
allowed_name="openclarion-egress-allowed-${run_id}"
denied_name="openclarion-egress-denied-${run_id}"
proxy_name="openclarion-egress-proxy-${run_id}"

cleanup() {
  docker rm -f -v "$allowed_name" "$denied_name" "$proxy_name" >/dev/null 2>&1 || true
  docker network rm "$sandbox_net" "$upstream_net" >/dev/null 2>&1 || true
  rm -rf "$workdir"
}
trap cleanup EXIT

ensure_image() {
  if [[ "$image" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
    case "$pull_policy" in
      always)
        docker pull "$image" >/dev/null
        ;;
      missing)
        if ! docker image inspect "$image" >/dev/null 2>&1; then
          docker pull "$image" >/dev/null
        fi
        ;;
      never)
        docker image inspect "$image" >/dev/null
        ;;
    esac
    return
  fi
  case "$pull_policy" in
    always)
      docker pull "$image" >/dev/null
      ;;
    missing)
      if ! docker image inspect "$image" >/dev/null 2>&1; then
        docker pull "$image" >/dev/null
      fi
      ;;
    never)
      ;;
  esac
  local digest
  digest="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$image" | sed -n '1p')"
  if [[ -z "$digest" || ! "$digest" =~ ^[^[:space:]@]+@sha256:[A-Fa-f0-9]{64}$ ]]; then
    echo "[egress-allowdeny-smoke] could not resolve a digest-pinned image ref for $image" >&2
    exit 2
  fi
  image="$digest"
}

run_helper_container() {
  docker run --rm \
    --pull never \
    --network "$sandbox_net" \
    --user 65532:65532 \
    --read-only \
    --security-opt no-new-privileges \
    --cap-drop ALL \
    --memory 128m \
    --cpus 0.5 \
    --pids-limit 64 \
    --mount "type=bind,source=${helper},target=/work/egress-allowdeny-smoke,readonly" \
    "$image" /work/egress-allowdeny-smoke "$@"
}

echo "[egress-allowdeny-smoke] building Linux helper..." >&2
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -o "$helper" ./scripts/egress_allowdeny_smoke
chmod 0555 "$helper"

ensure_image
echo "[egress-allowdeny-smoke] using digest-pinned image $image..." >&2

echo "[egress-allowdeny-smoke] creating Docker networks..." >&2
docker network create --internal --label openclarion.smoke=egress-allowdeny "$sandbox_net" >/dev/null
docker network create --label openclarion.smoke=egress-allowdeny "$upstream_net" >/dev/null

echo "[egress-allowdeny-smoke] starting upstream targets..." >&2
docker run -d \
  --name "$allowed_name" \
  --label openclarion.smoke=egress-allowdeny \
  --pull never \
  --network "$upstream_net" \
  --network-alias allowed.internal \
  --user 65532:65532 \
  --read-only \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --memory 128m \
  --cpus 0.5 \
  --pids-limit 64 \
  --mount "type=bind,source=${helper},target=/work/egress-allowdeny-smoke,readonly" \
  "$image" /work/egress-allowdeny-smoke serve --listen :8080 --name allowed >/dev/null

docker run -d \
  --name "$denied_name" \
  --label openclarion.smoke=egress-allowdeny \
  --pull never \
  --network "$upstream_net" \
  --network-alias denied.internal \
  --user 65532:65532 \
  --read-only \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --memory 128m \
  --cpus 0.5 \
  --pids-limit 64 \
  --mount "type=bind,source=${helper},target=/work/egress-allowdeny-smoke,readonly" \
  "$image" /work/egress-allowdeny-smoke serve --listen :8080 --name denied >/dev/null

echo "[egress-allowdeny-smoke] starting allowlist proxy..." >&2
docker run -d \
  --name "$proxy_name" \
  --label openclarion.smoke=egress-allowdeny \
  --pull never \
  --network "$sandbox_net" \
  --network-alias egress-proxy \
  --user 65532:65532 \
  --read-only \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --memory 128m \
  --cpus 0.5 \
  --pids-limit 64 \
  --mount "type=bind,source=${helper},target=/work/egress-allowdeny-smoke,readonly" \
  "$image" /work/egress-allowdeny-smoke proxy --listen :18080 --allow allowed.internal:8080 >/dev/null
docker network connect "$upstream_net" "$proxy_name"

echo "[egress-allowdeny-smoke] proving allowed target via proxy..." >&2
allowed_ok=0
for _ in $(seq 1 "$timeout_seconds"); do
  if run_helper_container client \
    --url http://allowed.internal:8080 \
    --proxy http://egress-proxy:18080 \
    --want-status 200 \
    --timeout 2s; then
    allowed_ok=1
    break
  fi
  sleep 1
done
if [[ "$allowed_ok" != "1" ]]; then
  echo "[egress-allowdeny-smoke] allowed target did not succeed through proxy" >&2
  exit 1
fi

echo "[egress-allowdeny-smoke] proving denied target is blocked by proxy..." >&2
run_helper_container client \
  --url http://denied.internal:8080 \
  --proxy http://egress-proxy:18080 \
  --want-status 403 \
  --timeout 2s

echo "[egress-allowdeny-smoke] proving sandbox client cannot bypass proxy..." >&2
run_helper_container client \
  --url http://allowed.internal:8080 \
  --want-fail \
  --timeout 2s

if [[ -n "$proof_path" ]]; then
  go run ./scripts/egress_allowdeny_smoke proof \
    --proof-path "$proof_path" \
    --image-ref "$image" \
    --source "make egress-allowdeny-smoke" \
    --run-id "$run_id" \
    --timeout-seconds "$timeout_seconds" \
    --allowed-target "allowed.internal:8080" \
    --denied-target "denied.internal:8080" \
    --proxy-target "egress-proxy:18080" >/dev/null
  echo "[egress-allowdeny-smoke] OK - smoke proof: $proof_path" >&2
fi

echo "[egress-allowdeny-smoke] OK - proxy allowed exact target, denied unlisted target, direct bypass failed." >&2
