# Phase 00: Prerequisites

## Goal

Create the repository baseline: governance files, CI, Go module, local services,
OpenAPI contract, and code generation proof-of-concept.

## Deliverables

- license, governance, security, DCO, code of conduct
- Go module skeleton
- Docker Compose for PostgreSQL and Temporal
- OpenAPI 3.1 `api/openapi.yaml`
- Ent and Atlas toolchain
- `make generate`, `make test`, `make lint`, `make pr`
- documentation hygiene check

## Acceptance

- documentation language gate passes
- OpenAPI generator command resolves
- local PostgreSQL and Temporal can start
- health endpoint compiles when runtime skeleton lands
