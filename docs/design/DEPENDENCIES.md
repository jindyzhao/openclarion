# Dependency Policy

## Baseline

| Area | Dependency | Validated |
|------|------------|----------|
| Go | Go 1.25+ (toolchain pinned in `go.mod` at M0) | 2026-05-19 |
| HTTP | std `net/http` (Go 1.22+ enhanced routing) | 2026-05-19 |
| API | OpenAPI 3.1, `oapi-codegen-exp` V3 (pinned to commit hash at M0) | 2026-05-19 |
| Database | PostgreSQL 18, Ent (TBD pinned at M0), Atlas (TBD pinned at M0) | 2026-05-19 |
| Workflow | Temporal Go SDK (TBD pinned at M0) | 2026-05-19 |
| Frontend | Node.js 24.x LTS, React 19, Next.js 16 | 2026-05-19 |
| Observability | OpenTelemetry Go (TBD pinned at M0), Prometheus client (TBD pinned at M0) | 2026-05-19 |
| Future vector | pgvector 0.7+ (not MVP) | 2026-05-19 |

> Versions marked `TBD pinned at M0` must be replaced with concrete
> `module@version` entries before M0 acceptance. CI must reject the literal
> string `latest` in `go.mod` and `package.json` for first-party dependencies.

## Rules

- Core runtime dependencies must be justified by an ADR or design doc.
- Database, workflow, API, and frontend code generation versions are pinned.
- New infrastructure dependencies require an ADR.
- Redis, MongoDB, and external vector databases are not part of the MVP runtime.
- Security updates may bypass normal release batching but must include validation.
- No third-party HTTP framework (Gin, Echo, Fiber) unless a future ADR accepts one.

## Risk Assessment

| Dependency | Risk | Mitigation |
|------------|------|------------|
| `oapi-codegen-exp` V3 | experimental, pre-v1, API may change | pin to commit hash; CI validates generated output compiles; fallback path: downgrade to v2 with `openapi.compat.yaml` bridge |
| Temporal operational complexity | adds infrastructure dependency | use PostgreSQL persistence (shared cluster, separate database); defer Cassandra/Elasticsearch until scale demands |

## Downgrade Paths

- **oapi-codegen-exp to v2**: reintroduce `api/openapi.compat.yaml` (3.0 bridge)
  and switch generator to `oapi-codegen/v2`. Requires a superseding ADR for
  ADR-0007.
- **Node.js 24 to 22**: Node.js 22 remains in maintenance until April 2027.
  Downgrade is safe if a dependency requires it.
- **Agent sandbox runtime**: ContainerProvider interface accepts any
  OCI-compliant runtime. The sandbox interior (Python, Go, or any agent
  framework) is swappable without control-plane changes.
