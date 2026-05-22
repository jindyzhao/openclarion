---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0010: Frontend Architecture

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion will need a report viewer first and an interactive diagnosis room
later. Frontend architecture must support both without coupling early MVP work to
interactive agent complexity.

## Decision Outcome

**Chosen option**: React and Next.js under `web/`, with route-shell pages,
feature modules, generated API types, and later WebSocket support for the
diagnosis room.

## Frontend Layers

| Layer | Responsibility |
|-------|----------------|
| route shell | auth guard, route params, feature composition |
| feature module | workflow UI state and API calls |
| components | reusable presentational UI |
| generated API types | OpenAPI-derived request and response contracts |
| stores | session and view state only |

### Confirmation

* no hand-written duplicate DTOs when generated types exist
* route pages remain thin
* report viewer is delivered before interactive diagnosis room
* WebSocket logic is isolated in diagnosis-room feature modules

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
