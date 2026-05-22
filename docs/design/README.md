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

| Priority | Scope | Timing |
|----------|-------|--------|
| P0 | Go control plane: alert reading, sharding, evidence, workflow dispatch | first |
| P1 | Headless LLM report loop | first |
| P2 | OpenClaw headless sandbox | proof-of-concept |
| P3 | interactive diagnosis room | later |

## Design Documents

| Document | Purpose |
|----------|---------|
| [interaction-flows/master-flow.md](interaction-flows/master-flow.md) | product interaction outcomes |
| [database/schema-catalog.md](database/schema-catalog.md) | data model authority |
| [phases/00-prerequisites.md](phases/00-prerequisites.md) | bootstrap plan |
| [phases/01-contracts.md](phases/01-contracts.md) | API and domain contracts |
| [phases/02-providers.md](phases/02-providers.md) | provider interfaces and defaults |
| [phases/03-workflows.md](phases/03-workflows.md) | Temporal workflow design |
| [phases/04-ai-integration.md](phases/04-ai-integration.md) | LLM and sandbox integration |
| [frontend/README.md](frontend/README.md) | frontend architecture |
| [ci/README.md](ci/README.md) | CI governance |

## Architecture Constraints

- PostgreSQL is the business source of truth.
- Temporal owns durable workflow orchestration.
- Go owns deterministic control-plane decisions.
- AI providers only analyze prepared evidence.
- OpenClaw runs only behind sandbox boundaries.
- Provider interfaces isolate external systems.
- OpenAPI 3.1 is the canonical API contract.
- Frontend types derive from OpenAPI artifacts.
