# Frontend Architecture

The frontend lives under `web/` in the monorepo. The first UI target was a
readonly report viewer; M5 adds the short-conversation diagnosis room. M3.1
turns the UI into a configurable alert operations console, so shared frontend
architecture now follows [ADR-0010](../../adr/ADR-0010-frontend-architecture.md).
The alert operations configuration contract follows
[ADR-0014](../../adr/ADR-0014-alert-operations-configuration.md).
The operator-facing configuration and proof sequence is documented in
[alert-operations-live-proof-runbook.md](../alert-operations-live-proof-runbook.md).

## Layering

| Layer | Responsibility |
|-------|----------------|
| `app/**/page.tsx` | route shell, route params, and first-load server fetches |
| `app/providers.tsx` | client-only Ant Design and TanStack Query providers |
| `features/**` | workflow-specific UI and API composition |
| `components/**` | reusable presentational components built on the console component layer |
| `stores/**` | session and local UI state |
| `src/lib/api/openapi.ts` | OpenAPI-derived contracts generated from `api/openapi.yaml` |
| `app/api/**/route.ts` | same-origin browser-safe mutation proxies |

## Subdocuments

| Document | Purpose |
|----------|---------|
| [architecture/README.md](architecture/README.md) | frontend route and feature architecture notes |
| [contracts/README.md](contracts/README.md) | generated API contract policy |
| [testing/README.md](testing/README.md) | frontend test and smoke coverage |

## Rules

- Use generated API types.
- Do not duplicate DTOs by hand.
- Keep route pages thin.
- Keep shared UI state in client providers and feature-local components, not in
  route pages.
- Use Ant Design as the standardized console component layer once a screen moves
  beyond static report viewing.
- Use Ant Design `Form`, feedback, table/list, and statistic primitives for
  touched interactive configuration screens instead of extending hand-built
  controls.
- Use TanStack Query for browser-side refresh, polling, and mutation
  invalidation when interactive settings screens need client-managed updates.
- Keep WebSocket logic isolated to diagnosis-room feature modules.
- Do not hardcode non-English UI literals outside i18n files.
- Do not hardcode customer Prometheus or Alertmanager endpoints in frontend
  code, tests, or examples.
- Do not store real bearer tokens or secret values in durable browser state.
- Report viewer remains the stable read-only operations slice; diagnosis-room
  WebSocket code stays isolated to `web/src/features/diagnosis-room/`.

## Current Slice

The current slice renders `/dashboard`, `/reports`, `/reports/[reportId]`,
`/diagnosis-room`, `/settings/alert-sources`, `/settings/grouping-policies`,
`/settings/report-workflow-policies`, `/settings/report-workflow-schedules`,
and `/settings/notification-channels`.
Report pages use generated OpenAPI TypeScript types for REST responses. The
diagnosis room uses generated OpenAPI types for ticket issuance and
feature-local frame types for the non-OpenAPI WebSocket protocol. The alert
source settings route uses generated OpenAPI types for profile reads, writes,
and sanitized connection-test results. The grouping policy settings route uses
generated OpenAPI types for policy reads, writes, and bounded dry-run preview
results. The report workflow policy settings route uses generated OpenAPI types
for policy reads, writes, explicit enable/disable actions, report workflow
impact previews, and explicit policy-driven replay actions, including the
optional report notification channel profile binding field. The report
workflow schedule settings route uses generated OpenAPI types for schedule
reads, writes, and explicit enable/disable actions while keeping Temporal
registration, reconciliation, and launcher-workflow execution backend-owned.
The notification channel settings route uses
generated OpenAPI types for profile reads, writes, and sanitized channel-test
results while keeping delivery target secrets behind `secret_ref` values. These
settings routes render Ant Design forms, summary metrics, feedback, tables,
tags, and row actions while
routing browser mutations/actions through same-origin Next.js route handlers
before reaching the Go API. Their browser list state now uses a shared TanStack
Query helper for first-load hydration, manual refresh, mutation invalidation,
and consistent API error feedback; action outputs such as connection-test,
preview, impact-preview, and replay results remain feature-local session state.
Route pages remain thin wrappers around feature modules. Playwright route smoke
tests run these pages against a
mocked OpenClarion API/WebSocket endpoint using the production Next.js server.
`make diagnosis-live-browser-smoke` is the manual M5 browser proof harness for
a real backend/worker stack.

