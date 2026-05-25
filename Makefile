# OpenClarion Makefile
#
# This Makefile is the canonical entry point for CI gates. GitHub Actions
# must call `make` targets defined here so that local validation and
# remote validation stay in lockstep.
#
# Usage:
#   make pr               # run the full PR validation bundle
#   make docs-hygiene     # documentation language gate
#   make forbidden        # all forbidden-method guards
#   make adr-check        # validate ADR index and reading order
#   make links-check      # validate relative markdown links in governed docs
#   make dco-check        # validate DCO sign-off on PR / local commits
#   make workflow-parity  # workflow YAML must call only make targets and follow CI safety policy
#
# Gates are introduced progressively per docs/design/ci/README.md.
# A gate that depends on non-existent code (e.g. Go module before M0
# bootstrap) is a no-op until the code lands; the wiring is permanent.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Pinned external tooling
# ---------------------------------------------------------------------------
#
# Atlas CLI ships as a self-contained binary, not a `go install`able module.
# We invoke it via a pinned Docker image so that local and CI runs use the
# same binary. The `latest` tag is forbidden by docs/design/DEPENDENCIES.md;
# upgrades require updating ATLAS_IMAGE here AND the corresponding row in
# DEPENDENCIES.md.
#
# Atlas wrapper shape (see scripts/lib_atlas.sh for the full contract):
#
#   - The host script (scripts/lib_atlas.sh::atlas::start_dev_pg) launches
#     an ephemeral postgres:18-alpine on a per-invocation Docker network.
#     Atlas is then run on the same network and talks to the dev DB via a
#     plain postgres:// URL. Atlas does NOT mount the host Docker socket
#     and the `docker://...` dev-url form is intentionally NOT used (the
#     Atlas image does not ship a Docker CLI).
#
#   - The host Go toolchain is mounted read-only into the Atlas container
#     at /usr/local/go because the Atlas image does not ship Go, and the
#     Ent loader Atlas invokes for `--to ent://...` requires `go run`.
#
#   - The Atlas container runs as $(id -u):$(id -g) so generated migration
#     files are owned by the invoking user, not root.
#
# The Makefile invokes the wrapper via thin bash entry scripts to keep
# this complex Docker invocation in one place.
ATLAS_IMAGE ?= arigaio/atlas:1.2.0

# Ent schema and migration paths (canonical layout per docs/design/database/).
ENT_PKG := ./internal/persistence/ent
ENT_SCHEMA_URL := ent://internal/persistence/ent/schema
MIGRATIONS_DIR := internal/persistence/migrations

# ---------------------------------------------------------------------------
# Top-level entry points
# ---------------------------------------------------------------------------

.PHONY: help pr ci

help: ## Show this help
	@awk 'BEGIN { FS = ":.*?## "; printf "Targets:\n" } \
		/^[a-zA-Z_-]+:.*?## / { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

pr: ci ## Alias for the workflow-equivalent PR validation bundle

ci: workflow-parity docs-hygiene forbidden adr-check links-check generate go-vet go-build go-test openapi-lint openapi-fresh ent-fresh atlas-drift ## Full CI bundle (must mirror GitHub Actions)
	@echo ""
	@echo "[ci] all gates passed."

# Note on dco-check: it is part of CI but excluded from `make pr` /
# `make ci` because, locally, contributors usually run validation on a
# clean branch where `@{u}..HEAD` is empty and the gate would be a
# no-op anyway. The CI job in .github/workflows/ci.yml provides the
# binding check on every PR. Run `make dco-check` manually when
# verifying a stack of unpushed commits.

# ---------------------------------------------------------------------------
# Documentation gates
# ---------------------------------------------------------------------------

.PHONY: docs-hygiene adr-check links-check dco-check workflow-parity

docs-hygiene: ## Reject non-English CJK literals in governed documentation
	@bash scripts/check_no_non_english_chars.sh

adr-check: ## Validate ADR index against files on disk
	@bash scripts/check_adr_index.sh

links-check: ## Validate relative markdown links in governed docs
	@bash scripts/check_markdown_links.sh

dco-check: ## Validate DCO Signed-off-by on PR / local commits (DCO.md)
	@bash scripts/check_dco_signoff.sh

workflow-parity: ## Reject workflow drift: inline shell, mutable actions, missing permissions/timeouts
	@bash scripts/check_workflow_make_parity.sh

# ---------------------------------------------------------------------------
# Forbidden-method gates (architectural boundary lints)
# ---------------------------------------------------------------------------
#
# These gates land before any production code so the boundary is enforced
# from commit one. Each script is a no-op when its target tree does not
# exist yet, and becomes blocking the moment matching files appear.

.PHONY: forbidden forbidden-imports forbidden-latest forbidden-oapi-v2 forbidden-sqlite

