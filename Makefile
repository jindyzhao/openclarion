# OpenClarion Makefile
#
# This Makefile is the canonical entry point for CI gates. GitHub Actions
# must call `make` targets defined here so that local validation and
# remote validation stay in lockstep.
#
# Usage:
#   make pr               # run the full PR validation bundle with wall-clock budget
#   make docs-hygiene     # documentation language, terminology, proof-state, and metadata gates
#   make forbidden        # all forbidden-method guards
#   make adr-check        # validate ADR index and reading order
#   make links-check      # validate markdown links, anchors, and docs reachability
#   make external-links-check # inventory external links; opt-in env enables live checks
#   make markdownlint     # validate governed Markdown structure/style
#   make gate-hardening-check # validate gate maturity checklist coverage
#   make comment-debt-check # validate tracked source comment debt
#   make deferred-followups-check # validate deferred decision ledger
#   make text-file-hygiene # validate tracked text encoding and line endings
#   make file-mode-check  # validate tracked Git file modes
#   make allowlist-discipline # validate allowlist owner / expiry / removal metadata
#   make branch-protection-check # validate branch protection required-check policy
#   make dependabot-policy-check # validate Dependabot update policy invariants
#   make manual-target-isolation # validate manual smoke/evidence targets stay out of CI
#   make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=diagnosis-live-browser-smoke # preflight one manual evidence target
#   make workflow-change-guard # validate PR workflow-file change isolation
#   make linear-history-check # validate PR ranges contain no merge commits
#   make pr-budget-test   # validate the make pr wall-clock budget wrapper
#   make repo-size-check  # validate Git-visible file size budgets
#   make pr-file-count-check # validate PR changed-file count cap
#   make go-coverage # enforce handwritten Go package coverage floor
#   make pr-title-check   # validate PR title Conventional Commit shape
#   make pr-description-check # validate PR body risk/rollback sections
#   make pr-template-check # validate PR template matches required PR body sections
#   make issue-template-check # validate issue template front matter and sections
#   make pr-impact-reference-check # validate issue/ADR reference for high-impact PRs
#   make testcontainers-contract # integration-test DB and host-network contract
#   make dco-check        # validate DCO sign-off on PR / local commits
#   make workflow-parity  # workflow YAML must call only make targets and follow CI safety policy
#   make actionlint       # validate GitHub Actions workflow semantics
#   make go-toolchain-check # validate Go version declarations across modules, lint, and workflows
#   make shell-syntax-check # validate tracked shell scripts with bash -n
#   make yaml-syntax-check # validate tracked YAML syntax and weak-feature policy
#   make openclarion-release-build # build OpenClarion release binary into an ignored or external path
#   make openclarion-service-image-build # manual deployment helper: build local scratch service image
#   make openclarion-service-image-push # manual deployment helper: push service image and print digest ref
#   make generated-headers # validate generated file headers
#   make generate-fresh   # validate make generate freshness and idempotence
#   make go-licenses-check # validate Go dependency license allowlist
#   make osv-scan         # validate npm lockfiles with OSV-Scanner
#   make operations-config-hygiene # validate alert-operations endpoint and browser-state hygiene
#   make manual-evidence-readiness # manual readiness preflight for remaining live/evidence targets
#   make diagnosis-auth-live-smoke # manual live diagnosis auth status/check proof through HTTP API
#   make notification-channel-live-smoke # manual live notification channel test through HTTP API
#   make alertmanager-auto-diagnosis-live-smoke # manual live Alertmanager webhook to auto_room AI notification proof
#   make report-live-smoke # manual live M2 smoke against real services
#   make report-policy-live-smoke # manual live M3.1 profile-driven smoke against real services
#   make report-schedule-live-smoke # manual M3.1 scheduled-trigger proof against real services
#   make report-live-smoke-output-test # M2/M3.1 live report proof validator tests
#   make agent-runtime-smoke # manual M4 smoke against a candidate sandbox image
#   make custom-thin-runner-smoke # manual M4 smoke: local custom runner candidate
#   make sandbox-m4-runtime-smoke-artifacts OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=... OPENCLARION_AGENT_RUNTIME_IMAGE=...
#   make agent-tool-scripts-test # M4 sandbox tool helper contract tests
#   make sandbox-baseline-audit # M4/M5 code-level sandbox baseline audit
#   make sandbox-m4-baseline-audit OUT=...
#   make sandbox-quality-compare-test # M4 offline sandbox/direct SubReport comparison tests
#   make sandbox-m4-subreport-generate SNAPSHOT_ID=... SCENARIO=... CANDIDATE_ID=... OUT=...
#   make sandbox-m4-quality-sample-export SELECTION=... ROOT=...
#   make sandbox-m4-quality-manifest-prepare ROOT=... SAMPLE_BASIS=... OUT=...
#   make sandbox-m4-quality-compare QUALITY_MANIFEST=... OUT=...
#   make sandbox-m4-decision-test # M4 proceed/iterate/defer decision logic tests
#   make sandbox-m4-decision BASELINE_AUDIT=... QUALITY_COMPARISON=... REVIEW_EVIDENCE=...
#   make sandbox-m4-review-evidence-template QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE=... REVIEWER=...
#   make sandbox-m4-review-evidence-template QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE_FILE=... REVIEWER=...
#   make sandbox-m4-evidence-packet QUALITY_MANIFEST=... REVIEW_EVIDENCE=... [RUNTIME_SMOKE_ARTIFACTS_ROOT=...] OUT_DIR=...
#   make sandbox-m4-evidence-packet-verify PACKET_DIR=... # verify retained M4 packet without rerunning helpers
#   make diagnosis-dev-oidc-issuer # manual M5 local OIDC issuer/token helper
#   make diagnosis-live-browser-smoke # manual M5 browser smoke against real backend/worker stack
#   make diagnosis-live-convergence-smoke # manual M5 backend-only WebSocket convergence smoke
#   make diagnosis-room-policy-test # M5 pure policy boundary tests
#   make diagnosis-room-workflow-test # M5 Temporal room workflow/client and lifecycle activity tests
#   make diagnosis-auth-test # M5 AuthProvider/OIDC/RBAC/WS ticket boundary + persistence + transport relay tests
#   make diagnosis-live-smoke-output-test # M5 live browser smoke proof validator tests
#   make stage5-local-worker-check # manual M5 local worker readiness check
#   make stage5-local-worker # manual M5 local worker/API process from private env
#   make container-provider-smoke # manual M4 smoke: real Docker Provider.Run lifecycle
#   make container-provider-timeout-smoke # manual M4 smoke: real Docker timeout cleanup
#   make container-provider-output-cap-smoke # manual M4 smoke: real Docker output cap
#   make egress-allowdeny-smoke # manual M4 smoke: Docker proxy topology allows/denies egress
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
#     an ephemeral pgvector PostgreSQL 18 on a per-invocation Docker network.
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
GOVULNCHECK_VERSION ?= v1.1.4
GO_LICENSES_VERSION ?= v1.6.0
OSV_SCANNER_VERSION ?= v1.9.2
ACTIONLINT_VERSION ?= v1.7.12
GOLANGCI_LINT_VERSION ?= v2.12.2
OASDIFF_VERSION ?= v1.11.7
GOLANGCI_LINT := $(CURDIR)/bin/golangci-lint
CUSTOM_GOLANGCI_LINT := $(CURDIR)/bin/custom-gcl
GO_MODULE_DIRS := . tools/openclarion-linter scripts/diagnosis_assistant_runner
GO_CHECK_PACKAGES := ./api/... ./cmd/... ./internal/... ./scripts/...
NPM ?= npm
PR_BUDGET ?= 15m
PR_BUDGET_MODE ?= enforce
ifeq ($(CI),true)
# ubuntu-24.04 provides the branded Chrome used by the Playwright channel.
PLAYWRIGHT_INSTALL ?= test -x /opt/google/chrome/chrome
PLAYWRIGHT_SMOKE_ENV ?= OPENCLARION_PLAYWRIGHT_CHANNEL=chrome
else
PLAYWRIGHT_INSTALL ?= npx playwright install --with-deps --only-shell chromium
PLAYWRIGHT_SMOKE_ENV ?=
endif

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
		/^[a-zA-Z][a-zA-Z0-9_-]+:.*?## / { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

