# Master Flow

This document is the executable interaction truth source for OpenClarion. Each
stage records its authority documents, success path, failure boundary, and
persistence artifact. The MVP implements the headless report path (S0-S4).
Short-conversation interactive diagnosis (S6) is V1 required at minimum-viable
scope.

## Vocabulary Boundary

OpenClarion's MVP is intelligent alert analysis, so this flow intentionally
uses `AlertEvent` and `AlertGroup`. These names are also the current OpenAPI
and persistence contract names. Future architecture work may generalize the
same pattern to other business signals, but that extension must preserve the
current alert workflow and pass through a compatibility plan before any public
rename.

## Flow Families

The stages below belong to two logical subsystems:

- **Insight Pipeline**: S0-S4, plus optional S5 report augmentation. This is
  the automatic alert-to-evidence-to-report path.
- **Agent Workspace**: S6. This is the human-initiated diagnosis room for
  report follow-up, explanation, action drafting, and audit handoff.

The two families share evidence, reports, provider interfaces, auth/audit
primitives, OpenAPI, PostgreSQL, and Temporal infrastructure. They do not share
workflow lifecycle, output schemas, conversation state, or operation authority.
The detailed boundary lives in
[../insight-pipeline-agent-workspace.md](../insight-pipeline-agent-workspace.md).

## Stage S0: Alert State Ingestion

| Aspect | Value |
|--------|-------|
| Authority | [phases/02-providers.md](../phases/02-providers.md), MetricsProvider |
| Owner | Go control plane |
| Trigger | poll cycle or webhook from Alertmanager or another alert-source provider |
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
| Authority | [phases/03-workflows.md](../phases/03-workflows.md), `ReportBatchWorkflow` + `ReportFanOutWorkflow` |
| Owner | Temporal workflow + LLMProvider |
| Trigger | `POST /api/v1/report-triggers/replay-window` maps replayed EvidenceSnapshot refs into a `ReportBatchWorkflow` start request |
| Success | one `SubReport` per logical group, each schema-validated |
| Failure | individual SubReport failure does not block siblings; fan-in tolerates partial success up to a configured threshold; failed SubReport is retryable |
| Persistence | `SubReport` rows linked to snapshot |
| Invariant | LLM output is validated before persistence; refusal and truncation are explicit error states |

## Stage S4: Final Report Reduce

| Aspect | Value |
|--------|-------|
| Authority | [phases/03-workflows.md](../phases/03-workflows.md), `ReportBatchWorkflow` + `FinalReportWorkflow` |
| Owner | Temporal workflow + IMProvider |
| Trigger | SubReport set complete in S3 (or threshold met) |
| Success | `FinalReport` persisted; notification delivered through IMProvider |
| Failure | notification delivery failure -> retry with exponential backoff; persistence is independent of delivery so report remains queryable |
| Persistence | `FinalReport` row; notification delivery log |
| Invariant | report and notification persistence are decoupled |

## Stage S5: Candidate Agent Sandbox Analysis (M4 Exploration)

| Aspect | Value |
|--------|-------|
| Authority | [phases/04-ai-integration.md](../phases/04-ai-integration.md), [adr/ADR-0005](../../adr/ADR-0005-ephemeral-container-security.md) |
| Owner | Go control plane + ContainerProvider |
| Trigger | operator runs the manual M4 quality-evidence path against representative persisted snapshots |
| Success | candidate sandbox output passes the production SubReport parser and enters retained direct-vs-sandbox quality review |
| Failure | sandbox timeout, invalid output, or non-zero exit rejects that candidate attempt; the production S3/S4 path remains unchanged and the container is cleaned up |
| Persistence | candidate SubReport rows and explicitly retained manual evidence artifacts; no production `SandboxRun` model exists |
| Invariant | S5 is not part of the production report path until representative quality evidence and a recorded M4 proceed decision justify integration |

## Stage S6: Short-Conversation Interactive Diagnosis (M5, V1 Required)

| Aspect | Value |
|--------|-------|
| Authority | [phases/05-interactive-diagnosis.md](../phases/05-interactive-diagnosis.md) |
| Owner | Go control plane + AuthProvider + ContainerProvider + Temporal `DiagnosisRoomWorkflow` |
| Trigger | authorized operator creates or opens a room for an `EvidenceSnapshot`, directly or from alert/report review |
| Success | bounded turns and evidence follow-ups persist; human-confirmed close satisfies the configured conclusion quorum; terminal close retains a source-bound summary and, when configured, a final notification outcome |
| Failure | the turn cap rejects additional turns; idle/session timeout closes gracefully; an initial-turn failure closes the task as failed, while a later turn failure retains a sanitized room error without accepting a partial turn; auth failure refuses the request or handshake |
| Persistence | `ChatSession`, `ChatTurn`, `ChatSessionApproval`, and `ChatSessionSummary` rows plus `DiagnosisTaskEvent` lifecycle and configured-notification audit |
| Invariant | tenant identity, room authority, turn count, and session lifetime are enforced by backend/workflow boundaries; no client-side bypass |

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
