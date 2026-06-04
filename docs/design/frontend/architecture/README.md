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
