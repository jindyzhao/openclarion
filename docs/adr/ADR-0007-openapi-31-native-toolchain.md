---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0007: OpenAPI 3.1 Native Toolchain

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion has no legacy API generator burden. The project needs one canonical
API contract for backend handlers, runtime validation, and frontend types.

## Decision Drivers

* single API source of truth
* native JSON Schema semantics
* generated Go and TypeScript artifacts
* generated-code freshness checks
* no compatibility artifact unless a later ADR accepts one

## Decision Outcome

**Chosen option**: OpenAPI 3.1 canonical specification with
`github.com/oapi-codegen/oapi-codegen-exp/cmd/oapi-codegen` for Go generation.
The generated HTTP server target is std `net/http` (not Gin, Echo, or Chi).

### Normative Decisions

1. `api/openapi.yaml` is the only canonical API contract.
2. The spec must use `openapi: 3.1.0`.
3. Go code generation must not use `oapi-codegen/v2`.
4. `api/openapi.compat.yaml` must not exist unless a superseding ADR accepts a bridge.
5. Frontend generated types must derive from the canonical spec.
6. Generated server code must use std `net/http` with `ServerInterface`.
7. The `oapi-codegen-exp` version must be pinned to a specific commit hash in `go.mod`.

### Risk: oapi-codegen-exp is Pre-v1

`oapi-codegen-exp` (V3) is an experimental rewrite based on `libopenapi`. It is
not yet v1.0 stable. This is the only Go code generator with native OpenAPI 3.1
support without a compatibility layer.

**Mitigation**:

* Pin to a tested commit hash; do not use `@latest` in CI.
* CI must compile generated code on every PR to detect upstream breakage early.
* Maintain awareness of the `oapi-codegen/v2` + `openapi.compat.yaml` fallback
  path documented in DEPENDENCIES.md.
* If V3 stabilizes and is released as `v3.x.x`, migrate the import path.

**Fallback path**: if `oapi-codegen-exp` becomes unmaintained or introduces
breaking changes that cannot be pinned around, reintroduce a 3.0 compatibility
spec and switch to `oapi-codegen/v2`. This requires a superseding ADR.

### Confirmation

* `make generate` compiles generated Go code
* generated frontend types are fresh
* CI fails on stale generated artifacts
* CI fails if `oapi-codegen/v2` or `openapi.compat.yaml` is introduced
* generated server uses std `net/http`, not a third-party framework

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
