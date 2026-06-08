# Frontend Testing

Frontend tests landed with the `web/` skeleton.
The route, component, same-origin handler, and browser-smoke split follows
[ADR-0010](../../../adr/ADR-0010-frontend-architecture.md).

Required layers:

- unit tests for feature reducers and formatters
- component tests for report viewer and diagnosis room widgets
- contract tests for generated API usage
- Playwright smoke tests for critical routes

Current gate coverage:

- `make frontend-checks`
- `make openapi-ts-fresh`
- `npm run typecheck`
- `npm run lint`
- `npm run test` (dashboard/report formatter tests and diagnosis-room
  transport tests)
- `npm run build`
- `npm run smoke` (Playwright route smoke against a mocked API/WebSocket
  endpoint)
- `npm run smoke:live` / `make diagnosis-live-browser-smoke` (manual
  Playwright browser smoke against a real backend/worker stack)
- `npm run deadcode`
- `npm run audit`

When a feature moves onto the standardized console layer, tests should cover:

- form/view-model parsing separately from Ant Design rendering
- mutation success and error handling around the same-origin route handler
  boundary
- TanStack Query invalidation behavior when a screen depends on client-managed
  refresh
- Playwright smoke coverage for the route-level operator workflow

The alert source, grouping policy, report workflow policy, and notification
channel settings screens are the reference pattern for this split:
feature-local `format.test.ts` files cover parser/view-model behavior, while
TypeScript/lint coverage checks the shared settings query-state helper and
Playwright covers the operator route workflow against the mocked API, including
row-level test, preview, replay, and mutation actions where the backend exposes
them.

The live diagnosis-room smoke is intentionally manual. It requires:

- `OPENCLARION_LIVE_API_BASE_URL`
- `OPENCLARION_LIVE_BEARER_TOKEN`
- either `OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID` for an existing room or
  `OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID` so the harness can create a room via
  `POST /api/v1/diagnosis/rooms`

The backend that serves `OPENCLARION_LIVE_API_BASE_URL` must also be configured
for the diagnosis room runtime path:

- `OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL`
- `OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID`
- `OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS` when the frontend and API are on
  different browser origins
- `OPENCLARION_SANDBOX_IMAGE_REF`
- `OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT`
- `OPENCLARION_IM_WEBHOOK_URL` when close notification delivery is part of the
  live proof

Optional variables:

- `OPENCLARION_LIVE_WEB_BASE_URL` to test an already-running frontend instead
  of starting local `next start`
- `OPENCLARION_LIVE_WEB_PORT` for the local production Next.js server
- `OPENCLARION_LIVE_BROWSER_WS_BASE_URL` when the browser WebSocket endpoint
  differs from `OPENCLARION_LIVE_API_BASE_URL`; local smoke defaults it to the
  API base URL, while production deployments should normally route
  `/ws/diagnosis` on the same browser origin. When set, this value must be an
  HTTP(S) or WS(S) base URL without userinfo, query string, or fragment state
- `OPENCLARION_LIVE_DIAGNOSIS_MESSAGE` for the submitted turn text
- `OPENCLARION_TEMPORAL_TASK_QUEUE` to isolate manual evidence runs from any
  worker that may already poll the default `openclarion` queue. Use the same
  value for the backend worker, `openclarion report-replay`, and
  `make diagnosis-live-browser-smoke` when a rehearsal stack uses a dedicated
  Temporal queue.
- `OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=1` to require the close path
  after the browser turn. This also requires local `DATABASE_URL` and
  `TEMPORAL_HOST_PORT` access for `openclarion diagnosis-room-close`, plus a
  worker configured with `OPENCLARION_IM_WEBHOOK_URL`.
- `DIAGNOSIS_LIVE_SMOKE_ASSUME_WORKER_READY=1` when the live worker is managed
  outside the current shell but is already configured for close-notification IM
  delivery.
- `DIAGNOSIS_LIVE_BROWSER_SMOKE_OUTPUT` for the JSON proof path

Run `make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=diagnosis-live-browser-smoke`
before the live smoke to check these local prerequisites without printing
tokens, URLs, session ids, or local paths.

The browser no longer renders an API base URL control for `/diagnosis-room`.
For a locally started production Next.js server, `web/playwright.live.config.ts`
injects `OPENCLARION_API_BASE_URL` and `OPENCLARION_BROWSER_WS_BASE_URL` into
the server process from the live-smoke environment. For an externally hosted
frontend, the deployment must already route the same-origin ticket route and
WebSocket upgrade to the intended backend.

The harness validates the proof with
`scripts/diagnosis_live_smoke_output` before reporting success. The checker
requires a passed flag, RFC3339 timestamp, HTTP(S) web/API URLs, non-empty
session id, non-zero submitted-message length, a lowercase SHA-256 digest for
the submitted message, a `turn_result` evidence claim, structured browser
observations for state load, `turn_result`, submitted message visibility,
submitted-message length and digest, connected status after the turn,
assistant-turn count increment, user+assistant transcript pair increment,
completed-turn log consistency with the assistant count, and transcript count
consistency with the pair model. When the harness created the room, it also
requires positive task/session/workflow identities that match the returned
session.