pr: ## Run the workflow-equivalent PR validation bundle with a wall-clock budget
	@go run ./scripts/pr_budget --budget "$(PR_BUDGET)" --mode "$(PR_BUDGET_MODE)" -- $(MAKE) ci

# Focused Go test targets remain granular workflow entrypoints. Their test sets
# are strict subsets of go-test, so the local aggregate executes them once.
ci: workflow-parity actionlint docs-hygiene forbidden adr-check links-check markdownlint doc-claims-check gate-hardening-check text-file-hygiene text-file-hygiene-test file-mode-check file-mode-check-test manual-target-isolation comment-debt-check comment-debt-check-test deferred-followups-check deferred-followups-check-test pr-template-check pr-template-check-test issue-template-check issue-template-check-test go-toolchain-check go-toolchain-check-test shell-syntax-check yaml-syntax-check allowlist-discipline allowlist-discipline-test branch-protection-check branch-protection-check-test dependabot-policy-check dependabot-policy-check-test workflow-change-guard-test linear-history-check-test pr-file-count-check-test pr-impact-reference-check-test pr-budget-test repo-size-check repo-size-check-test generated-headers generate-fresh secrets-scan operations-config-hygiene operations-config-hygiene-test govulncheck go-licenses-check osv-scan go-lint testcontainers-contract go-vet go-build diagnosis-agent-runtime-check sandbox-baseline-audit go-test go-coverage openapi-lint openapi-fresh openapi-breaking openapi-fingerprint ent-fresh atlas-drift frontend-checks ## Coverage-equivalent local CI bundle
	@echo ""
	@echo "[ci] all gates passed."

# Notes on PR-context-only checks:
# - dco-check is excluded from `make pr` / `make ci` because, locally,
#   contributors usually run validation on a clean branch where `@{u}..HEAD`
#   is empty and the gate would be a no-op anyway.
# - pr-title-check / pr-description-check are excluded because the
#   authoritative PR title/body exist only on the pull_request event. Run them
#   manually with:
#   PR_TITLE='feat: add concise description' make pr-title-check
#   PR_BODY="$(gh pr view --json body --jq .body)" make pr-description-check
# - pr-impact-reference-check is excluded because it needs both PR body and
#   PR changed-file range. Run it locally with:
#   PR_BODY="$(gh pr view --json body --jq .body)" IMPACT_REFERENCE_BASE_REF=main IMPACT_REFERENCE_HEAD_SHA=HEAD make pr-impact-reference-check
# - workflow-change-guard is excluded because its authoritative changed-file
#   range exists only on pull_request; run it locally with:
#   WORKFLOW_CHANGE_BASE_REF=main WORKFLOW_CHANGE_HEAD_SHA=HEAD make workflow-change-guard
# - linear-history-check is excluded because its authoritative range comes from
#   the pull_request event. Run it locally with:
#   LINEAR_HISTORY_BASE_REF=main LINEAR_HISTORY_HEAD_SHA=HEAD make linear-history-check
# - pr-file-count-check is excluded because GitHub's pull_request.changed_files
#   and labels are authoritative for the cap and maintainer override. Run it
#   locally with:
#   PR_FILE_COUNT_BASE_REF=main PR_FILE_COUNT_HEAD_SHA=HEAD make pr-file-count-check
# The CI jobs in .github/workflows/ci.yml provide binding checks on every PR.

# ---------------------------------------------------------------------------
# Documentation gates
# ---------------------------------------------------------------------------

.PHONY: docs-hygiene adr-check links-check external-links-check markdownlint doc-claims-check gate-hardening-check text-file-hygiene text-file-hygiene-test file-mode-check file-mode-check-test manual-target-isolation comment-debt-check comment-debt-check-test deferred-followups-check deferred-followups-check-test pr-template-check pr-template-check-test issue-template-check issue-template-check-test go-toolchain-check go-toolchain-check-test shell-syntax-check yaml-syntax-check allowlist-discipline allowlist-discipline-test branch-protection-check branch-protection-check-test dependabot-policy-check dependabot-policy-check-test workflow-change-guard workflow-change-guard-test linear-history-check linear-history-check-test pr-file-count-check pr-file-count-check-test pr-impact-reference-check pr-impact-reference-check-test pr-budget-test repo-size-check repo-size-check-test pr-title-check pr-description-check dco-check workflow-parity actionlint

