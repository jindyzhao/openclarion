# Design Overview

OpenClarion is a governed alert-response control plane. It reads alert state,
builds evidence, runs durable workflows, generates AI-assisted reports, and
records outcomes for audit and review.

## System Layers

```text
API / Webhook / Scheduler
        |
        v
Application Services
        |
        +--> Provider Interfaces
        |       Metrics, CMDB, IM, Auth, Approval, Container, LLM
        |
        +--> Temporal Workflows
        |
        +--> PostgreSQL Repositories
```

## Early Delivery Cutline

| Milestone | Scope | Deliverable |
|-----------|-------|-------------|
| M0 | Bootstrap: Go skeleton, local infra, OpenAPI, codegen | runnable health endpoint + CI |
| M1 | Go control plane: alert reading, sharding, evidence, workflow dispatch | replayable alert-to-evidence pipeline |
| M2 | Headless LLM report loop: SubReport, FinalReport, notification | end-to-end evidence-to-report-to-notification |
| M3 | Report viewer frontend + operational observability | browsable reports with evidence traceability |
| M4 | Agent sandbox exploration (independent track) | enhanced reports from sandboxed agent |
| M5 | Short-conversation interactive diagnosis (V1 required) | bounded-turn diagnosis room with sandboxed agent, chat persistence |

## Design Documents

| Document | Purpose |
|----------|---------|
| [architecture.md](architecture.md) | layering contract and orchestrator ports |
| [CURRENT_STATE.md](CURRENT_STATE.md) | implementation status snapshot |
| [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md) | intentionally deferred items with re-evaluation triggers |
| [interaction-flows/master-flow.md](interaction-flows/master-flow.md) | product interaction outcomes |
| [database/schema-catalog.md](database/schema-catalog.md) | data model authority |
| [phases/00-prerequisites.md](phases/00-prerequisites.md) | bootstrap plan |
| [phases/01-contracts.md](phases/01-contracts.md) | API and domain contracts |
| [phases/02-providers.md](phases/02-providers.md) | provider interfaces and defaults |
| [phases/03-workflows.md](phases/03-workflows.md) | Temporal workflow design |
| [phases/04-ai-integration.md](phases/04-ai-integration.md) | LLM and sandbox integration |
| [phases/05-interactive-diagnosis.md](phases/05-interactive-diagnosis.md) | short-conversation interactive diagnosis (M5) |
| [frontend/README.md](frontend/README.md) | frontend architecture |
| [ci/README.md](ci/README.md) | CI governance |

## Architecture Constraints

- PostgreSQL is the business source of truth.
- Temporal owns durable workflow orchestration; the choice is justified by M5
  short-conversation diagnosis (signals, queries, durable timers).
- Go owns deterministic control-plane decisions.
- AI providers only analyze prepared evidence.
- Agent sandboxes run behind generic ContainerProvider interface.
- Provider interfaces isolate external systems and stay small (single
  capability per interface).
- Business-level orchestrators (e.g. ReportOrchestrator,
  DiagnosisRoomOrchestrator) own use-case shape; workflow engine remains an
  implementation detail.
- OpenAPI 3.1 is the canonical API contract.
- Frontend types derive from OpenAPI artifacts.
- No specific agent runtime is required for MVP (M0-M2).
- See [architecture.md](architecture.md) for the layering contract.
- See [CURRENT_STATE.md](CURRENT_STATE.md) and
  [DEFERRED_FOLLOWUPS.md](DEFERRED_FOLLOWUPS.md) for current status and
  follow-up tracking.
