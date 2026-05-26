# Roadmap

> Last updated: 2026-05-26
> Author: jindyzhao
> Status: private incubation

## Milestones

```text
M0 Bootstrap  ->  M1 Control Plane  ->  M2 Report Loop  ->  M3 Frontend+Ops  ->  M4 Agent Sandbox  ->  M5 Short-Conversation Diagnosis
```

## M0: Bootstrap

- [x] governance files
- [x] GitHub issue and PR templates
- [x] CI documentation hygiene check
- [x] Go module skeleton
- [x] Docker Compose for PostgreSQL and Temporal
- [x] OpenAPI 3.1 skeleton (`api/openapi.yaml` with healthz)
- [x] oapi-codegen-exp generation chain verified
- [ ] Ent and Atlas toolchain (deferred to M1: pinned at first import per
      DEPENDENCIES.md "first-import pin" rule)
- [x] `make generate`, `make test`, `make lint`, `make pr`
- [x] health endpoint compiles and returns 200
- [x] `vacuum` OpenAPI lint as a real blocking gate

**Acceptance**: `make pr` passes; `docker compose up -d --wait` starts
PostgreSQL+Temporal+Temporal UI; `curl http://localhost:8080/healthz` returns
`{"status":"ok"}`.

> **Scope note**: Ent/Atlas/Temporal SDK pins are intentionally deferred to M1.
> Per the "first-import pin" rule (DEPENDENCIES.md), Go modules enter `go.mod`
> only when production code first imports them. ADR-0012 was amended on
> 2026-05-22 to move the Temporal Workflow Update round-trip validation to M1
> (the first milestone with a real Temporal workflow).

## M1: Go Control Plane

- [x] MetricsProvider interface and Prometheus implementation
- [x] AlertEvent Ent schema and migrations
- [ ] alert window replay harness
- [x] deterministic grouping algorithm
- [x] EvidenceSnapshot schema and builder
- [x] Temporal workflow bootstrap (DiagnosisWorkflow shell)
- [ ] API endpoints: list alerts, list snapshots

**Acceptance**: import 20 historical alerts -> auto-group -> persist
EvidenceSnapshot -> queryable via API.

> **Scope note**: Provider fake policy -- each provider port ships with its
> own fake in the same first-import PR (M1-PR4 already shipped fake
> `MetricsProvider`). The previously listed standalone "fake providers for
> all other interfaces" checklist item has been removed because there is no
> standalone milestone task for it; subsequent provider ports (LLM in M2,
> IM in M2, Container in M4, Auth in M5) bring their own fakes alongside
> the port definition.

## M2: Headless Report Loop

- [ ] LLMProvider interface
- [ ] OpenAI-compatible provider implementation
- [ ] fake LLM provider (deterministic, for tests)
- [ ] prompt templates (single alert, cascade, alert storm)
- [ ] structured JSON output parser with schema validation
- [ ] retry mechanism (3 attempts with error feedback)
- [ ] SubReport and FinalReport Ent schemas
- [ ] ReportFanOutWorkflow and FinalReportWorkflow
- [ ] golden prompt tests (structure validation, not content)
- [ ] IMProvider interface and Webhook implementation
- [ ] report notification flow
- [ ] API endpoints: list reports, get report detail

**Acceptance**: trigger alert batch -> auto-generate SubReport+FinalReport ->
Webhook receives notification.

## M3: Frontend and Observability

- [ ] Next.js project initialization (`web/`)
- [ ] generated TypeScript API types from OpenAPI
- [ ] report list page
- [ ] report detail page with evidence traceability
- [ ] basic dashboard: alert counts, report success rate
- [ ] OpenTelemetry instrumentation on Go services
- [ ] Prometheus metrics endpoint
- [ ] structured logging with correlation IDs

**Acceptance**: open browser -> view report list -> click into report detail
with evidence traceability links.

## M4: Agent Sandbox Baseline and Exploration

- [ ] ContainerProvider interface
- [ ] self-built Docker sandbox (non-root, readonly fs, network allowlist)
- [ ] evidence injection via mounted files
- [ ] tool scripts (metric query, topology lookup)
- [ ] timeout and cleanup logic
- [ ] output extraction via /workspace/out/output.json (file-based contract)
- [ ] minimum sandbox baseline for M5 (auditable file-based I/O, cleanup, limits)
- [ ] quality comparison: sandbox report vs direct LLM report
- [ ] decision gate: report-enhancement track proceeds, iterates, or stays deferred

**Acceptance**: given evidence -> agent in sandbox calls tools -> return
enhanced report with measurable quality delta vs M2 output; minimum sandbox
baseline is ready for M5 short-conversation diagnosis.

