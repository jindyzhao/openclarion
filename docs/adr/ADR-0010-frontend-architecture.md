---
id: ADR-0010
title: "Frontend Architecture"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0010: Frontend Architecture

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion needed a report viewer first and an interactive diagnosis room
later. That slice has now expanded into alert source configuration and will
continue into grouping policies, report workflow policies, notification channel
profiles, connection tests, and dry-run previews.

The frontend must therefore move from a minimal report-viewer shell to a stable
operations console without compromising the backend-owned configuration model
from [ADR-0014](ADR-0014-alert-operations-configuration.md). The console can
reference the structure of KubeVirt Shepherd's `web/` project, but OpenClarion
must keep its alert-first product boundary and avoid copying unrelated VM,
approval, or service-management concepts.

## Decision Outcome

**Chosen option**: React and Next.js under `web/`, with route-shell pages,
feature modules, generated API types, a standardized console component layer,
TanStack Query for browser-side refresh and mutation invalidation, and isolated
WebSocket support for the diagnosis room.

The standardized console layer is:

* Next.js App Router and React as the rendering foundation.
* Generated OpenAPI TypeScript types as the API DTO source.
* Same-origin Next.js route handlers for browser mutations that should not expose
  deployment-specific backend addresses or secret material.
* Ant Design, with the React 19 compatibility patch, for shared console UI.
* TanStack Query for client-side cache, refresh, polling, and mutation
  invalidation in interactive settings surfaces.

Diagnosis-room ticket issuance also uses the same-origin route-handler boundary.
The browser may hold an operator bearer token only as transient form/action
state and sends it to the Next.js route, which forwards only the
`Authorization` header and generated-contract JSON body to the Go API. The
WebSocket upgrade remains a non-OpenAPI backend route; production deployments
should expose it on the same browser origin through ingress, while local and
manual smoke runs may provide an explicit browser WebSocket base URL. Any
explicit browser WebSocket base URL must be an HTTP(S) or WS(S) base URL without
userinfo, query string, or fragment state.

This is an architecture foundation decision, not a requirement to rewrite every
existing page in one change.

Interactive configuration screens should migrate to Ant Design `Form`,
feedback, table/list, and statistic components as they are touched. Feature
modules may keep local draft view models when they are useful for parsing or
validation, but the rendered controls should not keep parallel hand-built
component systems once the shared console layer is available.

The settings overview route is a console navigation surface, not a browser
wizard. It may derive setup progress from server-fetched profile and policy
counts, render Ant Design `Steps` status for the next missing configuration
object, split retained proof into policy replay and scheduled-trigger evidence
targets, and point operators at the retained proof gate once every required
object type exists. It must not persist browser-local setup state, infer
workflow readiness from counts alone, start workflows, call providers, resolve
secrets, persist proof state, or claim retained proof before the backend proof
harnesses run against real services.

## Frontend Layers

| Layer | Responsibility |
|-------|----------------|
| route shell | route params, first-load server fetches, feature composition |
| client providers | Ant Design theme/App context and TanStack Query client |
| feature module | workflow UI state, API composition, and feature-local view models |
| components | reusable presentational UI built on the console component layer |
| generated API types | OpenAPI-derived request and response contracts |
| same-origin route handlers | browser-safe mutation proxy and secret-boundary preservation |
| stores | session, auth, and local UI state only |

### Consequences

* Good, because report viewing and diagnosis-room workflows can share generated
  API contracts without duplicating DTOs.
* Good, because route shells stay thin while feature modules own workflow UI
  behavior and WebSocket state.
* Good, because a shared console component layer keeps settings workflows
  visually and behaviorally consistent as they grow beyond read-only reports.
* Good, because Ant Design form feedback and table primitives reduce custom
  accessibility, validation, and responsive-layout code in operator settings
  screens.
* Good, because TanStack Query gives browser-side settings screens a standard
  refresh and mutation-invalidation model without moving durable configuration
  into browser state.
* Neutral, because the frontend must wait for generated OpenAPI types before
  consuming new backend response shapes.
* Neutral, because existing pages should migrate incrementally rather than
  through a broad visual rewrite.
* Bad, because Next.js build and smoke gates add CI cost once `web/` lands.
* Bad, because Ant Design and TanStack Query add dependency surface that must
  stay covered by lockfile, audit, typecheck, lint, build, and smoke gates.

### Confirmation

* no hand-written duplicate DTOs when generated types exist
* route pages remain thin
* shared client providers own console UI context and query client setup
* configuration forms use the standardized console component layer when they are
  migrated
* browser mutations that cross deployment or secret boundaries use same-origin
  route handlers
* report viewer is delivered before interactive diagnosis room
* diagnosis-room ticket issuance uses a same-origin route handler, while
  WebSocket logic is isolated in diagnosis-room feature modules
* settings screens do not hardcode customer Prometheus or Alertmanager endpoints
  and do not store secret values in durable browser state

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-06-05 | jindyzhao | Added standardized operations console foundation with Ant Design, TanStack Query, same-origin mutation routes, and incremental migration policy |
| 2026-06-05 | jindyzhao | Clarified that touched interactive settings screens should migrate forms, feedback, tables, and statistics to the standardized Ant Design console layer |
| 2026-06-06 | jindyzhao | Aligned diagnosis-room ticket issuance with the same-origin BFF boundary and documented the separate WebSocket ingress/base URL responsibility |
| 2026-06-06 | jindyzhao | Documented the count-driven settings overview readiness display as a navigation projection rather than browser-owned setup state |
| 2026-06-06 | jindyzhao | Clarified that retained proof targets in the settings overview remain display-only policy replay and scheduled-trigger evidence projections |
