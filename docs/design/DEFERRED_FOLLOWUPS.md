# Deferred Follow-ups

> Tracks decisions that have been intentionally deferred. Each entry records
> what was deferred, why, and the trigger that should bring it back. This
> prevents deferred items from being silently lost and prevents past
> discussions from being re-litigated without new information.

> Last updated: 2026-07-17

## How To Use This File

* Add an entry when a decision is made to defer something.
* Each entry must specify a re-evaluation trigger or a target milestone.
* Remove an entry only when the work is shipped (and recorded in
  [CURRENT_STATE.md](CURRENT_STATE.md)) or the decision is reaffirmed in a
  superseding ADR.

## Active Deferrals

### D5: Streaming Token-Level Partial Responses (M5)

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Updated | 2026-07-17 |
| Reason | The shipped diagnosis path already projects model deltas into bounded, transient semantic `message` snapshots and renders them as a replaceable assistant draft without persisting preview text. What remains deferred is a browser-facing raw token/delta protocol, which would expose incomplete structured output and add retry, ordering, and compatibility cost without improving the durable turn contract. |
| Trigger | UX evidence shows semantic draft snapshots are insufficient and that raw token-level rendering is required |
| Target | post-V1 |

### D8: Kubernetes Job ContainerProvider

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | M4 Docker sandbox is sufficient for control-plane-owned analysis. Kubernetes Job runtime is a future ContainerProvider implementation behind the same interface. |
| Trigger | deployment requirements demand orchestrated multi-tenant sandboxes |
| Target | post-V1 |

### D10: Phase Checklist File Split

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Updated | 2026-05-30 |
| Reason | M5 re-evaluation keeps the single `CHECKLIST.md`: it is 232 lines, still milestone-scannable, and detailed implementation history already lives in `CURRENT_STATE.md`, `docs/roadmap/tasks.md`, phase docs, and the M4 evidence documents. Splitting into `checklist/phase-*.md` would add navigation and link-maintenance cost without improving the current review flow. |
| Trigger | revisit only if `CHECKLIST.md` exceeds 400 lines, gains separate phase owners, or review feedback shows that single-file milestone scanning is blocking delivery |
| Target | post-V1, only if one trigger occurs |

## Closed Deferrals

### D1: Workflow Engine Spike (Temporal vs River+sqlc)

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-05-30 |
| Reason | The early spike is no longer needed. Temporal is already exercised by the shipped M2 report pipeline (`ReportBatchWorkflow`, `ReportFanOutWorkflow`, and `FinalReportWorkflow`) and the M5 diagnosis-room workflow (Updates, Queries, Signals, durable timers, per-turn sandbox Activity, transcript persistence, lifecycle audit, and close notification). The implementation is covered by `make temporal-workflow-tests`, `make diagnosis-room-workflow-test`, local report E2E, and frontend diagnosis route smoke. A River+sqlc comparison would now be a rewrite evaluation rather than a pre-commit architecture cross-check. |
| Trigger | closed after M2/M5 Temporal workflow evidence landed; reopen only if Temporal operations become a measured delivery or reliability blocker |
| Target | closed by M5 workflow evidence |

### D2: oapi-codegen-exp StrictServerInterface Adapter

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-05-30 |
| Reason | Re-evaluation found no V1 maintainability pain that justifies a manual strict adapter. The pinned `oapi-codegen-exp v0.1.0` generator emits 9 `ServerInterface` methods and the transport layer implements them directly with a compile-time `api.ServerInterface` assertion. Request decoding and response mapping remain local to the affected handlers, while `internal/transport/http` has endpoint-level coverage for list, detail, replay trigger, diagnosis room, WebSocket ticket, and WebSocket relay paths. A handwritten strict adapter would now duplicate generated request binding and add another transport contract to maintain. |
| Trigger | closed after M2/M5 handler growth did not materialize into adapter pain; reopen only if upstream generates a stable strict interface for the pinned OpenAPI 3.1 path, or if repeated handler request/response boilerplate is measured in review |
| Target | closed by V1 API handler evidence |