docs-hygiene: ## Reject non-English CJK literals, terminology drift, proof-state drift, and stale Last updated metadata in governed documentation
	@bash scripts/check_no_non_english_chars.sh
	@bash scripts/check_project_terminology.sh
	@go run ./scripts/e2e_verification_check
	@go run ./scripts/docs_metadata_check

adr-check: ## Validate ADR index against files on disk
	@bash scripts/check_adr_index.sh

links-check: ## Validate markdown links, anchors, and docs reachability
	@bash scripts/check_markdown_links.sh

external-links-check: ## Inventory external Markdown links; set OPENCLARION_EXTERNAL_LINKS_LIVE=1 for scheduled liveness
	@go run ./scripts/external_link_check

markdownlint: frontend-install ## Validate governed Markdown structure/style
	@bash scripts/check_markdownlint.sh

doc-claims-check: ## Validate shipped CURRENT_STATE.md path claims
	@bash scripts/check_doc_claims.sh

gate-hardening-check: ## Validate CI gate maturity checklist coverage
	@go run ./scripts/gate_hardening_check

text-file-hygiene: ## Validate tracked text file encoding and line endings
	@go run ./scripts/text_file_hygiene

text-file-hygiene-test: ## Validate tracked text file hygiene checker behavior
	@go test -race -count=1 ./scripts/text_file_hygiene

file-mode-check: ## Validate tracked Git file modes
	@go run ./scripts/file_mode_check

file-mode-check-test: ## Validate tracked Git file mode checker behavior
	@go test -race -count=1 ./scripts/file_mode_check

comment-debt-check: ## Validate source comment debt tracking
	@go run ./scripts/comment_debt_check

comment-debt-check-test: ## Validate source comment debt checker behavior
	@go test -race -count=1 ./scripts/comment_debt_check

deferred-followups-check: ## Validate deferred decision ledger
	@go run ./scripts/deferred_followups_check

deferred-followups-check-test: ## Validate deferred follow-up checker behavior
	@go test -race -count=1 ./scripts/deferred_followups_check

go-toolchain-check: ## Validate Go version declarations across modules, lint, and workflows
	@go run ./scripts/go_toolchain_check

go-toolchain-check-test: ## Validate Go toolchain version checker behavior
	@go test -race -count=1 ./scripts/go_toolchain_check

shell-syntax-check: ## Validate tracked shell scripts with bash -n
	@go test -race -count=1 ./scripts/shell_syntax_check
	@go run ./scripts/shell_syntax_check

yaml-syntax-check: ## Validate tracked YAML syntax and weak-feature policy
	@go test -race -count=1 ./scripts/yaml_syntax_check
	@go run ./scripts/yaml_syntax_check

allowlist-discipline: ## Validate allowlist owner, expiry, and removal metadata
	@go run ./scripts/allowlist_discipline

allowlist-discipline-test: ## Validate allowlist discipline checker behavior
	@go test -race -count=1 ./scripts/allowlist_discipline

dependabot-policy-check: ## Validate Dependabot update policy invariants
	@go run ./scripts/dependabot_policy_check

dependabot-policy-check-test: ## Validate Dependabot policy checker behavior
	@go test -race -count=1 ./scripts/dependabot_policy_check

branch-protection-check: ## Validate branch protection required-check policy
	@go run ./scripts/branch_protection_check

branch-protection-check-test: ## Validate branch protection policy checker behavior
	@go test -race -count=1 ./scripts/branch_protection_check

manual-target-isolation: ## Ensure manual smoke/evidence targets stay out of automated CI
	@go test -race -count=1 ./scripts/manual_target_isolation
	@go run ./scripts/manual_target_isolation

workflow-change-guard: ## Validate PR workflow-file change isolation
	@go run ./scripts/workflow_change_guard

workflow-change-guard-test: ## Validate workflow change guard behavior
	@go test -race -count=1 ./scripts/workflow_change_guard

linear-history-check: ## Validate PR ranges contain no merge commits
	@go run ./scripts/linear_history_check

linear-history-check-test: ## Validate linear history checker behavior
	@go test -race -count=1 ./scripts/linear_history_check

pr-file-count-check: ## Validate PR changed-file count cap
	@go run ./scripts/pr_file_count

pr-file-count-check-test: ## Validate PR file-count checker behavior
	@go test -race -count=1 ./scripts/pr_file_count

pr-impact-reference-check: ## Validate high-impact PRs link an issue or ADR
	@go run ./scripts/pr_impact_reference

pr-impact-reference-check-test: ## Validate high-impact PR reference checker behavior
	@go test -race -count=1 ./scripts/pr_impact_reference

pr-budget-test: ## Validate make pr wall-clock budget wrapper behavior
	@go test -race -count=1 ./scripts/pr_budget

repo-size-check: ## Enforce Git-visible file size budgets
	@go run ./scripts/repo_size_check

repo-size-check-test: ## Validate repository size budget checker behavior
	@go test -race -count=1 ./scripts/repo_size_check

pr-title-check: ## Validate PR title Conventional Commit shape
	@bash scripts/check_pr_title.sh

pr-description-check: ## Validate PR body risk/rollback sections
	@bash scripts/check_pr_description.sh

pr-template-check: ## Validate pull request template required sections
	@bash scripts/check_pr_template.sh

pr-template-check-test: ## Validate pull request template checker behavior
	@go test -race -count=1 ./scripts -run TestPRTemplateCheck

issue-template-check: ## Validate issue template front matter and sections
	@bash scripts/check_issue_templates.sh

issue-template-check-test: ## Validate issue template checker behavior
	@go test -race -count=1 ./scripts -run TestIssueTemplateCheck

dco-check: ## Validate DCO Signed-off-by on PR / local commits (DCO.md)
	@bash scripts/check_dco_signoff.sh

