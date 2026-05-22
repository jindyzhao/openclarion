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

### Normative Decisions

1. `api/openapi.yaml` is the only canonical API contract.
2. The spec must use `openapi: 3.1.0`.
3. Go code generation must not use `oapi-codegen/v2`.
4. `api/openapi.compat.yaml` must not exist unless a superseding ADR accepts a bridge.
5. Frontend generated types must derive from the canonical spec.

### Confirmation

* `make generate` compiles generated Go code
* generated frontend types are fresh
* CI fails on stale generated artifacts
* CI fails if `oapi-codegen/v2` or `openapi.compat.yaml` is introduced

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
