# Frontend Testing

Frontend tests landed with the `web/` skeleton.

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
- `OPENCLARION_LIVE_DIAGNOSIS_MESSAGE` for the submitted turn text
- `DIAGNOSIS_LIVE_BROWSER_SMOKE_OUTPUT` for the JSON proof path

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