workflow-parity: ## Reject workflow drift: inline shell, mutable actions, unpinned runners, unsafe defaults
	@bash scripts/check_workflow_make_parity.sh

actionlint: ## Validate GitHub Actions workflow syntax and semantics
	@go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# ---------------------------------------------------------------------------
# Forbidden-method gates (architectural boundary lints)
# ---------------------------------------------------------------------------
#
# These gates land before any production code so the boundary is enforced
# from commit one. Each script is a no-op when its target tree does not
# exist yet, and becomes blocking the moment matching files appear.

.PHONY: forbidden forbidden-latest forbidden-oapi-v2 forbidden-sqlite forbidden-agent-runtime

forbidden: forbidden-latest forbidden-oapi-v2 forbidden-sqlite forbidden-agent-runtime ## Run forbidden-method gates not covered by custom go-lint analyzers

forbidden-latest: ## Reject mutable dependency pins and unpinned Dockerfile base images (docs/design/DEPENDENCIES.md)
	@bash scripts/check_no_latest_pin.sh

forbidden-oapi-v2: ## Reject oapi-codegen/v2 and openapi.compat.yaml (ADR-0007)
	@bash scripts/check_no_oapi_v2.sh

forbidden-sqlite: ## Reject SQLite usage in Go tests (ADR-0001)
	@bash scripts/check_no_sqlite_in_tests.sh

forbidden-agent-runtime: ## Reject control-plane agent-framework deps and hardcoded runtime names before M4 acceptance
	@bash scripts/check_no_control_plane_agent_runtime_deps.sh

# ---------------------------------------------------------------------------
# Security gates (W1: supply-chain hardening)
# ---------------------------------------------------------------------------

.PHONY: secrets-scan operations-config-hygiene operations-config-hygiene-test govulncheck go-licenses-check osv-scan

secrets-scan: ## Detect leaked secrets via gitleaks (pinned version, .gitleaks.toml config)
	@bash scripts/run_secrets_scan.sh

operations-config-hygiene: ## Validate alert-operations endpoint and browser-state hygiene
	@go run ./scripts/operations_config_hygiene

operations-config-hygiene-test: ## Validate alert-operations hygiene checker behavior
	@go test -race -count=1 ./scripts/operations_config_hygiene

govulncheck: ## Detect known Go vulnerabilities in every first-party Go module
	@for dir in $(GO_MODULE_DIRS); do \
		echo "[govulncheck] $$dir"; \
		if [[ "$$dir" == "." ]]; then \
			go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) $(GO_CHECK_PACKAGES); \
		else \
			(cd "$$dir" && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...); \
		fi; \
	done

go-licenses-check: ## Validate Go dependency licenses against docs/design/DEPENDENCIES.md
	@GO_LICENSES_VERSION="$(GO_LICENSES_VERSION)" bash scripts/check_go_licenses.sh

osv-scan: ## Detect known vulnerabilities in npm package-lock files
	@OSV_SCANNER_VERSION="$(OSV_SCANNER_VERSION)" bash scripts/run_osv_scan.sh

# ---------------------------------------------------------------------------
# Go gates (activated at M0 bootstrap)
# ---------------------------------------------------------------------------

.PHONY: generated-headers generate generate-fresh go-vet go-build openclarion-release-build openclarion-service-image-build openclarion-service-image-push go-test go-coverage diagnosis-agent-runtime-check temporal-workflow-tests report-live-smoke-output-test sandbox-security agent-tool-scripts-test sandbox-baseline-audit sandbox-image-audit sandbox-m4-baseline-audit sandbox-quality-compare-test sandbox-m4-subreport-generate sandbox-m4-quality-sample-export sandbox-m4-quality-manifest-prepare sandbox-m4-quality-compare sandbox-m4-decision-test sandbox-m4-decision sandbox-m4-review-evidence-template sandbox-m4-evidence-packet-test sandbox-m4-evidence-packet sandbox-m4-evidence-packet-verify diagnosis-room-policy-test diagnosis-room-workflow-test diagnosis-auth-test diagnosis-chat-persistence-test diagnosis-live-smoke-output-test go-lint openclarion-linter-test testcontainers-contract openapi-lint openapi-fresh openapi-breaking openapi-fingerprint go-checks openapi-checks frontend-install ci-frontend-typecheck ci-frontend-lint ci-frontend-unit ci-frontend-build ci-frontend-smoke diagnosis-live-browser-smoke diagnosis-live-convergence-smoke ci-frontend-deadcode ci-frontend-audit openapi-ts-fresh frontend-checks

generated-headers: ## Validate generated files carry generator headers
	@bash scripts/check_generated_headers.sh

generate: ## Run all root-module code generators
	@go generate $(GO_CHECK_PACKAGES)

generate-fresh: ## Ensure make generate is fresh and idempotent
	@bash scripts/check_generate_fresh.sh

go-lint: openclarion-linter-test ## Run golangci-lint v2 with OpenClarion custom analyzers
	@mkdir -p "$(CURDIR)/bin"
	@GOBIN="$(CURDIR)/bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@bash scripts/check_lint_version.sh "$(GOLANGCI_LINT)" tools/openclarion-linter
	@$(GOLANGCI_LINT) custom --version $(GOLANGCI_LINT_VERSION) --destination "$(CURDIR)/bin" --name custom-gcl
	@$(CUSTOM_GOLANGCI_LINT) run $(GO_CHECK_PACKAGES)

openclarion-linter-test: ## Run custom OpenClarion analyzer tests
	@cd tools/openclarion-linter && go test ./...

testcontainers-contract: ## Enforce integration-test DB and host-network boundaries
	@bash scripts/check_testcontainers_integration.sh

go-vet: ## Run go vet
	@go vet $(GO_CHECK_PACKAGES)

go-build: ## Compile all packages
	@go build $(GO_CHECK_PACKAGES)

openclarion-release-build: ## Build OpenClarion release binary into an ignored or external path
	@go run ./scripts/openclarion_release_build

openclarion-service-image-build: ## Manual deployment helper: build local scratch service image; set OPENCLARION_SERVICE_IMAGE_REF
	@bash scripts/run_openclarion_service_image_build.sh

