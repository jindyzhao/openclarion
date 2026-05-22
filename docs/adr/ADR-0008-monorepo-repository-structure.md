---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0008: Monorepo Repository Structure

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion needs backend, frontend, API contracts, Ent schemas, migrations,
workflow code, and documentation to evolve atomically. Splitting repositories too
early would create contract drift.

## Decision Outcome

**Chosen option**: use a monorepo with `web/` as the frontend root.

## Repository Layout

```text
api/          OpenAPI contracts
cmd/          entrypoints
internal/     private runtime code
pkg/          public extension interfaces
ent/          Ent schemas and generated code
migrations/   Atlas migrations
web/          frontend application
docs/         documentation and ADRs
scripts/      repository checks
```

### Confirmation

* API changes can update Go and TypeScript generated artifacts in one PR.
* CI validates backend, frontend, API, and docs in one workflow set.
* root README documents the layout.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
