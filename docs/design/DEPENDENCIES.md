# Dependency Policy

## Baseline

| Area | Dependency |
|------|------------|
| Go | Go 1.25+ |
| API | OpenAPI 3.1, `oapi-codegen-exp` |
| Database | PostgreSQL 18, Ent, Atlas |
| Workflow | Temporal Go SDK |
| Frontend | Node.js 22+, React, Next.js |
| Observability | OpenTelemetry, Prometheus |

## Rules

- Core runtime dependencies must be justified by an ADR or design doc.
- Database, workflow, API, and frontend code generation versions are pinned.
- New infrastructure dependencies require an ADR.
- Redis, MongoDB, and external vector databases are not part of the MVP runtime.
- Security updates may bypass normal release batching but must include validation.
