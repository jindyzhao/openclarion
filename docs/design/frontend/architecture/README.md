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
streaming token-level partial responses in V1. This work must not block M0-M3.
