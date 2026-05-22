---
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
cutline that proves the product value while keeping OpenClaw interaction risk
isolated.

## Decision Drivers

* prove evidence capture and report quality early
* keep Go control flow deterministic
* avoid making interactive agent sessions a blocking dependency
* preserve future diagnosis-room design through compatible schemas
* keep operational dependencies small

## Decision Outcome

**Chosen option**: implement the Go control plane and headless LLM report loop
first. Validate OpenClaw as a headless sandbox proof-of-concept in parallel.
Interactive OpenClaw diagnosis rooms are a later track.

## Early Priority

| Priority | Scope | Risk | Reason |
|----------|-------|------|--------|
| P0 | Go control plane: alert polling, sharding, grouping, evidence snapshots, workflows | Medium | deterministic and testable |
| P1 | headless LLM reports: structured `SubReport` and `FinalReport` | Medium | validates product value quickly |
| P2 | OpenClaw headless sandbox | Medium-high | validates sandbox and output stability |
| P3 | interactive diagnosis room | High | requires identity, RBAC, WebSocket/PTY, session compression, audit |

## MVP Scope

1. Read active alerts from Prometheus-compatible or Alertmanager providers.
2. Deduplicate and group alerts deterministically.
3. Build `EvidenceSnapshot` records.
4. Generate `SubReport` and `FinalReport` through `LLMProvider`.
5. Persist evidence and reports in PostgreSQL.
6. Send reports through Email, Webhook, or Slack providers.
7. Run an OpenClaw headless sandbox PoC without blocking MVP acceptance.

## Non-MVP Scope

* realtime alert routing and paging replacement
* full ticketing system
* interactive OpenClaw diagnosis room
* SSO-dependent user experience
* RAG and knowledge-base retrieval
* multi-tenant SaaS operations

### Confirmation

* at least 20 replayed alert windows produce stable snapshots and reports
* three golden prompt scenarios pass: single alert, cascade, alert storm
* failure paths are persisted and retriable
* OpenClaw headless PoC does not require interactive sessions

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
