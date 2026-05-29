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
- Report viewer remains the stable read-only operations slice; diagnosis-room
  WebSocket code stays isolated to `web/src/features/diagnosis-room/`.

## Current Slice

The current slice renders `/dashboard`, `/reports`, `/reports/[reportId]`, and
`/diagnosis-room`. Report pages use generated OpenAPI TypeScript types for
REST responses. The diagnosis room uses generated OpenAPI types for ticket
issuance and feature-local frame types for the non-OpenAPI WebSocket protocol.
Route pages remain thin wrappers around feature modules. Playwright route smoke
tests run these pages against a mocked OpenClarion API/WebSocket endpoint using
the production Next.js server. `make diagnosis-live-browser-smoke` is the
manual M5 browser proof harness for a real backend/worker stack.
