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

> Per-turn container invocation enforces these constraints. Authoritative
> contract: [ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

Agent runtimes (sandboxed containers) must not receive host privileges. The Go
control plane owns sandbox creation, prompt injection, output capture, timeout,
cleanup, session close handling, and final notification. Automatic conversation
compression is out of scope for V1 short-conversation diagnosis.

## Sandbox Credential and Privilege Rules

- **Short-lived tokens only**: if an agent needs API access (LLM, Prometheus),
  the control plane issues a short-lived credential whose TTL does not exceed
  the container timeout. Long-lived secrets must never be injected into a
  container.
- **Image digest pinning**: sandbox container images must be referenced by
  digest (`@sha256:...`), not mutable tag, in all non-dev configurations.
- **Docker daemon boundary**: V1 uses the host Docker socket. Post-V1 should
  migrate to rootless Docker or a dedicated sandbox host with mTLS-protected
  Docker API.
- **Writable surface**: only `/workspace/out/` (writable tmpfs, capped 10MB);
  all other mounts are read-only.
- **Egress control**: container network must restrict outbound traffic to an
  explicit allowlist. SaaS endpoints with rotating IPs require a domain-based
  egress proxy (Envoy/Squid), not IP-based iptables rules.

## WebSocket Authentication

Browser `new WebSocket(url)` cannot set custom HTTP headers. V1 uses a
ticket-based handshake:

1. Browser authenticates via `POST /api/ws-ticket` (OIDC Bearer in header).
2. Server issues a single-use ticket (UUID, TTL <= 30s).
3. Browser connects with `wss://host/ws/diagnosis?ticket=xxx`.
4. Server validates and consumes the ticket on upgrade.

Rules:
- Ticket is single-use: deleted after consumption.
- TTL must be <= 30s to minimize replay window.
- Long-lived JWTs must never appear in query strings (server logs, referrer
  headers, browser history exposure).

## Review Checklist

- [ ] no hardcoded secret
- [ ] no sensitive log field
- [ ] all inputs validated
- [ ] authorization is fail-closed
- [ ] provider errors do not leak internal details
- [ ] sandbox permissions are minimal (non-root, no-new-privileges, resource limits)
- [ ] sandbox credentials are short-lived (TTL <= container timeout)
- [ ] container image referenced by digest, not mutable tag
- [ ] egress control tested (not just documented)
- [ ] WS ticket is single-use and short-lived
- [ ] no long-lived JWT in query strings or URLs
- [ ] generated API contract remains current
