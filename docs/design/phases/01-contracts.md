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
- SubReport
- ResolutionReport
- AuditLog

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