openclarion-service-image-push: ## Manual deployment helper: push service image and print immutable digest ref
	@bash scripts/run_openclarion_service_image_build.sh --push

go-test: ## Run all tests
	@go test -race -count=1 $(GO_CHECK_PACKAGES)

diagnosis-agent-runtime-check: ## Validate the isolated Eino-backed diagnosis runtime module
	@env GOWORK=off go -C scripts/diagnosis_assistant_runner vet ./...
	@env GOWORK=off go -C scripts/diagnosis_assistant_runner test -race -count=1 ./...
	@tmp_dir="$$(mktemp -d -t openclarion-diagnosis-agent-check.XXXXXX)"; \
		trap 'rm -rf "$$tmp_dir"' EXIT; \
		env GOWORK=off go -C scripts/diagnosis_assistant_runner build -trimpath -o "$$tmp_dir/diagnosis-assistant-runner" .

go-coverage: ## Enforce handwritten Go package coverage floor
	@bash scripts/check_go_coverage.sh

temporal-workflow-tests: ## Run focused Temporal workflow integration and replay tests
	@go test -race -count=1 -timeout=9m ./internal/orchestrator/temporal

report-live-smoke-output-test: ## Run focused M2/M3.1 live report proof validator tests
	@go test -race -count=1 ./scripts/report_ai_review_proof ./scripts/report_live_smoke_output ./scripts/report_schedule_live_smoke_output

sandbox-security: ## Run focused M4 sandbox contract and Docker security-spec tests
	@go test -race -count=1 ./internal/usecases/ports ./internal/providers/container/...

agent-tool-scripts-test: ## Run focused M4 sandbox tool helper contract tests
	@go test -race -count=1 ./scripts/agent_tool_metric_query ./scripts/agent_tool_topology_lookup

sandbox-baseline-audit: ## Run code-level M4/M5 sandbox baseline audit
	@go test -race -count=1 ./scripts/sandbox_baseline_audit
	@go run ./scripts/sandbox_baseline_audit

sandbox-image-audit: ## Manual M5 audit: verify OPENCLARION_SANDBOX_IMAGE_REF is the diagnosis assistant runtime
	@go test -race -count=1 ./scripts/sandbox_image_audit
	@go run ./scripts/sandbox_image_audit

sandbox-m4-baseline-audit: ## Manual M4 baseline audit retention: OUT=...
	@if [[ -z "$(OUT)" ]]; then \
		echo "[sandbox-m4-baseline-audit] usage: make sandbox-m4-baseline-audit OUT=<baseline-audit.json>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_baseline_audit --out "$(OUT)"

sandbox-quality-compare-test: ## Run focused M4 sandbox/direct SubReport comparison tests
	@go test -race -count=1 ./scripts/sandbox_quality_compare ./scripts/sandbox_quality_manifest_prepare ./scripts/sandbox_quality_sample_export ./scripts/sandbox_m4_subreport_generate

sandbox-m4-subreport-generate: ## Manual M4 sandbox SubReport generation: SNAPSHOT_ID=... SCENARIO=... CANDIDATE_ID=... OUT=...
	@if [[ -z "$(SNAPSHOT_ID)" || -z "$(SCENARIO)" || -z "$(CANDIDATE_ID)" || -z "$(OUT)" ]]; then \
		echo "[sandbox-m4-subreport-generate] usage: DATABASE_URL=<postgres-url> OPENCLARION_M4_SANDBOX_IMAGE_REF=<image@sha256:...> OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT=<dir> [OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED=<host:port> OPENCLARION_M4_SANDBOX_EGRESS_PROXY_URL=<proxy-url>] make sandbox-m4-subreport-generate SNAPSHOT_ID=<id> SCENARIO=<single_alert|cascade|alert_storm> CANDIDATE_ID=<stable-id> [GROUP_INDEX=<n>] OUT=<summary.json>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_m4_subreport_generate \
		--snapshot-id "$(SNAPSHOT_ID)" \
		--scenario "$(SCENARIO)" \
		--group-index "$(or $(GROUP_INDEX),0)" \
		--candidate-id "$(CANDIDATE_ID)" \
		--out "$(OUT)"

sandbox-m4-quality-sample-export: ## Manual M4 quality sample export: SELECTION=... ROOT=...
	@if [[ -z "$(SELECTION)" || -z "$(ROOT)" ]]; then \
		echo "[sandbox-m4-quality-sample-export] usage: DATABASE_URL=<postgres-url> make sandbox-m4-quality-sample-export SELECTION=<case-selection.json> ROOT=<empty-sample-root>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_quality_sample_export \
		--selection "$(SELECTION)" \
		--out-root "$(ROOT)"

sandbox-m4-quality-manifest-prepare: ## Manual M4 quality manifest preparation: ROOT=... SAMPLE_BASIS=... OUT=...
	@if [[ -z "$(ROOT)" || -z "$(SAMPLE_BASIS)" || -z "$(OUT)" ]]; then \
		echo "[sandbox-m4-quality-manifest-prepare] usage: make sandbox-m4-quality-manifest-prepare ROOT=<sample-root> SAMPLE_BASIS=<single-line basis> OUT=<quality-manifest.json>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_quality_manifest_prepare \
		--root "$(ROOT)" \
		--sample-basis "$(SAMPLE_BASIS)" \
		--out "$(OUT)"

sandbox-m4-quality-compare: ## Manual M4 quality comparison: QUALITY_MANIFEST=... OUT=...
	@if [[ -z "$(QUALITY_MANIFEST)" || -z "$(OUT)" ]]; then \
		echo "[sandbox-m4-quality-compare] usage: make sandbox-m4-quality-compare QUALITY_MANIFEST=<quality-manifest.json> OUT=<quality-comparison.json>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_quality_compare \
		--manifest "$(QUALITY_MANIFEST)" \
		--fail-on-regression \
		--out "$(OUT)"

sandbox-m4-decision-test: ## Run focused M4 sandbox decision gate tests
	@go test -race -count=1 ./scripts/sandbox_m4_decision

