# Architecture Decision Records

Architecture Decision Records are immutable records of significant technical
decisions. Once accepted, they should not be edited in place. Use a new ADR to
amend or supersede an accepted decision.

## Status Definitions

| Status | Meaning |
|--------|---------|
| Proposed | under review |
| Accepted | active decision |
| Superseded | replaced by a newer ADR |
| Deprecated | no longer recommended |
| Rejected | considered but not accepted |

## ADR Index

| ID | Title | Status |
|----|-------|--------|
| [ADR-0001](ADR-0001-postgresql-single-source.md) | PostgreSQL as the Single Source of Truth | Proposed |
| [ADR-0002](ADR-0002-ai-agent-black-box.md) | AI Agent Black-Box Runtime Boundary | Proposed |
| [ADR-0003](ADR-0003-provider-extension-interfaces.md) | Provider Extension Interfaces | Proposed |
| [ADR-0004](ADR-0004-temporal-workflow-engine.md) | Temporal Workflow Engine | Proposed |
| [ADR-0005](ADR-0005-ephemeral-container-security.md) | Ephemeral AI Container Security Model | Proposed |
| [ADR-0006](ADR-0006-feasibility-and-mvp-cutline.md) | Feasibility and MVP Cutline | Proposed |
| [ADR-0007](ADR-0007-openapi-31-native-toolchain.md) | OpenAPI 3.1 Native Toolchain | Proposed |
| [ADR-0008](ADR-0008-monorepo-repository-structure.md) | Monorepo Repository Structure | Proposed |
| [ADR-0009](ADR-0009-go-control-plane-scheduling.md) | Go Control Plane Scheduling | Proposed |
| [ADR-0010](ADR-0010-frontend-architecture.md) | Frontend Architecture | Proposed |
| [ADR-0011](ADR-0011-ci-governance.md) | CI Governance and Quality Gates | Proposed |

## Reading Order

1. ADR-0006 - project cutline and MVP sequencing
2. ADR-0001 - persistence foundation
3. ADR-0003 - extension seams
4. ADR-0009 - Go control plane
5. ADR-0004 - workflow execution
6. ADR-0002 and ADR-0005 - AI runtime boundaries
7. ADR-0007, ADR-0008, ADR-0010, ADR-0011 - engineering governance

## Creating ADRs

1. Copy [TEMPLATE.md](TEMPLATE.md).
2. Keep the decision atomic.
3. Use Proposed status for review.
4. Update this index.
5. After acceptance, keep the ADR immutable.
