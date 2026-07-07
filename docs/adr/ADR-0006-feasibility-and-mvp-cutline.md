---
id: ADR-0006
title: "Feasibility and MVP Cutline"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0006: Feasibility and MVP Cutline

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion is feasible, but the first milestone must avoid coupling value
validation to the hardest interactive-agent path. The project needs an MVP
cutline that proves the product value independently of any specific AI agent
runtime.

## Decision Drivers

* prove evidence capture and report quality early
* keep Go control flow deterministic
* avoid making interactive agent sessions or specific agent runtimes a blocking
  dependency for M0-M2
* preserve future diagnosis-room design through compatible schemas
* keep operational dependencies small
* each milestone must produce a runnable, demonstrable deliverable

## Decision Outcome

**Chosen option**: implement the Go control plane and headless LLM report loop
first. Agent sandbox exploration is a separate later milestone (M4) that does not
block MVP acceptance. Interactive diagnosis is a V1 commitment but its initial
scope is intentionally minimal: a short-conversation diagnosis room with a
bounded number of turns and a fixed session lifetime. Long-session features
(automatic compression, multi-day rooms, complex RBAC tiers) are explicitly
deferred.

### Consequences

* Good, because M0-M2 can prove the alert-to-evidence-to-report loop without
  waiting for an agent runtime decision.
* Good, because M4 and M5 still have explicit delivery slots, so sandbox and
  diagnosis work remains planned rather than open-ended exploration.
* Neutral, because provider breadth and long-session features move behind the
  V1 boundary until the core alert-analysis workflow is stable.
* Bad, because some stakeholders may expect interactive diagnosis before the
  headless report loop has produced representative evidence.

## Milestone Delivery Strategy

| Milestone | Scope | Deliverable | Risk |
|-----------|-------|-------------|------|
| M0 | Bootstrap: Go skeleton, local infra, OpenAPI, codegen | runnable health endpoint + CI | Low |
| M1 | Go control plane: alert polling, sharding, grouping, evidence snapshots, workflow dispatch | replayable alert-to-evidence pipeline | Medium |
| M2 | Headless LLM report loop: structured SubReport, FinalReport, notification | end-to-end evidence-to-report-to-notification | Medium |
| M3 | Report viewer frontend + operational observability | browsable reports with evidence traceability | Medium-low |
| M4 | Agent sandbox exploration: generic ContainerProvider, tool-augmented analysis | enhanced reports from sandboxed agent (PoC) | Medium-high |
| M5 | Short-conversation interactive diagnosis (V1 required) | bounded-turn diagnosis room with sandboxed agent, chat persistence | Medium-high |

## MVP Scope (M0 through M2)

1. Read active alerts from Prometheus-compatible or Alertmanager providers.
2. Deduplicate and group alerts deterministically.
3. Build `EvidenceSnapshot` records.
4. Generate `SubReport` and `FinalReport` through `LLMProvider` (OpenAI-
   compatible API, no agent runtime dependency).
5. Persist evidence and reports in PostgreSQL.
6. Send reports through Webhook provider (Email as a stretch goal).

## Provider Implementation Schedule

| Milestone | Real Providers | Fake Providers |
|-----------|---------------|----------------|
| M1 | MetricsProvider (Prometheus) | CMDB, IM, Auth, Approval, Container, LLM |
| M2 | LLMProvider (OpenAI-compatible), IMProvider (Webhook) | CMDB, Auth, Approval, Container |
| M3 | - (frontend consumes existing API) | - |
| M4 | ContainerProvider (self-built Docker sandbox) | - |
| M5 | AuthProvider (OIDC) | - |

## Non-MVP Scope

* realtime alert routing and paging replacement
* full ticketing system
* long-running diagnosis rooms (multi-day, conversation compression)
* complex multi-tier RBAC beyond owner/admin, including leader-tier flows
* SSO-dependent user experience
* RAG and knowledge-base retrieval
* multi-tenant SaaS operations
* binding to any specific agent runtime (OpenClaw or others)

## V1 Commitment Boundary

M0 through M5 form the V1 commitment. M5 must be delivered, but its scope is
limited:

* **Required for V1**: short-conversation diagnosis room (bounded turns, fixed
  lifetime, owner+admin RBAC, audit trail, chat persistence)
* **Deferred beyond V1**: lifecycle-end summarization, long sessions, complex
  approval flows, advanced unsafe-instruction filtering

The M5 minimum-viable scope justifies the Temporal selection in ADR-0004:
short-but-real human-signal workflows still benefit from durable execution.

## Acceptance Demonstrations

| Milestone | Acceptance Demo |
|-----------|----------------|
| M0 | `make pr` passes; `docker compose up` starts PG+Temporal; healthz returns 200 |
| M1 | import 20 historical alerts -> auto-group -> persist EvidenceSnapshot -> queryable via API |
| M2 | trigger alert batch -> auto-generate SubReport+FinalReport -> Webhook receives notification |
| M3 | open browser -> view report list -> click into report detail with evidence traceability |
| M4 | given evidence -> agent in sandbox calls tools to analyze -> return enhanced report (quality delta vs M2) |
| M5 | authorized user opens diagnosis room -> short-conversation with sandboxed agent (bounded turns) -> chat persisted, audit logged |

### Confirmation

* at least 20 replayed alert windows produce stable snapshots and reports
* three golden prompt scenarios pass: single alert, cascade, alert storm
* failure paths are persisted and retriable
* no specific agent runtime is required for M0-M2 acceptance

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Remove OpenClaw hard binding; restructure to M0-M5 milestones; add acceptance demos |
| 2026-05-19 | jindyzhao | M5 reclassified from optional exploration to V1 required (short-conversation scope); long-session features deferred |
