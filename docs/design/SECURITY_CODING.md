# Security Coding Guide

## Mandatory Rules

1. Never commit secrets.
2. Never log passwords, tokens, API keys, raw prompt secrets, or provider credentials.
3. Validate all user input at API boundaries.
4. Use Ent or parameterized SQL.
5. Check authorization before connecting a user to a diagnosis session.
6. Keep AI tools readonly by default.
7. Run agent sandboxes as non-root and short-lived.
8. Record audit events for lifecycle and approval decisions.

## AI Runtime Safety

OpenClaw and other agent runtimes must not receive host privileges. The Go
control plane owns sandbox creation, prompt injection, output capture, timeout,
cleanup, and lifecycle-end compression.

## Review Checklist

- [ ] no hardcoded secret
- [ ] no sensitive log field
- [ ] all inputs validated
- [ ] authorization is fail-closed
- [ ] provider errors do not leak internal details
- [ ] sandbox permissions are minimal
- [ ] generated API contract remains current
