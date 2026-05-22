# Delivery Checklist

## M0: Foundation

- [x] license and governance files exist
- [x] CI workflow exists
- [x] ADR index is current
- [x] documentation contains no non-English governed text
- [x] forbidden-method gates landed (imports / latest / oapi-v2 / sqlite-in-tests)
- [x] `make pr` entry point exists and runs all M0 gates
- [ ] Go module initialized
- [ ] Docker Compose starts PostgreSQL and Temporal
- [ ] oapi-codegen-exp generation chain works
- [ ] `make pr` passes against committed Go module
- [ ] health endpoint returns 200

## M1: Control Plane

- [ ] MetricsProvider interface and Prometheus implementation compile
- [ ] fake providers support workflow tests
- [ ] active alerts can be read
- [ ] alert windows can be replayed
- [ ] grouping is deterministic
- [ ] `EvidenceSnapshot` records are persisted
- [ ] Temporal workflow starts from a snapshot
- [ ] API: list alerts and list snapshots return data

## M2: Report Loop

- [ ] `LLMProvider` interface exists
- [ ] OpenAI-compatible provider calls `/chat/completions`
- [ ] LLMProvider detects provider capability (strict schema vs json_object fallback)
- [ ] fake LLM provider produces deterministic output for tests
- [ ] JSON report parser validates output against schema
- [ ] retry mechanism feeds validation errors back to LLM (up to 3 attempts)
- [ ] `finish_reason` / refusal / truncation checked before accepting output
- [ ] failed AI output is marked and retryable
- [ ] LLM Activity carries idempotency key (`snapshotID + groupIndex`)
- [ ] Webhook Activity carries idempotency key (prevents duplicate notifications)
- [ ] FinalReport persistence Activity succeeds **before** IMProvider notification starts
- [ ] golden prompt tests pass (structure, not content)
- [ ] IMProvider Webhook sends report notification
- [ ] end-to-end: alert batch -> report -> webhook notification

## M3: Frontend and Observability

- [ ] report list page renders generated API types
- [ ] report detail page links back to evidence snapshot
- [ ] route pages remain thin
- [ ] OpenTelemetry traces visible in collector
- [ ] Prometheus metrics endpoint exposes key counters

## M4: Agent Sandbox (Exploration)

- [ ] ContainerProvider interface exists
- [ ] non-root sandbox with resource limits and `no-new-privileges`
- [ ] fixed timeout with deterministic cleanup
- [ ] network egress control enforced (iptables or egress proxy; SaaS LLM uses domain proxy)
- [ ] egress control design tested before M4 acceptance (not just documented)
- [ ] evidence injected via mounted files (read-only bind mount)
- [ ] output extracted from `/workspace/out/output.json` (writable tmpfs, capped 10MB)
- [ ] container image referenced by digest (`@sha256:...`), not mutable tag
- [ ] short-lived API credentials injected (TTL <= container timeout); no long-lived secrets
- [ ] Docker daemon access boundary documented (V1: host socket; post-V1: rootless or dedicated)
- [ ] quality delta measured vs M2 direct LLM output

## M5: Short-Conversation Interactive Diagnosis (V1 Required)

> Implements the per-turn container invocation contract from
> [ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

- [ ] AuthProvider OIDC integration
- [ ] RBAC checks pass for owner and admin (leader deferred)
- [ ] WS ticket-based auth: single-use, TTL<=30s, consumed on upgrade, no JWT in query string
- [ ] DiagnosisRoomWorkflow uses Temporal Update (primary), Signals (close/cancel), Queries, durable timers
- [ ] Temporal Update timeout: WS handler uses context timeout (~3min); informs user on exceed
- [ ] Concurrent-turn rejection: Update Validator rejects if turn already in flight
- [ ] Turn idempotency: each turn carries unique `message_id`; duplicates rejected by Validator
- [ ] WS disconnect handling: workflow completes turn regardless; reconnect restores via Query
- [ ] bounded-turn enforcement is config-driven and cannot be bypassed by client
- [ ] context byte/token budget enforced before mounting into container; truncate or reject on exceed
- [ ] fixed session lifetime + idle timeout fire correctly
- [ ] WebSocket handshake requires valid ticket before upgrade
- [ ] chat turns persist to database with full audit trail
- [ ] unsafe-instruction filter (deny-list) active
- [ ] audit events logged for session lifecycle
- [ ] final group notification sent on session close
- [ ] no automatic compression in V1 path
