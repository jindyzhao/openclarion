# Frontend Contracts

Frontend request and response types must be generated from `api/openapi.yaml`.
Feature modules may define view models, but not duplicate API DTOs.
The frontend route and same-origin mutation boundary follows
[ADR-0010](../../../adr/ADR-0010-frontend-architecture.md).
Alert operations configuration contracts follow
[ADR-0014](../../../adr/ADR-0014-alert-operations-configuration.md).

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
- `/settings/report-workflow-policies` uses `ReportWorkflowPolicy`,
  `ReportWorkflowPolicyWriteRequest`, `ReportWorkflowPolicyListResponse`,
  `ReportWorkflowPolicyImpactPreviewResult`,
  `ReportWorkflowPolicyReplayRequest`, and `ReportReplayTriggerResponse`
- `/settings/notification-channels` uses `NotificationChannelProfile`,
  `NotificationChannelProfileWriteRequest`,
  `NotificationChannelProfileListResponse`, and the notification-channel test
  result contract
- `/diagnosis-room` uses generated diagnosis ticket and room-create contracts
  through a same-origin ticket route, plus feature-local WebSocket frame types

Upcoming operations settings features must add OpenAPI schemas before frontend
implementation. Frontend modules may define form draft view models, but saved
alert source, grouping, workflow, and notification data must come from
generated API contracts.

Interactive settings features should use generated contracts at the API edge and
may use TanStack Query for browser-side refresh or mutation invalidation. Query
keys are view/cache identifiers only; they must not become durable workflow
configuration or replace backend-owned policy/profile identifiers.
Shared settings query helpers may centralize `ApiResult` error handling,
first-load hydration, manual refresh, and list invalidation, but they must not
define saved profile/policy DTOs or infer provider/workflow behavior from
browser cache state.

Connection-test contracts are action results, not profile state. Frontend code
may cache the last result for display during the current browser session, but
must not persist it as durable configuration or use it as an enablement flag.
Bearer-backed connection tests depend on backend secret resolution; the
frontend continues to send only persisted profile IDs and renders only the
sanitized result contract.

Grouping preview contracts follow the same action-result rule. A preview result
is computed by the backend from bounded persisted alert samples and can be shown
or refreshed by the browser, but it must not be stored by the frontend as
policy state or treated as a workflow enablement decision.

Report workflow policy enablement contracts are action results, not workflow
starts. The frontend may call enable/disable for a persisted policy ID and
render the returned policy state, but form saves must not imply enablement. The
write contract includes an optional report notification channel profile ID;
`null` or omission means no channel binding, while a positive ID binds a
report-capable notification channel profile for backend validation.
Impact preview is a separate action contract: the frontend sends only a
persisted policy ID, renders readiness status, stable reason codes, recent
event/group counters, and bounded group samples, and must not treat a ready
preview as enablement or workflow start. The backend computes the preview from
persisted configuration and recent `AlertEvent` rows only.
Policy replay is a separate action contract: the frontend may send a bounded
window, limit, and optional idempotency identifiers for a persisted policy ID,
then render replay stats and workflow handles as session action output. The
browser must not override the stored report scenario or persist replay output
as policy state.

Notification channel profile contracts are durable configuration contracts.
The frontend may create or replace profile metadata with `secret_ref` values.
Notification channel test contracts are action results: the frontend sends only
a persisted channel ID, renders sanitized status/reason/message plus bounded
provider acknowledgement metadata, and must not persist the result as profile
state or treat it as report delivery enablement. Runtime delivery selection is
backend-owned and can only happen from report notification Activity code when a
workflow carries a bound channel profile ID.

Diagnosis-room ticket contracts are authentication bootstrap action contracts,
not durable browser state. The frontend sends `DiagnosisWSTicketRequest` to the
same-origin `/api/diagnosis/ws-ticket` route with a transient bearer token in
the request header; the route forwards the generated-contract body to
`POST /api/v1/diagnosis/ws-ticket` and returns the ticket plus the browser
WebSocket URL selected by deployment configuration. The browser must not expose
a free-form backend API base URL field for this path, persist bearer tokens, or
duplicate WebSocket frame DTOs into OpenAPI. WebSocket frames remain local
feature types because `/ws/diagnosis` is an upgrade route rather than an
OpenAPI JSON endpoint.
