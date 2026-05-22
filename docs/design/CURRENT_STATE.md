# Current State

> Snapshot of what has actually shipped. Updated as code lands. This is the
> truth source for "where are we now". Forward-looking plans live in
> [../roadmap/tasks.md](../roadmap/tasks.md). Decisions intentionally not
> done live in [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md).

> Last updated: 2026-05-19
> Stage: pre-M0 (documentation + governance only)

## Implementation Status

| Area | Status | Notes |
|------|--------|-------|
| Repository governance (LICENSE, GOVERNANCE, MAINTAINERS, etc.) | shipped | English-only documentation |
| ADRs 0001-0012 | shipped (proposed status) | technology stack and architecture decisions recorded |
| Phase docs 00-05 | shipped | each milestone has a phase document |
| Master flow (S0-S6) | shipped | stage-by-stage authority and failure boundaries documented |
| CI: docs hygiene gate | shipped | `scripts/check_no_non_english_chars.sh` |
| Go module | not started | M0 deliverable |
| OpenAPI spec | not started | M0 deliverable |
| `oapi-codegen-exp` toolchain | not started | M0 deliverable |
| PostgreSQL + Temporal compose | not started | M0 deliverable |
| Ent schemas | not started | M1 deliverable |
| Temporal workflows | not started | M1+ deliverable |
| LLMProvider | not started | M2 deliverable |
| IMProvider Webhook | not started | M2 deliverable |
| Frontend (Next.js) | not started | M3 deliverable |
| ContainerProvider sandbox | not started | M4 deliverable |
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

## Open Validation Items Before M0

* confirm `oapi-codegen-exp` commit hash and pin policy
* draft `Makefile` targets: `generate`, `test`, `lint`, `pr`
* draft `docker-compose.yml` for PostgreSQL 18 + Temporal dev-server

## Non-Blocking Cross-Checks

* optional spike comparing Temporal vs River+sqlc on M2 fan-out/fan-in
  (cross-check ADR-0004 if M0 Temporal setup feels disproportionate)

## Update Discipline

* This file is updated in the same PR that changes implementation status.
* Status entries use: `not started`, `in progress`, `shipped`, `deferred`.
* When a row moves to `deferred`, a corresponding entry must appear in
  [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md).

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-19 | jindyzhao | Initial snapshot at pre-M0 state |
