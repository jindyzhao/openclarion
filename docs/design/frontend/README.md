# Frontend Architecture

The frontend will live under `web/` in the monorepo. The first UI target is a
readonly report viewer. The short-conversation diagnosis room ships later in V1
as M5.

## Layering

| Layer | Responsibility |
|-------|----------------|
| `app/**/page.tsx` | route shell only |
| `features/**` | workflow-specific UI and API composition |
| `components/**` | reusable presentational components |
| `stores/**` | session and local UI state |
| generated types | OpenAPI-derived contracts |

## Rules

- Use generated API types.
- Do not duplicate DTOs by hand.
- Keep route pages thin.
- Keep WebSocket logic isolated to diagnosis-room feature modules.
- Do not hardcode non-English UI literals outside i18n files.
- Report viewer ships before the interactive diagnosis room.