### D3: Lifecycle-End Conversation Compression (M5)

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-07-14 |
| Reason | A post-V1 deterministic lifecycle-end summary now reads the complete persisted transcript, stores one immutable versioned `ChatSessionSummary` bound to ordered source turns, preserves every `ChatTurn`, and exposes the bounded read model through workflow, WebSocket, REST, and frontend state. This does not introduce periodic semantic compression or unbounded active sessions. |
| Trigger | closed after lifecycle-end summary persistence and projection landed; reopen only if active multi-day sessions require periodic semantic compression |
| Target | landed post-V1 |

### D4: Leader-Tier RBAC and Approval Flows (M5)

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-07-14 |
| Reason | Product requirements now include an explicit owner-and-leader conclusion quorum. Diagnosis rooms support a per-room `single` or `owner_and_leader` approval mode, a dedicated conclusion-approval permission, immutable actor approvals bound to the current assistant conclusion digest, and a persisted quorum check before terminal close. |
| Trigger | closed after the product requirement and implementation landed; reopen only for new quorum types or external governance integration |
| Target | landed post-V1 |

### D6: Concrete Version Pinning for Temporal SDK / Ent / Atlas / OTel

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-05-22 (M0 review), 2026-05-22 (M1-PR1 start), 2026-05-25 (M1-PR3 Temporal SDK pin), 2026-05-28 (M3 OTel HTTP tracing pin) |
| Reason | Original deferral assumed M0 would replace `TBD pinned at M0` placeholders for these modules. Post-M0 review (2026-05-22) replaced that policy with the `first-import pin` rule in DEPENDENCIES.md: a Go module enters `go.mod` only when production code first imports it, and is pinned to a concrete `module@version` at that moment. ADR-0012 was amended (2026-05-22) so the Temporal SDK pin and Update round-trip validation move from M0 to M1. The `forbidden-latest` CI gate keeps enforcing the no-`latest` rule throughout. **M1-PR1 (2026-05-22)** lands the Ent and Atlas pins: `entgo.io/ent v0.14.6` (direct require + `tool` directive in `go.mod`) and Atlas CLI `arigaio/atlas:1.2.0` (Docker image pin). The Atlas wrapper redesign (2026-05-22) replaced the original Docker-socket plan with a host-launched ephemeral `postgres:18-alpine` on a per-invocation dedicated Docker network, with Atlas reaching the dev DB via a plain `postgres://` URL; no host Docker socket is mounted, and the `docker://...` dev-url form is not used. Atlas is **not** added to `go.mod` because it is invoked as a CLI via the pinned image rather than imported as a Go library; the pin lives in `Makefile` (`ATLAS_IMAGE`) and `docs/design/DEPENDENCIES.md`. **M1-PR3 (2026-05-25)** lands the Temporal Go SDK pin: `go.temporal.io/sdk v1.44.0` (direct require in `go.mod`), entering via the first-import rule when `internal/orchestrator/temporal/` production code first imports it. **M3 backend observability (2026-05-28)** lands OpenTelemetry Go direct pins through `internal/observability/tracing`: `go.opentelemetry.io/otel v1.44.0`, `go.opentelemetry.io/otel/sdk v1.44.0`, OTLP HTTP trace exporter `v1.44.0`, and `otelhttp v0.68.0`; this version family is above the `GO-2026-4985` fixed-in floor reported by `govulncheck`. |
| Trigger | Closed by M3 OpenTelemetry HTTP tracing first import. |
| Target | M3 (OpenTelemetry Go) |

### D7: pgvector / RAG Retrieval

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Updated | 2026-07-17 |
| Reason | The report path and diagnosis room now share the optional model-scoped pgvector corpus. Report prompts and new-version diagnosis turns consume bounded advisory historical context, fail open when optional retrieval is unavailable, and persist explicit source references that are projected through workflow, REST, WebSocket, and frontend state without treating history as current evidence. The idempotent reindex command covers existing accepted reports. |
| Trigger | closed after report and diagnosis-room retrieval, provenance, and focused workflow coverage landed; reopen only for a new retrieval corpus or evidence-authority model |
| Target | landed post-V1 |

### D9: Email IMProvider Implementation

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-19 |
| Reason | Profile-backed SMTP Email notification channels now resolve deployment-managed SMTP URL secrets for report-scoped delivery. |
| Trigger | closed after profile-backed Email provider support landed |
| Target | landed |