## M5: Short-Conversation Interactive Diagnosis (V1 Required)

> Implements the per-turn container invocation contract from
> [ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

Delivered as part of V1 with intentionally minimal scope. See
[phases/05-interactive-diagnosis.md](../design/phases/05-interactive-diagnosis.md)
for the V1 scope boundary.

- [ ] AuthProvider interface and OIDC implementation
- [ ] RBAC checks (owner and admin; leader is deferred)
- [ ] DiagnosisRoomWorkflow (Temporal: signals, queries, durable timers)
- [ ] ChatSession and ChatTurn Ent schemas
- [ ] bounded-turn enforcement (config-driven turn ceiling)
- [ ] fixed session lifetime + idle timeout
- [ ] diagnosis room route (Next.js)
- [ ] WebSocket proxy with authenticated handshake
- [ ] unsafe-instruction filter (deny-list)
- [ ] audit logging for session lifecycle
- [ ] final group notification on session close

**Acceptance**: authorized user opens diagnosis room -> short-conversation
with sandboxed agent within turn and time limits -> chat persisted, audit
logged, final notification sent.

**Explicitly Out-of-Scope (V1)**: lifecycle-end compression, multi-day
sessions, leader-tier approval, streaming partial responses.

## Future

- [ ] pgvector retrieval (RAG)
- [ ] Kubernetes Job ContainerProvider
- [ ] DingTalk and Feishu IM providers
- [ ] NetBox CMDB provider
- [ ] scheduled weekly and monthly reports
- [ ] multi-tenant operations
- [ ] Email and Slack IM providers (upgrade from Webhook)

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | English roadmap reset and MVP cutline |
| 2026-05-19 | jindyzhao | Restructure to M0-M5; remove OpenClaw binding; add acceptance criteria |
| 2026-05-19 | jindyzhao | M5 reclassified as V1 required (short-conversation scope); defer compression and long sessions |
| 2026-05-26 | jindyzhao | M1-PR4 lands `MetricsProvider` interface + Prometheus implementation + in-memory fake provider + `alertingest.IngestOnce` library. Item ticked: `MetricsProvider interface and Prometheus implementation`. `AlertEvent Ent schema and migrations` and `Temporal workflow bootstrap (DiagnosisWorkflow shell)` were already shipped in M1-PR1 and M1-PR3 respectively and are now ticked retroactively to keep this checklist in sync with `CURRENT_STATE.md`. Remaining M1 items (`alert window replay harness`, `deterministic grouping algorithm`, `EvidenceSnapshot schema and builder`, `fake providers for all other interfaces`, `API endpoints`) are explicitly out of scope for M1-PR4. |
| 2026-05-26 | jindyzhao | M1-PR5 lands deterministic grouping algorithm (`GroupEvents` pure function in `internal/usecases/alertgrouping/`). Item ticked: `deterministic grouping algorithm`. |
| 2026-05-26 | jindyzhao | M1-PR6 lands EvidenceSnapshot deterministic builder (`BuildSnapshot` pure function in `internal/usecases/evidencebuild/`). Item ticked: `EvidenceSnapshot schema and builder` (schema shipped in M1-PR1, builder in M1-PR6). |
| 2026-05-26 | jindyzhao | M1-PR6 shipped post-merge as `22428fb`. M1-PR7 in progress: alert window replay harness in `internal/usecases/alertreplay/` (`ReplayWindow(ctx, provider, factory, Request) (Stats, error)` orchestrator that ingests via `alertingest.IngestOnce`, re-reads the half-open `[WindowStart, WindowEnd)` window with `Limit+1` as a safety valve, runs deterministic grouping, then a per-group pipeline transaction covering save / refresh / existing diff -> link events -> `BuildSnapshot` -> save snapshot when not duplicate -> close group when previously active). `AlertRepository` ports gained `ListEventsByStartsAtRange(start, end, limit)` and `FindGroupByNaturalKey(groupKey, firstSeenAt)`; both reject zero/blank inputs and the range method compares `end > start` after `NormalizeUTCMicro` so sub-microsecond windows the normaliser collapses are rejected. D7 closed->refresh tension: refresh updates mutable fields of an already-closed group when the same `(group_key, first_seen_at)` natural key recurs, but status is preserved (no reopen); a window whose `first_seen_at` differs creates a NEW row by design. M1 checklist `alert window replay harness` will be ticked when the PR merges; the previously listed standalone `fake providers for all other interfaces` checklist item is removed and replaced by a scope note above. |
