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
# Top-level entry points
# ---------------------------------------------------------------------------

.PHONY: help pr ci

help: ## Show this help
	@awk 'BEGIN { FS = ":.*?## "; printf "Targets:\n" } \
		/^[a-zA-Z_-]+:.*?## / { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

pr: ci ## Alias for the workflow-equivalent PR validation bundle

ci: workflow-parity docs-hygiene forbidden adr-check links-check ## Full CI bundle (must mirror GitHub Actions)
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
# Future targets (introduced progressively as code lands)
# ---------------------------------------------------------------------------
#
# Per docs/design/ci/README.md, the targets below are placeholders for
# upcoming milestones. They will be wired into `ci` once the underlying
# code (Go module, OpenAPI spec, Ent schema, etc.) is committed.
#
#   generate           - run all code generators (M0 once Go skeleton lands)
#   go-vet             - go vet ./... (M0)
#   go-build           - go build ./... (M0)
#   go-test            - go test ./... (M0)
#   openapi-lint       - vacuum lint api/openapi.yaml (M0)
#   openapi-fresh      - make generate must produce zero diff (M0)
#   ent-fresh          - ent generation freshness (M1)
#   atlas-drift        - Atlas migration drift check (M1)
#   provider-boundary  - forbidden cross-layer imports (M2, Go analyzer)
#   sandbox-security   - container non-root / limits gate (M4)