sandbox-m4-decision: ## Manual M4 decision: BASELINE_AUDIT=... QUALITY_COMPARISON=... REVIEW_EVIDENCE=...
	@if [[ -z "$(BASELINE_AUDIT)" || -z "$(QUALITY_COMPARISON)" || -z "$(REVIEW_EVIDENCE)" ]]; then \
		echo "[sandbox-m4-decision] usage: make sandbox-m4-decision BASELINE_AUDIT=<baseline.json> QUALITY_COMPARISON=<quality.json> REVIEW_EVIDENCE=<review.json>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_m4_decision \
		--baseline-audit "$(BASELINE_AUDIT)" \
		--quality-comparison "$(QUALITY_COMPARISON)" \
		--review-evidence "$(REVIEW_EVIDENCE)"

sandbox-m4-review-evidence-template: ## Manual M4 review evidence template: QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE[_FILE]=... REVIEWER=...
	@if [[ -z "$(QUALITY_COMPARISON)" || -z "$(RUNTIME_SMOKE_ARTIFACTS_ROOT)" || -z "$(SELECTED_CANDIDATE)" || -z "$(REVIEWER)" ]]; then \
		echo "[sandbox-m4-review-evidence-template] usage: make sandbox-m4-review-evidence-template QUALITY_COMPARISON=<quality-comparison.json> RUNTIME_SMOKE_ARTIFACTS_ROOT=<artifact-root-dir> [RUNTIME_SMOKE_REF_PREFIX=<relative-prefix>] SELECTED_CANDIDATE=<candidate-id> RUNTIME_CANDIDATE=<image@sha256:digest> REVIEWER=<reviewer> [EVIDENCE_DATE=YYYY-MM-DD] [REPRESENTATIVE_SAMPLE=1] [OUT=<review-evidence.json>]"; \
		echo "[sandbox-m4-review-evidence-template] alternate: set RUNTIME_CANDIDATE_FILE=<digest-ref.txt> instead of RUNTIME_CANDIDATE"; \
		exit 2; \
	fi
	@if [[ -z "$(RUNTIME_CANDIDATE)" && -z "$(RUNTIME_CANDIDATE_FILE)" ]]; then \
		echo "[sandbox-m4-review-evidence-template] set exactly one of RUNTIME_CANDIDATE or RUNTIME_CANDIDATE_FILE"; \
		exit 2; \
	fi
	@if [[ -n "$(RUNTIME_CANDIDATE)" && -n "$(RUNTIME_CANDIDATE_FILE)" ]]; then \
		echo "[sandbox-m4-review-evidence-template] set exactly one of RUNTIME_CANDIDATE or RUNTIME_CANDIDATE_FILE"; \
		exit 2; \
	fi
	@args=( \
		--quality-comparison "$(QUALITY_COMPARISON)" \
		--runtime-smoke-artifacts-root "$(RUNTIME_SMOKE_ARTIFACTS_ROOT)" \
		--selected-candidate "$(SELECTED_CANDIDATE)" \
		--reviewer "$(REVIEWER)" \
	); \
	if [[ -n "$(RUNTIME_CANDIDATE)" ]]; then args+=(--runtime-candidate "$(RUNTIME_CANDIDATE)"); else args+=(--runtime-candidate-file "$(RUNTIME_CANDIDATE_FILE)"); fi; \
	if [[ -n "$(RUNTIME_SMOKE_REF_PREFIX)" ]]; then args+=(--runtime-smoke-ref-prefix "$(RUNTIME_SMOKE_REF_PREFIX)"); fi; \
	if [[ -n "$(EVIDENCE_DATE)" ]]; then args+=(--evidence-date "$(EVIDENCE_DATE)"); fi; \
	if [[ "$(REPRESENTATIVE_SAMPLE)" == "1" ]]; then args+=(--representative-sample); fi; \
	if [[ -n "$(OUT)" ]]; then args+=(--out "$(OUT)"); fi; \
	go run ./scripts/sandbox_m4_review_evidence_template "$${args[@]}"

sandbox-m4-evidence-packet-test: ## Run focused M4 sandbox evidence packet tests
	@go test -race -count=1 ./scripts/sandbox_m4_evidence_packet ./scripts/sandbox_m4_runtime_smoke_artifacts

sandbox-m4-evidence-packet: ## Manual M4 evidence packet: QUALITY_MANIFEST=... REVIEW_EVIDENCE=... OUT_DIR=...
	@if [[ -z "$(QUALITY_MANIFEST)" || -z "$(REVIEW_EVIDENCE)" || -z "$(OUT_DIR)" ]]; then \
		echo "[sandbox-m4-evidence-packet] usage: make sandbox-m4-evidence-packet QUALITY_MANIFEST=<manifest.json> REVIEW_EVIDENCE=<review.json> [RUNTIME_SMOKE_ARTIFACTS_ROOT=<artifact-root-dir>] OUT_DIR=<empty-output-dir>"; \
		exit 2; \
	fi
	@go run ./scripts/sandbox_m4_evidence_packet \
		--quality-manifest "$(QUALITY_MANIFEST)" \
		--review-evidence "$(REVIEW_EVIDENCE)" \
		--runtime-smoke-artifacts-root "$(RUNTIME_SMOKE_ARTIFACTS_ROOT)" \
		--out-dir "$(OUT_DIR)"

sandbox-m4-evidence-packet-verify: ## Manual M4 evidence packet verification: PACKET_DIR=... or PACKET_JSON=...
	@if [[ -z "$(PACKET_DIR)$(PACKET_JSON)" || ( -n "$(PACKET_DIR)" && -n "$(PACKET_JSON)" ) ]]; then \
		echo "[sandbox-m4-evidence-packet-verify] usage: make sandbox-m4-evidence-packet-verify PACKET_DIR=<packet-dir> OR PACKET_JSON=<packet.json>"; \
		exit 2; \
	fi
	@packet_input="$(PACKET_DIR)"; \
	if [[ -n "$(PACKET_JSON)" ]]; then packet_input="$(PACKET_JSON)"; fi; \
	go run ./scripts/sandbox_m4_evidence_packet --verify-packet "$$packet_input"

diagnosis-room-policy-test: ## Run focused M5 short-conversation policy tests
	@go test -race -count=1 ./internal/usecases/diagnosisroom

