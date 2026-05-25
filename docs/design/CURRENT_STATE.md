# Current State

> Snapshot of what has actually shipped. Updated as code lands. This is the
> truth source for "where are we now". Forward-looking plans live in
> [../roadmap/tasks.md](../roadmap/tasks.md). Decisions intentionally not
> done live in [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md).

> Last updated: 2026-05-25
> Stage: M1-PR2 in progress (domain layer + persistence repository
> contracts + `UnitOfWork` + DI wiring on top of the M1-PR1 foundation,
> which shipped post-merge as `b7233c7` -- 5 Ent schemas (`AlertEvent`,
> `AlertGroup`, `EvidenceSnapshot`, `DiagnosisTask`,
> `DiagnosisTaskEvent`), the first migration
> `20260525060310_initial_schema.sql`, the redesigned Atlas wrapper,
> and the `cmd/openclarion` + `/healthz` service surface). The
> Temporal Go SDK first-import pin and `DiagnosisWorkflow` shell are
> scoped to M1-PR3 per the first-import rule: dependencies enter
> `go.mod` only when production code first requires them, not to
> satisfy roadmap dates.)

## Implementation Status

| Area | Status | Notes |
|------|--------|-------|
| Repository governance (LICENSE, GOVERNANCE, MAINTAINERS, etc.) | shipped | English-only documentation |
| ADRs 0001-0012 | shipped (proposed status) | technology stack and architecture decisions recorded |
| Phase docs 00-05 | shipped | each milestone has a phase document |
| Master flow (S0-S6) | shipped | stage-by-stage authority and failure boundaries documented |
| CI: docs hygiene gate | shipped | `scripts/check_no_non_english_chars.sh` |
| CI: Go checks gate | shipped | `make go-checks` (generate, vet, build, test) |
| CI: OpenAPI checks gate | shipped | `make openapi-checks` (`vacuum` lint + `openapi-fresh`) |
| Go module | shipped | `github.com/openclarion/openclarion`, Go 1.25.9, oapi-codegen-exp v0.1.0, vacuum (tools) |
| OpenAPI spec | shipped | `api/openapi.yaml` (3.1.0, healthz endpoint, vacuum 100/100) |
| `oapi-codegen-exp` toolchain | shipped | V3, std net/http ServerInterface, `go generate` chain |
| `vacuum` OpenAPI lint | shipped | `go tool` dependency, `--fail-severity error`, real blocking gate |
| Docker Compose (PostgreSQL + Temporal) | shipped | tag-pinned (`postgres:18-alpine`, `temporalio/auto-setup:1.25.2`); digest pin deferred to M4 sandbox |
| Health endpoint | shipped | `GET /healthz` returns 200 with `{"status":"ok"}` |
| Ent toolchain | shipped (M1-PR1) | `entgo.io/ent v0.14.6` direct require + `tool` directive; `make ent-generate` / `make ent-fresh` |
| Atlas toolchain | shipped (M1-PR1) | `arigaio/atlas:1.2.0` Docker image pin; wrapper landed in `scripts/lib_atlas.sh` plus three thin entry scripts: host launches per-invocation `postgres:18-alpine` on a dedicated Docker network, Atlas container mounts host Go toolchain read-only at `/usr/local/go`, runs as `$(id -u):$(id -g)`, talks to dev DB via plain `postgres://`. `make atlas-migrate-diff` / `make atlas-drift` / `make atlas-smoke` targets shipped; `make atlas-smoke` verified locally 2026-05-22 (produced 2 files, clean cleanup); `make atlas-drift` runs in CI with `actions/setup-go` so the host Go toolchain is mounted into the Atlas container. |
| Ent schemas | shipped (M1-PR1) | 5 entities landed: `AlertEvent`, `AlertGroup`, `EvidenceSnapshot`, `DiagnosisTask`, `DiagnosisTaskEvent`. AlertEvent <-M2N-> AlertGroup; AlertGroup -1:N-> EvidenceSnapshot; EvidenceSnapshot -1:N-> DiagnosisTask; DiagnosisTask -1:N-> DiagnosisTaskEvent. All entities use bigserial PK; FK columns are surfaced as explicit `field.Int` (`alert_group_id` / `evidence_snapshot_id` / `task_id`) so composite-index column ordering matches docs intent. Constraint set after 2026-05-22 review: `EvidenceSnapshot.digest` is **per-group** unique on `(alert_group_id, digest)` (NOT cross-row global, since two distinct groups MAY produce identical canonical payloads); `DiagnosisTask` natural identity is `(workflow_id, run_id)` (NOT `workflow_id` alone), with `run_id` NOT NULL + immutable, so Temporal retries that spawn a new `run_id` are NEW rows. First migration `20260525060310_initial_schema.sql` committed; `make atlas-drift` reports synced. |
| Temporal Go SDK / workflows | not started | M1-PR3 deliverable (`DiagnosisWorkflow` shell + Update round-trip integration test, per ADR-0012 amendment); SDK enters `go.mod` only when that PR's production code first imports it (first-import rule) |
| LLMProvider | not started | M2 deliverable |
| IMProvider Webhook | not started | M2 deliverable |
| Frontend (Next.js) | not started | M3 deliverable |
| ContainerProvider sandbox | not started | M4 deliverable (digest pin gate activates here) |
| AuthProvider OIDC | not started | M5 deliverable |
| Diagnosis room | not started | M5 deliverable (V1 short-conversation scope) |