forbidden: forbidden-imports forbidden-latest forbidden-oapi-v2 forbidden-sqlite ## Run all forbidden-method gates

forbidden-imports: ## Reject Gin/Echo/Fiber/Redis/MongoDB imports (ADR-0001, ADR-0012)
	@bash scripts/check_no_forbidden_imports.sh

forbidden-latest: ## Reject `latest` version pins in go.mod and package.json (docs/design/DEPENDENCIES.md)
	@bash scripts/check_no_latest_pin.sh

forbidden-oapi-v2: ## Reject oapi-codegen/v2 and openapi.compat.yaml (ADR-0007)
	@bash scripts/check_no_oapi_v2.sh

forbidden-sqlite: ## Reject SQLite usage in Go tests (ADR-0001)
	@bash scripts/check_no_sqlite_in_tests.sh

# ---------------------------------------------------------------------------
# Go gates (activated at M0 bootstrap)
# ---------------------------------------------------------------------------

.PHONY: generate go-vet go-build go-test openapi-lint openapi-fresh go-checks openapi-checks

generate: ## Run all code generators (go generate ./...)
	@go generate ./...

go-vet: ## Run go vet
	@go vet ./...

go-build: ## Compile all packages
	@go build ./...

go-test: ## Run all tests
	@go test -race -count=1 ./...

go-checks: generate go-vet go-build go-test ## Combined Go validation gate

openapi-lint: ## Lint OpenAPI spec with vacuum (mandatory gate)
	@go tool github.com/daveshanley/vacuum lint --details --fail-severity error api/openapi.yaml

openapi-fresh: ## Ensure generated code is up-to-date with OpenAPI spec
	@go generate ./api/...
	@if ! git diff --quiet -- api/openapi.gen.go; then \
		echo "[openapi-fresh] FAIL: api/openapi.gen.go is stale. Run 'go generate ./api/...' and commit."; \
		git diff api/openapi.gen.go; \
		exit 1; \
	fi
	@echo "[openapi-fresh] generated code is up-to-date."

openapi-checks: openapi-lint openapi-fresh ## Combined OpenAPI validation gate

# ---------------------------------------------------------------------------
# Ent / Atlas gates (activated at M1-PR1: persistence foundation)
# ---------------------------------------------------------------------------
#
# Ent is the canonical schema definition; Atlas is the canonical migration
# producer. Generation freshness is enforced via `ent-fresh`; schema /
# migration alignment is enforced via `atlas-drift` (which copies the
# committed migrations into a temp dir, runs `atlas migrate diff` against
# the live ent schema, and fails if Atlas wants to write a new migration).
#
# Atlas runs inside the pinned Docker image (ATLAS_IMAGE). The full
# wrapper contract (per-invocation dev Postgres, host Go toolchain mount,
# non-root user, scoped Docker network) lives in scripts/lib_atlas.sh.
# See ADR-0001 (PostgreSQL single source of truth) and
# docs/design/database/migrations.md.

.PHONY: ent-generate ent-fresh atlas-migrate-diff atlas-drift atlas-smoke

ent-generate: ## Regenerate ent client + entity code from schemas under internal/persistence/ent/schema
	@go generate $(ENT_PKG)/...

ent-fresh: ent-generate ## Reject stale ent-generated code (M1)
	@if ! git diff --quiet -- $(ENT_PKG); then \
		echo "[ent-fresh] FAIL: ent-generated code is stale. Run 'make ent-generate' and commit."; \
		git diff --stat -- $(ENT_PKG); \
		exit 1; \
	fi
	@echo "[ent-fresh] generated code is up-to-date."

atlas-migrate-diff: ## Generate a new Atlas migration from ent schema diff (usage: make atlas-migrate-diff NAME=add_alert_status)
	@if [ -z "$(NAME)" ]; then \
		echo "[atlas-migrate-diff] usage: make atlas-migrate-diff NAME=<migration_name>"; \
		exit 2; \
	fi
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/atlas_migrate_diff.sh "$(NAME)"

atlas-drift: ## Reject ent-schema vs migrations drift; runs in a temp copy so the real dir is never mutated (M1)
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/check_atlas_drift.sh

atlas-smoke: ## One-shot smoke: verify Dockerized Atlas can read the ent schema (M1-PR1 acceptance gate)
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/check_atlas_smoke.sh

# ---------------------------------------------------------------------------
# Future targets (introduced progressively as code lands)
# ---------------------------------------------------------------------------
#
# Per docs/design/ci/README.md, the targets below are placeholders for
# upcoming milestones. They will be wired into `ci` once the underlying
# code (provider boundaries, sandbox security) is committed.
#
#   provider-boundary  - forbidden cross-layer imports (M2, Go analyzer)
#   sandbox-security   - container non-root / limits gate (M4)
