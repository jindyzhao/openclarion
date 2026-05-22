---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0011: CI Governance and Quality Gates

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

The repository must preserve documentation quality, generated-code consistency,
security hygiene, and architecture boundaries from the beginning.

## Decision Outcome

**Chosen option**: define repository-owned `make` targets and CI checks for
backend, frontend, API, security, docs, and generated artifacts.

## Required Gate Families

| Gate | Purpose |
|------|---------|
| `make generate` | Ent, OpenAPI, and frontend type generation |
| `make lint` | Go, frontend, vulnerability, and secret scans |
| `make test` | unit and integration tests |
| `make pr` | local mirror of required PR checks |
| docs hygiene | English-only governed docs and valid markdown links |
| API contract | OpenAPI lint, generation freshness, diff checks |
| architecture checks | forbidden imports, provider layering, transaction boundaries |

### Confirmation

* `.github/workflows/ci.yml` calls repository-owned scripts or make targets
* documentation checks run on every pull request
* generated-code freshness is blocking once code generation exists

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