## Active ADRs (Quick Index)

| ADR | Status | Subject |
|-----|--------|---------|
| 0001 | proposed | PostgreSQL single source of truth |
| 0002 | proposed | AI agent black-box principle |
| 0003 | proposed | Provider extension interfaces |
| 0004 | proposed | Temporal workflow engine (driven by M5) |
| 0005 | proposed | Ephemeral container security |
| 0006 | proposed | Feasibility and MVP cutline (M5 = V1 required, short-conversation scope) |
| 0007 | proposed | OpenAPI 3.1 native toolchain (`oapi-codegen-exp` V3) |
| 0008 | proposed | Monorepo repository structure |
| 0009 | proposed | Go control plane scheduling |
| 0010 | proposed | Frontend architecture |
| 0011 | proposed | CI governance |
| 0012 | proposed | Technology stack validation (Gin removed, Node.js 24.x LTS, Next.js 16, generic agent sandbox) |

## Open Validation Items Before M1

* **Atlas wrapper smoke verified locally on 2026-05-22.** The wrapper
  (`scripts/lib_atlas.sh` plus three thin entry scripts) ran
  `make atlas-smoke` end-to-end against host docker and produced 2
  files; cleanup left no residual `.atlas-*` directories,
  `atlas-pg-*` containers, or `atlas-net-*` networks. The CI
  `atlas-drift` job already runs `actions/setup-go` so the host Go
  toolchain is available when the wrapper mounts it into the Atlas
  container. See [database/migrations.md](database/migrations.md) for
  the full wrapper contract and the empirical findings table that
  drove the redesign.
* **Remaining four Ent schemas landed on 2026-05-22** (`AlertGroup`,
  `EvidenceSnapshot`, `DiagnosisTask`, `DiagnosisTaskEvent`); first
  migration cut via `make atlas-migrate-diff NAME=initial_schema`
  (`20260525060310_initial_schema.sql`, recut after the same-day
  review that tightened `EvidenceSnapshot` to per-group unique digest
  and `DiagnosisTask` to `(workflow_id, run_id)` natural identity);
  `make atlas-drift` reports
  synced.
* Temporal Go SDK first-import pin (>= 1.21) and Update round-trip integration
  test -- planned for **M1-PR3** (`DiagnosisWorkflow` shell), per ADR-0012
  amendment. The dependency enters `go.mod` only when M1-PR3 production code
  first imports it; not earlier.

## Non-Blocking Cross-Checks

* optional spike comparing Temporal vs River+sqlc on M2 fan-out/fan-in
  (cross-check ADR-0004 if the M1-PR3 Temporal setup feels disproportionate)

## Update Discipline

