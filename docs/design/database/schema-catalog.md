# Database Schema Catalog

PostgreSQL is the business source of truth. Ent schemas are the canonical
application schema definitions; Atlas migrations are the canonical migration
artifacts.

## Core Entities

| Entity | Purpose |
|--------|---------|
| `AlertEvent` | raw alert event, fingerprint, labels, status, timing, raw payload |
| `AlertWindow` | replayable polling window and active alert snapshot |
| `AlertGroup` | deterministic grouping result for report fan-out |
| `EvidenceSnapshot` | enriched evidence package sent to AI providers |
| `DiagnosisTask` | workflow-bound lifecycle record |
| `SubReport` | per-group AI report |
| `ResolutionReport` | final report and closure outcome |
| `ChatSession` | later interactive session lifecycle |
| `ChatTurn` | append-only human, assistant, system, and tool messages |
| `AuditLog` | security and lifecycle audit trail |

## JSONB Usage

Use JSONB for raw alert payloads, provider-specific evidence, tool results, and
model metadata. Extract commonly queried fields into typed columns.

## Retention

Raw evidence retention, report retention, and chat retention require explicit
operator configuration before public release.
