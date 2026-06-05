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
- `/settings/alert-sources` uses `AlertSourceProfile`,
  `AlertSourceProfileWriteRequest`, `AlertSourceProfileListResponse`, and the
  alert-source connection-test result contract
- `/settings/grouping-policies` uses `GroupingPolicy`,
  `GroupingPolicyWriteRequest`, `GroupingPolicyListResponse`, and grouping
  preview result contracts
- `/diagnosis-room` uses generated diagnosis ticket and room-create contracts,
  plus feature-local WebSocket frame types

Upcoming operations settings features must add OpenAPI schemas before frontend
implementation. Frontend modules may define form draft view models, but saved
alert source, grouping, workflow, and notification data must come from
generated API contracts.

Interactive settings features should use generated contracts at the API edge and
may use TanStack Query for browser-side refresh or mutation invalidation. Query
keys are view/cache identifiers only; they must not become durable workflow
configuration or replace backend-owned policy/profile identifiers.

Connection-test contracts are action results, not profile state. Frontend code
may cache the last result for display during the current browser session, but
must not persist it as durable configuration or use it as an enablement flag.

Grouping preview contracts follow the same action-result rule. A preview result
is computed by the backend from bounded persisted alert samples and can be shown
or refreshed by the browser, but it must not be stored by the frontend as
policy state or treated as a workflow enablement decision.