### D11: Legacy Forbidden-Imports Bash Deletion

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-27 |
| Reason | W3-2a originally landed the `openclarion-arch` forbidden-imports analyzer in parallel with the existing `scripts/check_no_forbidden_imports.sh` gate. The plan was revised on 2026-05-27 to allow immediate retirement after rigorous local equivalence verification: analyzer fixtures cover legacy forbidden modules, concrete provider boundaries, fake-provider test exemptions, and the analyzer retains a unit test pinning the retired legacy deny-list. |
| Trigger | closed on 2026-05-27 by W3-2b |
| Target | W3-2b |

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-07-17 | jindyzhao | Clarify D5 after reviewing the shipped streaming path: bounded semantic assistant snapshots are implemented end to end; only browser-facing raw token/delta delivery remains deferred. |
| 2026-07-17 | jindyzhao | Close D3 and D7 after the immutable lifecycle-end summary and bounded diagnosis-room historical report retrieval landed with source preservation, advisory provenance, and focused workflow coverage. |
| 2026-07-14 | jindyzhao | Update D7 after report-side pgvector retrieval landed; keep the deferral open only for diagnosis-room consumption and its provenance contract. |
| 2026-07-14 | jindyzhao | Close D4 after configurable single-party and owner-plus-leader conclusion approval landed with digest-bound immutable approvals and persisted quorum enforcement. |
| 2026-05-19 | jindyzhao | Initial set of deferrals: spike, Strict adapter, M5 long-session features, version pinning, future providers |
| 2026-05-22 | jindyzhao | Update D6: replace "must pin at M0" policy with the `first-import pin` rule (DEPENDENCIES.md). Temporal/Ent/Atlas pinning targets shift to M1; OTel to M3. ADR-0012 amended in the same window. |
| 2026-05-22 | jindyzhao | D6 partially closed at M1-PR1 start: Ent v0.14.6 pinned (direct require + tool directive); Atlas pinned via `arigaio/atlas:1.2.0` Docker image under the original Plan A draft (Dockerized Atlas with mounted Docker socket; later superseded by the wrapper redesign in the same milestone window). Temporal SDK and OTel remain open. |
| 2026-05-25 | jindyzhao | D6 Temporal SDK portion closed at M1-PR3: `go.temporal.io/sdk v1.44.0` pinned via first-import rule. Only OTel remains open for M3. |
| 2026-05-27 | jindyzhao | Add D11 for W3-2b: legacy forbidden-imports bash deletion is deferred until the two-week analyzer equivalence window completes. |
| 2026-05-27 | jindyzhao | Close D11 after revising the plan to permit immediate retirement on rigorous local equivalence proof; analyzer tests now pin the retired legacy deny-list and cover red/green fixtures. |
| 2026-07-08 | jindyzhao | Close D9 after profile-backed SMTP Email notification channels landed for report-scoped delivery. |
| 2026-05-28 | jindyzhao | D6 closed: M3 OpenTelemetry HTTP tracing first import pins `go.opentelemetry.io/otel v1.44.0`, `go.opentelemetry.io/otel/sdk v1.44.0`, OTLP HTTP trace exporter `v1.44.0`, and `otelhttp v0.68.0`; collector smoke and broader tracing coverage are tracked as M3 implementation work, not dependency-pin deferral. |
| 2026-05-30 | jindyzhao | D2 closed after re-evaluation: the generated `ServerInterface` handler surface remains small and covered by endpoint tests, so a handwritten strict adapter would add maintenance cost without proven V1 value. |
| 2026-05-30 | jindyzhao | D10 re-evaluated after M2/M5 growth: keep the single delivery checklist for now and replace the expired M2-review trigger with concrete size, ownership, and reviewer-friction triggers. |
| 2026-05-30 | jindyzhao | Add deferred follow-up ledger gate and move closed D6/D11 entries under the Closed Deferrals section so status and section stay machine-checkable. |
| 2026-05-30 | jindyzhao | D1 closed after re-evaluation: the M2 report workflows and M5 diagnosis-room workflow now provide enough Temporal evidence that the River+sqlc spike would be a rewrite evaluation rather than an early architecture cross-check. |
