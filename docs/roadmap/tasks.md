# Roadmap

> Last updated: 2026-05-19
> Author: jindyzhao
> Status: private incubation

## Milestones

```text
M0 Bootstrap  ->  M1 Control Plane  ->  M2 Report Loop  ->  M3 Frontend+Ops  ->  M4 Agent Sandbox  ->  M5 Short-Conversation Diagnosis
```

## M0: Bootstrap

- [ ] governance files
- [ ] GitHub issue and PR templates
- [ ] CI documentation hygiene check
- [ ] Go module skeleton
- [ ] Docker Compose for PostgreSQL and Temporal
- [ ] OpenAPI 3.1 skeleton (`api/openapi.yaml` with healthz)
- [ ] oapi-codegen-exp generation chain verified
- [ ] Ent and Atlas toolchain
- [ ] `make generate`, `make test`, `make lint`, `make pr`
- [ ] health endpoint compiles and returns 200

**Acceptance**: `make pr` passes; `docker compose up` starts PG+Temporal;
healthz returns 200.

## M1: Go Control Plane

- [ ] MetricsProvider interface and Prometheus implementation
- [ ] AlertEvent Ent schema and migrations
- [ ] alert window replay harness
- [ ] deterministic grouping algorithm
- [ ] EvidenceSnapshot schema and builder
- [ ] Temporal workflow bootstrap (DiagnosisWorkflow shell)
- [ ] fake providers for all other interfaces
- [ ] API endpoints: list alerts, list snapshots

**Acceptance**: import 20 historical alerts -> auto-group -> persist
EvidenceSnapshot -> queryable via API.

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