diagnosis-room-workflow-test: ## Run focused M5 Temporal diagnosis room workflow/client and lifecycle activity tests
	@go test -race -count=1 ./internal/orchestrator/temporal -run 'Test(DiagnosisRoom|RunDiagnosisTurn)'

diagnosis-auth-test: ## Run focused M5 AuthProvider/RBAC/WS ticket/relay tests
	@go test -race -count=1 ./internal/usecases/diagnosisauth ./internal/usecases/ports
	@go test -race -count=1 ./internal/providers/auth/...
	@go test -race -count=1 ./internal/persistence/repository -run TestDiagnosisAuthTicketStore
	@go test -race -count=1 ./internal/transport/http -run 'Test(IssueDiagnosisAuthSession|CheckDiagnosisAuth|IssueDiagnosisWSTicket|HandleDiagnosisWebSocket|DiagnosisWebSocketRelay)'

diagnosis-chat-persistence-test: ## Run focused M5 ChatSession/ChatTurn persistence tests
	@go test -race -count=1 ./internal/domain -run 'TestNewChatSession|TestChatSession_RecordTurnAndClose|TestNewChatTurn'
	@go test -race -count=1 ./internal/persistence/repository -run 'TestDiagnosisRepository_(SaveChatSessionAndQuery|SaveChatSession_DuplicateKeys|SaveChatTurnAndList|ChatInvariantGuards)'

diagnosis-live-smoke-output-test: ## Run focused M5 live smoke proof validator tests
	@go test -race -count=1 ./scripts/diagnosis_live_smoke_output ./scripts/diagnosis_live_convergence_smoke_output

go-checks: generate go-vet go-build go-test diagnosis-agent-runtime-check ## Combined Go validation gate

openapi-lint: ## Lint OpenAPI spec with vacuum (mandatory gate)
	@bash scripts/check_openapi_lint.sh

openapi-fresh: ## Ensure generated code is up-to-date with OpenAPI spec
	@tmp="$$(mktemp)"; \
	trap 'rm -f "$$tmp"' EXIT; \
	cp api/openapi.gen.go "$$tmp"; \
	go generate ./api/...; \
	if ! cmp -s "$$tmp" api/openapi.gen.go; then \
		echo "[openapi-fresh] FAIL: api/openapi.gen.go is stale. Run 'go generate ./api/...' and commit."; \
		diff -u "$$tmp" api/openapi.gen.go || true; \
		exit 1; \
	fi
	@echo "[openapi-fresh] generated code is up-to-date."

openapi-breaking: openapi-lint ## Detect breaking OpenAPI changes with oasdiff (soft-fail until W4 sunset)
	@OASDIFF_VERSION="$(OASDIFF_VERSION)" bash scripts/check_openapi_breaking.sh

openapi-fingerprint: ## Validate critical OpenAPI node fingerprint lock
	@go run scripts/check_openapi_fingerprint.go

openapi-checks: openapi-lint openapi-fresh ## Combined OpenAPI lint/freshness gate

# ---------------------------------------------------------------------------
# Frontend gates (activated at M3: web/ skeleton)
# ---------------------------------------------------------------------------

frontend-install: ## Install frontend dependencies from package-lock.json
	@cd web && $(NPM) ci

ci-frontend-typecheck: frontend-install ## Run frontend TypeScript typecheck
	@cd web && $(NPM) run typecheck

ci-frontend-lint: frontend-install ## Run frontend ESLint
	@cd web && $(NPM) run lint

ci-frontend-unit: frontend-install ## Run frontend unit tests
	@cd web && $(NPM) run test

ci-frontend-build: frontend-install ## Build the Next.js frontend
	@cd web && $(NPM) run build

ci-frontend-smoke: ci-frontend-build ## Run Playwright route smoke with a mocked API
	@cd web && $(PLAYWRIGHT_INSTALL)
	@cd web && $(PLAYWRIGHT_SMOKE_ENV) $(NPM) run smoke:built

diagnosis-live-browser-smoke: ## Manual M5 smoke: browser diagnosis room against a real backend/worker stack
	@bash scripts/run_diagnosis_live_browser_smoke.sh

diagnosis-live-convergence-smoke: ## Manual M5 smoke: backend-only diagnosis convergence against a real backend/worker stack
	@bash scripts/run_diagnosis_live_convergence_smoke.sh

ci-frontend-deadcode: frontend-install ## Run frontend dead-code checks
	@cd web && $(NPM) run deadcode

ci-frontend-audit: frontend-install ## Run frontend npm audit
	@cd web && $(NPM) run audit

openapi-ts-fresh: frontend-install ## Ensure generated frontend OpenAPI types are up-to-date
	@bash scripts/check_openapi_ts_fresh.sh

frontend-checks: frontend-install ci-frontend-typecheck ci-frontend-lint ci-frontend-unit ci-frontend-build ci-frontend-smoke ci-frontend-deadcode ci-frontend-audit openapi-ts-fresh ## Combined frontend validation gate

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

.PHONY: ent-generate ent-fresh atlas-migrate-diff atlas-drift atlas-smoke manual-evidence-readiness alert-consultation-setup diagnosis-auth-live-smoke notification-channel-live-smoke alertmanager-auto-diagnosis-live-smoke report-live-smoke report-policy-live-smoke report-schedule-live-smoke agent-runtime-smoke custom-thin-runner-smoke container-provider-smoke container-provider-timeout-smoke container-provider-output-cap-smoke egress-allowdeny-smoke local-egress-proxy-build sandbox-m4-runtime-smoke-artifacts diagnosis-assistant-runner-build diagnosis-dev-oidc-issuer stage5-local-worker-check stage5-local-worker

ent-generate: ## Regenerate ent client + entity code from schemas under internal/persistence/ent/schema
	@go generate $(ENT_PKG)/...

