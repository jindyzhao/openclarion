# Frontend Architecture Notes

## Report Viewer First

The first frontend milestone renders persisted `FinalReport` and
`EvidenceSnapshot` data. It does not require a live AI container.

## Console Foundation

The frontend now uses ADR-0010 as the console architecture decision. The
app-wide foundation is:

- Next.js App Router for route shells and first-load server fetches.
- Generated OpenAPI TypeScript contracts for API DTOs.
- Same-origin Next.js route handlers for browser mutations that must avoid
  exposing deployment-specific backend addresses or secret material.
- Ant Design as the standardized console component layer.
- TanStack Query for client-side refresh, polling, and mutation invalidation
  where interactive settings screens need browser-managed state.

KubeVirt Shepherd's frontend is a useful reference for structure: client
providers, generated API contracts, feature modules, and a standardized console
component layer. OpenClarion should reuse those architecture patterns only where
they support alert operations, not copy unrelated VM, approval, or
service-management concepts.

Feature migration is incremental. The shared provider and shell may land before
every feature screen is rewritten with Ant Design components. New settings
surfaces should use the standardized console layer from the start.

The alert source settings screen is the first migrated interactive settings
surface. It uses Ant Design statistics, alerts, forms, buttons, table columns,
tags, and empty states while keeping OpenAPI-generated DTOs, local parser tests,
and same-origin mutation route handlers as the data boundary.

The grouping policy settings screen follows the same architecture. The route is
owned by `/settings/grouping-policies`, first-load reads use generated
`GroupingPolicy` response contracts, and browser mutations go through
same-origin handlers under `/api/config/grouping-policies`. The frontend may
hold the last preview result for the active browser session, but persisted
policy rows and preview execution stay in the Go API. A preview is a backend
dry run over bounded persisted alert samples; it is not a Prometheus,
Alertmanager, or workflow call.

## Interactive Diagnosis in M5

The short-conversation diagnosis room is a V1-required M5 milestone. It requires:

- identity bootstrap
- RBAC checks
- WebSocket proxy
- sandbox lifecycle coordination
- audit logging

It does not include automatic conversation compression, long sessions, or
streaming token-level partial responses in V1.

The M5 route now lives at `/diagnosis-room`. It keeps ticket issuance and
WebSocket frame handling inside `web/src/features/diagnosis-room/`, while the
route page remains a thin App Router wrapper. The automated route smoke proves
the browser path against a mocked API/WebSocket endpoint. The manual `make
diagnosis-live-browser-smoke` gate uses `web/playwright.live.config.ts` for the
same browser path against a real backend/worker stack; captured live evidence
remains a separate M5 acceptance item.

For local manual runs, `make diagnosis-dev-oidc-issuer` can provide the OIDC
discovery, JWKS, and short-lived local operator ID token needed by the real
`cmd/openclarion` OIDC verifier. It is only an identity helper: M5 acceptance
still requires a real persisted `EvidenceSnapshot`, Temporal worker, sandbox
provider, and retained `diagnosis-live-browser-smoke` proof.

## Alert Source Settings

The alert source settings route lives at `/settings/alert-sources`. The App
Router page stays thin: it server-fetches `GET /api/v1/config/alert-sources`
through `web/src/features/settings/alert-sources/api.ts`, then renders the
feature-owned interactive view.

Browser mutations do not call the Go API directly. They call same-origin Next.js
route handlers under `/api/config/alert-sources`, and those route handlers use
the server-side API base URL to forward create/replace/test requests. This
keeps the browser away from deployment-specific backend addresses and keeps
credential material outside durable browser state; the UI accepts only
`secret_ref`, never a bearer token value.

Connection tests are backend actions, not frontend probes. The UI may trigger a
test for a persisted profile ID and render the sanitized status, reason, and
small counters returned by OpenAPI, but it must not expose the upstream base URL
in test output or infer workflow enablement from a successful test. Prometheus
no-auth tests can report live alert-listing reachability. Bearer-backed tests
remain blocked until the backend has a secret resolver, and Alertmanager remains
unsupported until its adapter is implemented.

The feature mirrors the backend's configuration model: generated OpenAPI types
are the DTO source, form parsing is local validation only, PostgreSQL-backed Go
configuration APIs remain the source of truth, and future grouping/workflow/
notification settings should reuse the same page -> feature -> same-origin
route -> Go API layering. The rendered controls now use the shared Ant Design
console layer; feature-local parsing remains in `format.ts` so API write-request
validation stays testable without coupling tests to component rendering.

## Grouping Policy Settings

The grouping policy settings route lives at `/settings/grouping-policies`.
Policy form controls capture display name, grouping dimension label keys,
severity label key, optional alert source filter, and enabled state. The screen
renders persisted policy rows from the backend and exposes a row-level preview
action.

Preview output is action state, not policy state. The UI can show the latest
preview status, number of events scanned, number of events matched, and grouped
samples returned by OpenAPI during the current session. It must not save preview
results as durable configuration, create workflow bindings, or infer that a
policy is production-ready only because preview returned groups.
