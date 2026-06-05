# Frontend Contracts

Frontend request and response types must be generated from `api/openapi.yaml`.
Feature modules may define view models, but not duplicate API DTOs.

Current generated output:

- `web/src/lib/api/openapi.ts`
- `make openapi-ts-fresh`
- `npm run api:generate` from `web/`

Current API-backed features:

- `/dashboard` uses `DashboardSummary`
- `/reports` uses `ReportListResponse`
- `/reports/[reportId]` uses `FinalReportDetail`
- `/diagnosis-room` uses generated diagnosis ticket and room-create contracts,
  plus feature-local WebSocket frame types

Upcoming operations settings features must add OpenAPI schemas before frontend
implementation. Frontend modules may define form draft view models, but saved
alert source, grouping, workflow, and notification data must come from
generated API contracts.
