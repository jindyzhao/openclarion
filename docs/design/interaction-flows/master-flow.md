# Master Flow

This document describes the target product interaction flow. The MVP implements
the headless report path first and leaves interactive diagnosis for a later
track.

## Stage S0: Alert State Ingestion

OpenClarion reads active alert state through `MetricsProvider`. It does not own
realtime paging or on-call routing.

## Stage S1: Deterministic Grouping

The Go control plane deduplicates alerts, groups them by configured dimensions,
and writes replayable records.

## Stage S2: Evidence Snapshot

The control plane enriches grouped alerts through `CMDBProvider` and metric
queries, then persists an `EvidenceSnapshot`.

## Stage S3: Headless Report Fan-Out

Temporal fans out report activities. Each group produces a structured
`SubReport` through `LLMProvider`.

## Stage S4: Final Report Reduce

Temporal reduces subreports into a `FinalReport`, persists it, and sends it
through `IMProvider`.

## Stage S5: Optional OpenClaw Headless Sandbox

OpenClaw may be invoked as a short-lived sandbox for comparison or richer
analysis. This path is headless, bounded, and does not require user interaction.

## Stage S6: Later Interactive Diagnosis

A future diagnosis room allows authorized users to converse with a short-lived
OpenClaw container. The Go control plane owns identity, RBAC, WebSocket proxy,
audit, timeout, and lifecycle-end compression.

## Closure Outcomes

| Outcome | Meaning |
|---------|---------|
| `true_positive` | real issue confirmed |
| `false_positive` | alert did not represent a real issue |
| `expected_maintenance` | expected change or planned maintenance |
| `self_resolved` | recovered before human action |
| `unresolved_escalated` | still firing and escalated |

## Invariants

- AI does not route alerts.
- AI does not own workflow lifecycle.
- AI does not execute production-changing actions.
- Go verifies external state through providers.
- Reports remain traceable to evidence snapshots.
