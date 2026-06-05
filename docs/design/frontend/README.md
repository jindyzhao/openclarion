# Frontend Architecture

The frontend lives under `web/` in the monorepo. The first UI target was a
readonly report viewer; M5 adds the short-conversation diagnosis room. M3.1
turns the UI into a configurable alert operations console, so shared frontend
architecture now follows [ADR-0010](../../adr/ADR-0010-frontend-architecture.md).

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
`/diagnosis-room`, `/settings/alert-sources`, and
`/settings/grouping-policies`. Report pages use generated OpenAPI TypeScript
types for REST responses. The diagnosis room uses generated OpenAPI types for
ticket issuance and feature-local frame types for the non-OpenAPI WebSocket
protocol. The alert source settings route uses generated OpenAPI types for
profile reads, writes, and sanitized connection-test results. The grouping
policy settings route uses generated OpenAPI types for policy reads, writes, and
bounded dry-run preview results. Both settings routes render Ant Design forms,
summary metrics, feedback, tables, tags, and row actions while routing browser
mutations/actions through same-origin Next.js route handlers before reaching the
Go API. Route pages remain thin wrappers around feature modules. Playwright
route smoke tests run these pages against a mocked OpenClarion API/WebSocket
endpoint using the production Next.js server. `make diagnosis-live-browser-smoke`
is the manual M5 browser proof harness for a real backend/worker stack.

## Next Operations Slice

The remaining operations settings slices are report workflow policies,
notification channels, secret-backed/Alertmanager connection adapters, and
impact-preview actions. These screens must consume generated OpenAPI types and
call backend configuration APIs; they must not own durable configuration or
workflow routing in local component state. Connection tests, grouping previews,
dry-runs, and enablement remain separate backend actions so operators can review
impact before a profile or workflow becomes active.
