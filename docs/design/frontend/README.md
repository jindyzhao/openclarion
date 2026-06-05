# Frontend Architecture

The frontend lives under `web/` in the monorepo. The first UI target was a
readonly report viewer; M5 adds the short-conversation diagnosis room.

## Layering

| Layer | Responsibility |
|-------|----------------|
| `app/**/page.tsx` | route shell only |
| `features/**` | workflow-specific UI and API composition |
| `components/**` | reusable presentational components |
| `stores/**` | session and local UI state |
| `src/lib/api/openapi.ts` | OpenAPI-derived contracts generated from `api/openapi.yaml` |

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
- Keep WebSocket logic isolated to diagnosis-room feature modules.
- Do not hardcode non-English UI literals outside i18n files.
- Do not hardcode customer Prometheus or Alertmanager endpoints in frontend
  code, tests, or examples.
- Do not store real bearer tokens or secret values in durable browser state.
- Report viewer remains the stable read-only operations slice; diagnosis-room
  WebSocket code stays isolated to `web/src/features/diagnosis-room/`.

## Current Slice

The current slice renders `/dashboard`, `/reports`, `/reports/[reportId]`,
`/diagnosis-room`, and `/settings/alert-sources`. Report pages use generated
OpenAPI TypeScript types for REST responses. The diagnosis room uses generated
OpenAPI types for ticket issuance and feature-local frame types for the
non-OpenAPI WebSocket protocol. The alert source settings route uses generated
OpenAPI types for profile reads and writes, and browser mutations go through
same-origin Next.js route handlers before reaching the Go API so backend
endpoints and secret values are not exposed to durable browser state. Route
pages remain thin wrappers around feature modules. Playwright route smoke tests
run these pages against a mocked OpenClarion API/WebSocket endpoint using the
production Next.js server. `make diagnosis-live-browser-smoke` is the manual M5
browser proof harness for a real backend/worker stack.

## Next Operations Slice

The remaining operations settings slices are grouping policies, report workflow
policies, notification channels, and impact-preview actions. These screens must
consume generated OpenAPI types and call backend configuration APIs; they must
not own durable configuration or workflow routing in local component state.
Connection tests, grouping previews, dry-runs, and enablement remain separate
backend actions so operators can review impact before a profile or workflow
becomes active.
