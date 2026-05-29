---
id: ADR-0008
title: "Monorepo Repository Structure"
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
agents/       agent workspace configs (NOT read by Go)
web/          frontend application
docs/         documentation and ADRs
scripts/      repository checks
```

### agents/ Directory

The `agents/` directory contains version-controlled agent workspace
configurations (role definitions, skills, prompts, tool endpoints). These are
mounted into sandbox containers as read-only bind mounts. Go never reads or
interprets their contents (see ADR-0002). Agent developers iterate on these
configs independently of Go code changes.

### Consequences

* Good, because API, backend, frontend, schema, migration, and documentation
  changes can land atomically.
* Good, because sandbox agent configuration remains versioned without becoming
  control-plane source code.
* Neutral, because CI must stay disciplined enough to keep unrelated package
  changes from making every PR expensive.
* Bad, because repository growth can make ownership boundaries less obvious
  unless docs and gates keep the layout current.

### Confirmation

* API changes can update Go and TypeScript generated artifacts in one PR.
* CI validates backend, frontend, API, and docs in one workflow set.
* root README documents the layout.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Add agents/ directory for agent workspace configs |