## Next Operations Slice

The remaining operations work is real-stack proof and scheduled trigger
integration proof, not a broader browser-owned configuration model. The
schedule settings page can edit persisted schedule metadata, while backend code
owns Temporal Schedule registration, reconciliation, and launcher-workflow
execution.
These screens must continue to consume generated OpenAPI types and call backend
configuration APIs; they must not own durable configuration or workflow routing
in local component state. Connection tests, grouping previews, impact previews,
policy replays, dry-runs, and enablement remain separate backend actions so
operators can review impact before a profile or workflow becomes active.
Report workflow policies can reference report-capable notification channel
profiles as configuration, and backend report notification Activities can use
that binding when the worker has the notification channel secret resolver
configured. The frontend still does not resolve secrets, construct providers,
or expose generic notification sending.
The live proof path should follow
[alert-operations-live-proof-runbook.md](../alert-operations-live-proof-runbook.md)
so operators configure alert sources, grouping rules, notification channels,
workflow policies, and schedules in the intended order before retaining proof.

### Console Configuration Graph

The operations console presents persisted server-owned objects rather than a
browser-owned wizard state. The durable graph is:

```text
alert source profile + grouping policy + notification channel profile
        |
        v
report workflow policy
        |
        v
report workflow schedule
```

The UI may guide operators through this graph, but it must preserve the action
split from ADR-0014. Form saves mutate metadata only. Row actions perform
connection tests, grouping previews, channel tests, impact previews, explicit
policy replay, enable/disable, or schedule synchronization through backend
APIs. A route component must not infer that a successful save, test, or preview
means a workflow should start.

### Alert Source Configuration

Operators configure Prometheus or Alertmanager through
`/settings/alert-sources`. The browser submits only profile metadata: source
kind, display name, base URL, auth mode, optional `secret_ref`, enabled state,
and labels. The backend owns persistence and connection tests. For bearer-backed
profiles, the browser never submits or stores bearer values; it only submits the
`secret_ref`, and backend wiring decides whether that reference can be resolved
for a bounded provider call.

Connection-test results remain sanitized UI state. They may show status, reason,
checked time, source kind, auth mode, and small alert counters, but they must not
echo upstream URLs, tokens, raw provider errors, or sampled alert payloads. A
successful test does not enable a profile by itself.

### Rule and Workflow Configuration

Operators configure grouping rules through `/settings/grouping-policies`.
Grouping previews run only against bounded persisted `AlertEvent` samples in
PostgreSQL. They do not call Prometheus or Alertmanager, persist `AlertGroup`
rows, or start workflows.

Operators configure report behavior through
`/settings/report-workflow-policies`. A report workflow policy binds an alert
source, grouping policy, report scenario, diagnosis follow-up mode, and optional
report notification channel. Form saves keep metadata disabled or at the
existing enablement state; explicit Enable and Disable actions are separate.
Impact preview is a review action over persisted data. Replay is a separate
operator action that sends only policy ID, replay window, limit, and optional
idempotency identifiers. The stored policy owns scenario selection.

Scheduled triggers use `/settings/report-workflow-schedules` for persisted
schedule metadata and explicit enablement. The frontend models only
server-owned state and does not implement browser timers, local cron state, or
direct Temporal calls. Saving, replacing, enabling, or disabling a schedule is
not a workflow start and must not call alert providers, resolve secrets, or
send notifications; backend code may synchronize Temporal Schedule metadata
after the persisted mutation succeeds.
