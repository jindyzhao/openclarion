# Frontend Architecture Notes

## Report Viewer First

The first frontend milestone renders persisted `FinalReport` and
`EvidenceSnapshot` data. It does not require a live AI container.

## Interactive Diagnosis in M5

The short-conversation diagnosis room is a V1-required M5 milestone. It requires:

- identity bootstrap
- RBAC checks
- WebSocket proxy
- sandbox lifecycle coordination
- audit logging

It does not include automatic conversation compression, long sessions, or
streaming token-level partial responses in V1.

The M5 route now lives at `/diagnosis-room`. It keeps ticket issuance and
WebSocket frame handling inside `web/src/features/diagnosis-room/`, while the
route page remains a thin App Router wrapper. The automated route smoke proves
the browser path against a mocked API/WebSocket endpoint. The manual `make
diagnosis-live-browser-smoke` gate uses `web/playwright.live.config.ts` for the
same browser path against a real backend/worker stack; captured live evidence
remains a separate M5 acceptance item.

For local manual runs, `make diagnosis-dev-oidc-issuer` can provide the OIDC
discovery, JWKS, and short-lived local operator ID token needed by the real
`cmd/openclarion` OIDC verifier. It is only an identity helper: M5 acceptance
still requires a real persisted `EvidenceSnapshot`, Temporal worker, sandbox
provider, and retained `diagnosis-live-browser-smoke` proof.

## Alert Source Settings

The alert source settings route lives at `/settings/alert-sources`. The App
Router page stays thin: it server-fetches `GET /api/v1/config/alert-sources`
through `web/src/features/settings/alert-sources/api.ts`, then renders the
feature-owned interactive view.

Browser mutations do not call the Go API directly. They call same-origin Next.js
route handlers under `/api/config/alert-sources`, and those route handlers use
the server-side API base URL to forward create/replace requests. This keeps the
browser away from deployment-specific backend addresses and keeps credential
material outside durable browser state; the UI accepts only `secret_ref`, never
a bearer token value.

The feature mirrors the backend's configuration model: generated OpenAPI types
are the DTO source, form parsing is local validation only, PostgreSQL-backed Go
configuration APIs remain the source of truth, and future grouping/workflow/
notification settings should reuse the same page -> feature -> same-origin
route -> Go API layering.
