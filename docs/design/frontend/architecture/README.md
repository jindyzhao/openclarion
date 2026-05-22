# Frontend Architecture Notes

## Report Viewer First

The first frontend milestone renders persisted `FinalReport` and
`EvidenceSnapshot` data. It does not require a live AI container.

## Interactive Diagnosis Later

The diagnosis room requires:

- identity bootstrap
- RBAC checks
- WebSocket proxy
- sandbox lifecycle coordination
- audit logging
- lifecycle-end compression

This work must not block P0/P1.
