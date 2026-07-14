#!/usr/bin/env bash
# scripts/lib_atlas.sh
#
# Shared helpers for the Atlas wrapper used by the atlas-smoke,
# atlas-drift, and atlas-migrate-diff entry scripts. NOT executable;
# this file is sourced.
#
# Wrapper shape (M1-PR1, post-redesign 2026-05-22):
#
#   1. The host script launches an ephemeral pgvector PostgreSQL 18
#      container attached to a dedicated per-invocation Docker network
#      (no --network host, no published ports). Concurrent invocations
#      (local parallel jobs, CI matrix) get unique container / network
#      names so they cannot collide.
#
#   2. Atlas runs in the pinned arigaio/atlas:1.2.0 image on the same
#      network with:
#        - host Go toolchain mounted read-only at /usr/local/go
#          (the Atlas image itself does not ship Go; the Ent loader
#          Atlas invokes for `--to ent://...` is a `go run` against
#          the project's own ent code)
#        - --user "$(id -u):$(id -g)" so files Atlas writes
#          (migration SQL, atlas.sum) are owned by the invoking user
#          on the host, NOT root
#        - HOME=/tmp, GOCACHE=/tmp/gocache, GOMODCACHE=/tmp/gomodcache
#          because the non-root user has no writable home in the
#          distroless base image
#
#   3. Atlas talks to the dev Postgres via a plain
#      postgres://postgres:postgres@<container-name>:5432/dev?... URL
#      that Docker resolves through the dedicated network's DNS.
#      Atlas does NOT mount the host Docker socket; the dev Postgres
#      is launched by the host script before Atlas runs, not by Atlas
#      itself (the Atlas image does not ship a Docker CLI).
#
# This wrapper deliberately keeps the same `arigaio/atlas:<pin>` image
# and the same `--to ent://...` semantics so behaviour is identical
# between local runs and CI; the only host requirement on top of
# Docker is a working `go` binary so that GOROOT can be resolved and
# mounted into the container. CI provides this via actions/setup-go.

# Single source of truth for ATLAS_IMAGE and ENT_SCHEMA_URL is the
# root Makefile; the canonical entry point is `make atlas-*` which
# injects these values explicitly into the script's environment. The
# defaults below are a fallback ONLY for direct script invocation
# (manual debugging) and intentionally mirror the Makefile values so
# script-only runs still work out of the box. If the two ever drift,
# the Makefile wins because every CI gate goes through it.
ATLAS_IMAGE="${ATLAS_IMAGE:-arigaio/atlas:1.2.0}"
DEV_PG_IMAGE="${DEV_PG_IMAGE:-pgvector/pgvector:0.8.2-pg18-trixie}"
ENT_SCHEMA_URL="${ENT_SCHEMA_URL:-ent://internal/persistence/ent/schema}"

# Per-invocation unique resource names.
ATLAS_RUN_ID="${ATLAS_RUN_ID:-$$-${RANDOM:-0}}"
ATLAS_NET="atlas-net-${ATLAS_RUN_ID}"
ATLAS_PG_NAME="atlas-pg-${ATLAS_RUN_ID}"

atlas::require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[atlas] required tool not found in PATH: $1" >&2
    exit 1
  }
}

# Resolves GOROOT_HOST to a directory containing bin/go.
# CI must run actions/setup-go before invoking any wrapper that calls
# this; locally, any working `go` install is sufficient.
atlas::resolve_goroot() {
  if [[ -z "${GOROOT_HOST:-}" ]]; then
    if ! command -v go >/dev/null 2>&1; then
      echo "[atlas] host 'go' binary required: the Atlas image does not ship a Go runtime, and the Ent loader Atlas invokes for ent:// runs 'go' inside the Atlas container via the mounted host toolchain." >&2
      exit 1
    fi
    GOROOT_HOST="$(go env GOROOT)"
  fi
  if [[ ! -x "$GOROOT_HOST/bin/go" ]]; then
    echo "[atlas] resolved GOROOT_HOST='$GOROOT_HOST' does not contain bin/go." >&2
    exit 1
  fi
}

atlas::start_dev_pg() {
  echo "[atlas] starting dev Postgres ($DEV_PG_IMAGE) on network $ATLAS_NET..."
  docker network create "$ATLAS_NET" >/dev/null
  docker run -d --rm \
    --name "$ATLAS_PG_NAME" \
    --network "$ATLAS_NET" \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_PASSWORD=postgres \
    -e POSTGRES_DB=dev \
    "$DEV_PG_IMAGE" >/dev/null

  for i in $(seq 1 60); do
    # Query the target database because pg_isready can succeed while the image
    # entrypoint is still creating POSTGRES_DB.
    if docker exec "$ATLAS_PG_NAME" psql -X -v ON_ERROR_STOP=1 -U postgres -d dev \
      -Atc 'SELECT 1' >/dev/null 2>&1; then
      if ! docker exec "$ATLAS_PG_NAME" psql -X -v ON_ERROR_STOP=1 -U postgres -d dev \
        -c 'CREATE EXTENSION IF NOT EXISTS vector' >/dev/null; then
        echo "[atlas] failed to enable vector extension." >&2
        exit 1
      fi
      echo "[atlas] dev Postgres ready after ${i}s."
      return 0
    fi
    sleep 1
  done
  echo "[atlas] dev Postgres did not become ready within 60s." >&2
  exit 1
}

atlas::stop_dev_pg() {
  docker stop "$ATLAS_PG_NAME" >/dev/null 2>&1 || true
  docker network rm "$ATLAS_NET" >/dev/null 2>&1 || true
}

# atlas::run <atlas-args...>
atlas::run() {
  local uid gid
  uid="$(id -u)"
  gid="$(id -g)"
  docker run --rm \
    --network "$ATLAS_NET" \
    --user "${uid}:${gid}" \
    -v "$PWD:/workspace" \
    -w /workspace \
    -v "$GOROOT_HOST:/usr/local/go:ro" \
    -e PATH=/usr/local/go/bin:/usr/bin:/bin \
    -e HOME=/tmp \
    -e GOROOT=/usr/local/go \
    -e GOCACHE=/tmp/gocache \
    -e GOMODCACHE=/tmp/gomodcache \
    "$ATLAS_IMAGE" \
    "$@"
}

# Dev URL pointing at the dev Postgres container by name. Resolved via
# the dedicated network's embedded DNS, so it never conflicts with any
# host-side port and works in CI matrix runs.
atlas::dev_url() {
  echo "postgres://postgres:postgres@${ATLAS_PG_NAME}:5432/dev?search_path=public&sslmode=disable"
}