* This file is updated in the same PR that changes implementation status.
* Status entries use: `not started`, `in progress`, `shipped`, `deferred`.
* When a row moves to `deferred`, a corresponding entry must appear in
  [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md).

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-19 | jindyzhao | Initial snapshot at pre-M0 state |
| 2026-05-22 | jindyzhao | M0 bootstrap complete: Go module, Docker Compose, OpenAPI, oapi-codegen-exp, health endpoint, CI gates |
| 2026-05-22 | jindyzhao | Post-review fixes: PostgreSQL `postgres:18-alpine` tag (digest scrapped); `vacuum` promoted to real blocking gate via `go tool`; Ent / Atlas / Temporal SDK pin moved to M1 per first-import rule; ADR-0012 amended to move Temporal Update validation to M1 |
| 2026-05-22 | jindyzhao | M1-PR1 start: persistence foundation. `entgo.io/ent v0.14.6` pinned (direct require + tool directive); Atlas pinned to `arigaio/atlas:1.2.0` Docker image (original Plan A draft: Dockerized Atlas with mounted Docker socket -- later superseded by the wrapper redesign in the same milestone window); `AlertEvent` Ent schema landed as smoke target; `make ent-fresh` and `make atlas-drift` gates landed in CI; `make atlas-smoke` and remaining four schemas pending host-docker validation |
| 2026-05-22 | jindyzhao | M1-PR1 Atlas wrapper redesign landed: dropped `--dev-url docker://...` + Docker socket approach (Atlas image lacks Docker CLI and Go runtime); new wrapper (`scripts/lib_atlas.sh` plus three thin entry scripts) launches dev Postgres from host on a dedicated Docker network, mounts host Go toolchain read-only into Atlas container, runs as `$(id -u):$(id -g)`, dev-url is plain `postgres://`; `raw_payload` corrected from `bytea` to JSONB; CI `atlas-drift` job acquired `actions/setup-go`; AlertEvent primary key locked as Ent default `bigserial` (UUID reserved for security-sensitive single-use tokens only) |
| 2026-05-22 | jindyzhao | M1-PR1 Atlas smoke gate verified on host docker: `make atlas-smoke` produced 2 files end-to-end via the redesigned wrapper; cleanup left no residual `.atlas-*` dirs, `atlas-pg-*` containers, or `atlas-net-*` networks. Remaining four Ent schemas and the first migration are next. |
| 2026-05-22 | jindyzhao | M1-PR1 schemas + first migration landed: `AlertGroup`, `EvidenceSnapshot`, `DiagnosisTask`, `DiagnosisTaskEvent` Ent schemas committed (M2N AlertEvent<->AlertGroup; 1:N down the chain; all bigserial PKs); FK columns surfaced as explicit `field.Int` so composite-index column ordering matches docs intent (`(alert_group_id, created_at)`, `(task_id, dedupe_key) UNIQUE`, `(task_id, occurred_at)`); first migration cut via `make atlas-migrate-diff NAME=initial_schema`; `make atlas-drift` reports synced. |
| 2026-05-22 | jindyzhao | M1-PR1 schema review fixes pre-baseline: (1) `EvidenceSnapshot.digest` no longer table-wide UNIQUE -- replaced with composite `UNIQUE (alert_group_id, digest)` because the model is `AlertGroup -1:N-> EvidenceSnapshot` and two groups MAY produce identical canonical payloads; (2) `DiagnosisTask` identity changed from `UNIQUE(workflow_id)` + optional latest-`run_id` to natural `UNIQUE(workflow_id, run_id)` with `run_id` NOT NULL + immutable, plus a non-unique `workflow_id` chain index, so Temporal retries that spawn a new `run_id` are NEW rows (matches Temporal's own `(workflow_id, run_id)` event-history boundary); (3) `docs/design/DEPENDENCIES.md` and `DEFERRED_FOLLOWUPS.md` cleaned of leftover `--dev-url docker://...` / mounted-Docker-socket descriptions (those describe a plan that was abandoned during the same-day wrapper redesign); (4) `ATLAS_IMAGE` and `ENT_SCHEMA_URL` are now propagated explicitly from `Makefile` to wrapper scripts via per-target `ATLAS_IMAGE="$(ATLAS_IMAGE)" bash ...` recipes -- the lib_atlas.sh defaults are now an explicit fallback for direct script debugging only. Initial migration recut as `20260525060310_initial_schema.sql`; `make atlas-drift` reports synced; `make pr` all gates passed. |
| 2026-05-22 | jindyzhao | M1-PR1 shipped post-merge as `b7233c7`. Stage moves to **M1-PR2** (domain layer + persistence repository contracts + `UnitOfWork` + DI wiring). Temporal Go SDK first-import pin (>= 1.21) and Update round-trip integration test scoped to **M1-PR3** with the `DiagnosisWorkflow` shell, per ADR-0012 amendment and the first-import rule (dependencies enter `go.mod` only when production code first imports them). `DEPENDENCIES.md`, `ADR-0004`, and `END_TO_END_VERIFICATION.md` aligned with this scope split. |
