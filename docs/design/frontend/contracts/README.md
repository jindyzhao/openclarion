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
