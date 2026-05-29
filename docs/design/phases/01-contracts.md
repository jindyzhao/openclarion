# Phase 01: Contracts

## Goal

Define API, domain, provider, and persistence contracts before implementing
runtime behavior.

## API Contracts

- OpenAPI 3.1 is canonical.
- Generated Go and TypeScript types derive from `api/openapi.yaml`.
- Hand-written duplicate DTOs are not allowed when generated types exist.

## Domain Contracts

Initial domain objects:

- AlertEvent
- AlertWindow
- AlertGroup
- EvidenceSnapshot
- DiagnosisTask
- DiagnosisTaskEvent (append-only lifecycle log; one row per state transition;
  `dedupe_key` UNIQUE per task allows idempotent producers)
- SubReport
- FinalReport
- AuditLog

> Persistence ownership and current schema status live in
> [../database/schema-catalog.md](../database/schema-catalog.md).

## Provider Contracts

- MetricsProvider
- CMDBProvider
- IMProvider
- AuthProvider
- ApprovalProvider
- ContainerProvider
- LLMProvider

## Acceptance

- API schema lints
- provider interfaces compile
- fake providers exist for tests
- database schema catalog matches Ent plan
- `make ent-fresh` passes (Ent generation is stable after schema changes)
- `make atlas-drift` passes (no Atlas-suggested migration delta)
- `make atlas-smoke` passes once on host docker before the first migration
  is cut (manual acceptance gate; see
  [../database/migrations.md](../database/migrations.md))
