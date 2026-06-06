# Frontend Architecture Notes

## Report Viewer First

The first frontend milestone renders persisted `FinalReport` and
`EvidenceSnapshot` data. It does not require a live AI container.

## Console Foundation

The frontend now uses
[ADR-0010](../../../adr/ADR-0010-frontend-architecture.md) as the console
architecture decision. The app-wide foundation is:

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

Interactive settings surfaces share a small query-state helper under
`web/src/features/settings/`. Route pages still perform first-load server
fetches, but browser refresh and create/replace/enable/disable mutations use
TanStack Query query keys and invalidation instead of per-page list copies.
Feature-specific action outputs, including connection-test, preview, and replay
results, remain local session state unless the backend exposes them as durable
configuration.

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

The report workflow policy settings screen follows the same architecture. The
route is owned by `/settings/report-workflow-policies`, first-load reads use
generated `ReportWorkflowPolicy` response contracts, and browser mutations go
through same-origin handlers under `/api/config/report-workflow-policies`.
Policy form saves create or replace disabled or previously enabled metadata
only, including an optional report notification channel profile binding;
explicit enable/disable row actions are the backend-owned state
transition. A separate row-level replay action calls the backend policy replay
endpoint with a bounded window and optional idempotency identifiers. A save or
enable action is not a Temporal workflow start, and the replay action does not
let the browser override the stored report scenario. This boundary follows
[ADR-0014](../../../adr/ADR-0014-alert-operations-configuration.md).

The notification channel settings screen follows the same architecture. The
route is owned by `/settings/notification-channels`, first-load reads use
generated `NotificationChannelProfile` response contracts, and browser
mutations go through same-origin handlers under
`/api/config/notification-channels`. The UI accepts only delivery metadata and
`secret_ref` values; it does not accept endpoint URLs or credential values, does
not resolve secrets. The row-level test action sends one controlled backend
test notification for a persisted channel ID and renders only sanitized status,
reason, message, and bounded provider acknowledgement metadata. Report workflow
policies can reference report-capable channels by ID. Runtime delivery
selection remains backend-owned and runs only from the report notification
Activity when the worker is configured with a notification channel secret
resolver, as defined by
[ADR-0014](../../../adr/ADR-0014-alert-operations-configuration.md).

The report workflow schedule settings screen follows the same architecture.
The route is owned by `/settings/report-workflow-schedules`, first-load reads
use generated `ReportWorkflowSchedule` response contracts, and browser
mutations go through same-origin handlers under
`/api/config/report-workflow-schedules`. Schedule form saves persist metadata
only: report workflow policy ID, Temporal Schedule ID, interval/offset, replay
window/delay, replay limit, and catch-up window. Explicit enable/disable row
actions are backend-owned state transitions, and the backend validates that the
bound report workflow policy is enabled before accepting schedule enablement.
The frontend must not use browser timers, local cron state, direct Temporal
calls, provider calls, secret resolution, or notification sending for scheduled
report triggers. Temporal Schedule registration, reconciliation, and
launcher-workflow execution remain backend work under
[ADR-0014](../../../adr/ADR-0014-alert-operations-configuration.md).

## Interactive Diagnosis in M5

The short-conversation diagnosis room is a V1-required M5 milestone. It requires:

- identity bootstrap
- RBAC checks
- WebSocket proxy
- sandbox lifecycle coordination
- audit logging

It does not include automatic conversation compression, long sessions, or
streaming token-level partial responses in V1.

The M5 route now lives at `/diagnosis-room`. The route page remains a thin App
Router wrapper over `web/src/features/diagnosis-room/`, the rendered controls
use the standardized Ant Design console layer, and ticket issuance goes through
a same-origin Next.js route handler at `/api/diagnosis/ws-ticket`. The browser
may hold an operator bearer token only in transient form/action state; it sends
that token to the same-origin route, and the route forwards only the
`Authorization` header plus generated-contract JSON body to the Go API. The
non-OpenAPI WebSocket frame handling stays local to the diagnosis-room feature.
Production deployments should route `/ws/diagnosis` through the same browser
origin; local and manual smoke runs can provide an explicit browser WebSocket
base URL. That explicit base URL must be an HTTP(S) or WS(S) base URL without
userinfo, query string, or fragment state. The automated route smoke proves the
browser path against a mocked API/WebSocket endpoint. The manual `make
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
and Alertmanager tests can report live alert-listing reachability through the
backend adapters. Bearer-backed tests require the backend secret resolver to
map the persisted `secret_ref` to a token; missing resolver entries remain a
blocked backend result and are not handled in the browser.

The feature mirrors the backend's configuration model: generated OpenAPI types
are the DTO source, form parsing is local validation only, PostgreSQL-backed Go
configuration APIs remain the source of truth, and future grouping/workflow/
notification settings should reuse the same page -> feature -> same-origin
route -> Go API layering plus the shared settings query-state helper. The
rendered controls now use the shared Ant Design console layer; feature-local
parsing remains in `format.ts` so API write-request validation stays testable
without coupling tests to component rendering.

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

## Report Workflow Policy Settings

The report workflow policy settings route lives at
`/settings/report-workflow-policies`. Policy form controls capture display
name, alert source profile ID, grouping policy ID, optional report notification
channel profile ID, trigger mode, report scenario, and diagnosis follow-up mode.
Enabled state is intentionally absent from the form.

Enablement is a backend action, not a form field. The UI can render the
persisted draft/enabled state and call row-level enable/disable actions for a
persisted policy ID. The backend validates that the bound alert source and
grouping policy are already enabled before accepting enablement. If a report
notification channel is bound, the backend also validates that it is enabled and
has the report delivery scope. The frontend must not call alert providers or
notification providers, or treat a saved draft as active workflow routing.

Impact preview is a backend action. The UI can call row-level impact preview
for a persisted policy ID and render readiness status, reason codes, recent
event/group counters, and bounded group samples as current-session action
state. The backend reads persisted configuration and recent `AlertEvent` rows
only; it does not call providers, resolve secrets, start Temporal workflows,
send notifications, or persist grouping/snapshot output. A ready impact preview
does not enable the policy.

Replay is also a backend action. The UI can call row-level replay for an enabled
policy ID with a replay window, limit, and optional correlation/workflow IDs.
The backend resolves the stored alert source, grouping policy, source filter,
report scenario, and server-side credentials before starting report generation.
The UI may show the returned replay stats and workflow handle as session action
state, but it must not store them as policy state or infer that future workflows
should start automatically.

## Notification Channel Settings

The notification channel settings route lives at
`/settings/notification-channels`. Profile form controls capture display name,
adapter kind, deployment-managed secret reference, delivery scopes, enabled
state, and labels.

The profile remains a frontend configuration contract only. The frontend may
save or replace a profile through generated OpenAPI write contracts, but it
must not collect delivery endpoint URLs, credential values, or
provider-specific secret material. It may request the backend to send one
controlled test notification for a persisted channel ID and render the sanitized
result during the current browser session, but it must not infer runtime report
delivery from an enabled profile or successful test alone.
Report workflow policies may reference a channel profile by ID; backend report
notification Activities can send through that channel only when the worker has
the notification channel secret resolver configured.