ent-fresh: ## Reject stale ent-generated code (M1)
	@tmp="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	cp -a "$(ENT_PKG)" "$$tmp/ent-before"; \
	$(MAKE) --no-print-directory ent-generate; \
	if ! diff -qr "$$tmp/ent-before" "$(ENT_PKG)" >/tmp/openclarion-ent-fresh.diff; then \
		echo "[ent-fresh] FAIL: ent-generated code is stale. Run 'make ent-generate' and commit."; \
		sed -n '1,120p' /tmp/openclarion-ent-fresh.diff; \
		rm -f /tmp/openclarion-ent-fresh.diff; \
		exit 1; \
	fi; \
	rm -f /tmp/openclarion-ent-fresh.diff
	@echo "[ent-fresh] generated code is up-to-date."

atlas-migrate-diff: ## Generate a new Atlas migration from ent schema diff (usage: make atlas-migrate-diff NAME=add_alert_status)
	@if [ -z "$(NAME)" ]; then \
		echo "[atlas-migrate-diff] usage: make atlas-migrate-diff NAME=<migration_name>"; \
		exit 2; \
	fi
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/atlas_migrate_diff.sh "$(NAME)"

atlas-drift: ## Reject ent-schema vs migrations drift; runs in a temp copy so the real dir is never mutated (M1)
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/check_atlas_drift.sh

atlas-smoke: ## Manual one-shot smoke: verify Dockerized Atlas can read the ent schema (M1-PR1 acceptance gate)
	@ATLAS_IMAGE="$(ATLAS_IMAGE)" ENT_SCHEMA_URL="$(ENT_SCHEMA_URL)" bash scripts/check_atlas_smoke.sh

manual-evidence-readiness: ## Manual readiness preflight for remaining M2/M3.1/M4/M5 live and evidence targets
	@args=(); \
	if [[ -n "$(MANUAL_EVIDENCE_TARGET)" ]]; then args+=(--target "$(MANUAL_EVIDENCE_TARGET)"); fi; \
	go run ./scripts/manual_evidence_readiness "$${args[@]}"

alert-consultation-setup: ## Manual live setup: Alertmanager source + WeCom channel + auto_room workflow policy
	@bash scripts/run_alert_consultation_setup.sh

diagnosis-auth-live-smoke: ## Manual live diagnosis auth proof: HTTP API status/check with LDAP or bearer credentials
	@bash scripts/run_diagnosis_auth_live_smoke.sh

notification-channel-live-smoke: ## Manual live notification channel test: HTTP API -> profile secret_ref resolver -> Webhook
	@bash scripts/run_notification_channel_live_smoke.sh

alertmanager-auto-diagnosis-live-smoke: ## Manual live Alertmanager webhook -> auto_room diagnosis -> AI notification proof
	@bash scripts/run_alertmanager_auto_diagnosis_live_smoke.sh

report-live-smoke: ## Manual M2 smoke: real Prometheus -> Temporal -> Webhook via report-replay --wait
	@bash scripts/run_report_live_smoke.sh

report-policy-live-smoke: ## Manual M3.1 smoke: profile-driven policy replay -> Temporal -> notification via report-policy-replay --wait
	@bash scripts/run_report_policy_live_smoke.sh

report-schedule-live-smoke: ## Manual M3.1 scheduled-trigger proof: Temporal Schedule action -> report delivery
	@bash scripts/run_report_schedule_live_smoke.sh

agent-runtime-smoke: ## Manual M4 smoke: candidate sandbox image satisfies ADR-0013 file I/O and security posture
	@bash scripts/run_agent_runtime_smoke.sh

diagnosis-assistant-runner-build: ## Build the local Eino diagnosis runner image and print its immutable ref
	@bash scripts/build_diagnosis_assistant_runner.sh

custom-thin-runner-smoke: ## Manual M4 smoke: build local custom thin runner candidate and prove it through both runtime harnesses
	@bash scripts/run_custom_thin_runner_smoke.sh

container-provider-smoke: ## Manual M4 smoke: Docker-backed ContainerProvider.Run against a real local daemon
	@bash scripts/run_container_provider_smoke.sh

container-provider-timeout-smoke: ## Manual M4 smoke: Docker-backed ContainerProvider timeout stop/remove against a real local daemon
	@OPENCLARION_CONTAINER_PROVIDER_SMOKE_TIMEOUT_SECONDS=2 \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_SOURCE="make container-provider-timeout-smoke" \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_EXPECT_ERROR_CONTAINS="context deadline exceeded" \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON='["sh","-c","sleep 30"]' \
	bash scripts/run_container_provider_smoke.sh

container-provider-output-cap-smoke: ## Manual M4 smoke: Docker-backed ContainerProvider output cap against a real local daemon
	@OPENCLARION_CONTAINER_PROVIDER_SMOKE_OUTPUT_MAX_BYTES=64 \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_SOURCE="make container-provider-output-cap-smoke" \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_EXPECT_ERROR_CONTAINS="exited with code||exceeds maximum" \
	OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON='["sh","-c","dd if=/dev/zero of=/workspace/out/output.json bs=1024 count=1"]' \
	bash scripts/run_container_provider_smoke.sh

egress-allowdeny-smoke: ## Manual M4 smoke: Docker internal network + proxy allows listed egress and denies bypass/unlisted targets
	@bash scripts/run_egress_allowdeny_smoke.sh

local-egress-proxy-build: ## Manual local sandbox egress proxy image build; requires Docker and Go
	@bash scripts/build_local_egress_proxy.sh

sandbox-m4-runtime-smoke-artifacts: ## Manual M4 smoke bundle: retain canonical runtime-smoke artifacts for review evidence
	@bash scripts/run_sandbox_m4_runtime_smoke_artifacts.sh

diagnosis-dev-oidc-issuer: ## Manual M5 helper: local OIDC issuer and short-lived token endpoint
	@go run ./scripts/dev_oidc_issuer $(ARGS)

stage5-local-worker-check: ## Manual M5 helper: validate private env and runtime prerequisites for local worker
	@bash scripts/run_stage5_local_worker.sh --check-only

stage5-local-worker: ## Manual M5 helper: run current API and diagnosis worker from a private env file
	@bash scripts/run_stage5_local_worker.sh

# Future gates are tracked in docs/design/ci/README.md, not as inactive
# Makefile placeholders. Add a target here only when its script/tool,
# CI workflow job, and Schedule row land in the same change.
