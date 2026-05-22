# Master Flow

This document is the executable interaction truth source for OpenClarion. Each
stage records its authority documents, success path, failure boundary, and
persistence artifact. The MVP implements the headless report path (S0-S4).
Short-conversation interactive diagnosis (S6) is V1 required at minimum-viable
scope.

## Stage S0: Alert State Ingestion

| Aspect | Value |
|--------|-------|
| Authority | [phases/02-providers.md](../phases/02-providers.md), MetricsProvider |
| Owner | Go control plane |
| Trigger | poll cycle or webhook from Alertmanager |
| Success | active alerts pulled and normalized into AlertEvent records |
| Failure | provider error -> backoff and retry; partial pull marks the cycle as `incomplete` for replay |
| Persistence | `AlertEvent` rows in PostgreSQL |
| Invariant | OpenClarion does not own realtime paging or on-call routing |

## Stage S1: Deterministic Grouping

| Aspect | Value |
|--------|-------|
| Authority | [phases/01-contracts.md](../phases/01-contracts.md), grouping algorithm |
| Owner | Go control plane |
| Trigger | new AlertEvent batch from S0 |
| Success | events grouped by configured dimensions; replayable record persisted |
| Failure | grouping conflict -> deterministic resolution by configured priority; never silently drop |
| Persistence | `AlertGroup` rows; replay log |
| Invariant | grouping is replay-deterministic given the same input window |

## Stage S2: Evidence Snapshot

| Aspect | Value |
|--------|-------|
| Authority | [database/schema-catalog.md](../database/schema-catalog.md), `EvidenceSnapshot` |
| Owner | Go control plane + CMDBProvider + MetricsProvider |
| Trigger | AlertGroup created in S1 |
| Success | enriched snapshot persisted with full provenance |
| Failure | provider partial failure -> snapshot marked `partial` with explicit `missing_fields`; downstream stages still proceed but flag report quality |
| Persistence | `EvidenceSnapshot` row |
| Invariant | snapshot is the single source for all downstream AI analysis |

## Stage S3: Headless Report Fan-Out

| Aspect | Value |
|--------|-------|
| Authority | [phases/03-workflows.md](../phases/03-workflows.md), `ReportFanOutWorkflow` |
| Owner | Temporal workflow + LLMProvider |
| Trigger | EvidenceSnapshot ready in S2 |
| Success | one `SubReport` per logical group, each schema-validated |
| Failure | individual SubReport failure does not block siblings; fan-in tolerates partial success up to a configured threshold; failed SubReport is retryable |
| Persistence | `SubReport` rows linked to snapshot |
| Invariant | LLM output is validated before persistence; refusal and truncation are explicit error states |

## Stage S4: Final Report Reduce

| Aspect | Value |
|--------|-------|
| Authority | [phases/03-workflows.md](../phases/03-workflows.md), `FinalReportWorkflow` |
| Owner | Temporal workflow + IMProvider |
| Trigger | SubReport set complete in S3 (or threshold met) |
| Success | `FinalReport` persisted; notification delivered through IMProvider |
| Failure | notification delivery failure -> retry with exponential backoff; persistence is independent of delivery so report remains queryable |
| Persistence | `FinalReport` row; notification delivery log |
| Invariant | report and notification persistence are decoupled |

## Stage S5: Optional Agent Sandbox Analysis (M4)

| Aspect | Value |
|--------|-------|
| Authority | [phases/04-ai-integration.md](../phases/04-ai-integration.md), [adr/ADR-0005](../../adr/ADR-0005-ephemeral-container-security.md) |
| Owner | Go control plane + ContainerProvider |
| Trigger | snapshot meets configured criteria for tool-augmented analysis |
| Success | sandbox returns structured JSON; enhanced report attached to FinalReport |
| Failure | sandbox timeout or non-zero exit -> mark attempt failed, fall back to S3-only report; container always cleaned up |
| Persistence | `SandboxRun` row; output JSON archived |
| Invariant | sandbox interior is runtime-agnostic; control plane owns lifecycle |

## Stage S6: Short-Conversation Interactive Diagnosis (M5, V1 Required)

| Aspect | Value |
|--------|-------|
| Authority | [phases/05-interactive-diagnosis.md](../phases/05-interactive-diagnosis.md) |
| Owner | Go control plane + AuthProvider + ContainerProvider + Temporal `DiagnosisRoomWorkflow` |
| Trigger | authorized user opens diagnosis room from a FinalReport |
| Success | bounded-turn conversation completes; chat persisted; final notification sent on close |
| Failure | turn limit reached -> graceful close with notification; idle timeout -> graceful close; sandbox failure -> session terminated with explicit error turn; auth failure -> handshake refused |
| Persistence | `ChatSession` and `ChatTurn` rows; audit log |
| Invariant | turn count and session lifetime are enforced at workflow level; no client-side bypass |

## Closure Outcomes

| Outcome | Meaning |
|---------|---------|
| `true_positive` | real issue confirmed |
| `false_positive` | alert did not represent a real issue |
| `expected_maintenance` | expected change or planned maintenance |
| `self_resolved` | recovered before human action |
| `unresolved_escalated` | still firing and escalated |

## System Invariants

- AI does not route alerts.
- AI does not own workflow lifecycle.
- AI does not execute production-changing actions.
- Go verifies external state through providers.
- Reports remain traceable to evidence snapshots.
- Every stage failure is recorded; no silent drops.
- M0-M2 acceptance does not depend on any specific agent runtime.
- M5 short-conversation scope is the V1 ceiling; long-session features are
  out of scope (see ADR-0006 V1 Commitment Boundary).

## Authority Links

- Milestone strategy: [adr/ADR-0006](../../adr/ADR-0006-feasibility-and-mvp-cutline.md)
- Workflow engine: [adr/ADR-0004](../../adr/ADR-0004-temporal-workflow-engine.md)
- Provider boundaries: [adr/ADR-0003](../../adr/ADR-0003-provider-extension-interfaces.md)
- AI black-box principle: [adr/ADR-0002](../../adr/ADR-0002-ai-agent-black-box.md)
- Sandbox security: [adr/ADR-0005](../../adr/ADR-0005-ephemeral-container-security.md)
- Technology validation: [adr/ADR-0012](../../adr/ADR-0012-technology-validation.md)
