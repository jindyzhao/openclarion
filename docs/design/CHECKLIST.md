# Delivery Checklist

## M0: Foundation

- [ ] license and governance files exist
- [ ] CI workflow exists
- [ ] ADR index is current
- [ ] documentation contains no non-English governed text
- [ ] Go module initialized
- [ ] Docker Compose starts PostgreSQL and Temporal
- [ ] oapi-codegen-exp generation chain works
- [ ] `make pr` passes
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
- [ ] fake LLM provider produces deterministic output for tests
- [ ] JSON report parser validates output against schema
- [ ] retry mechanism feeds validation errors back to LLM
- [ ] golden prompt tests pass (structure, not content)
- [ ] failed AI output is marked and retryable
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
- [ ] non-root sandbox with resource limits
- [ ] fixed timeout with deterministic cleanup
- [ ] network allowlist enforced
- [ ] evidence injected via mounted files
- [ ] output extracted from `/workspace/output.json`
- [ ] quality delta measured vs M2 direct LLM output

## M5: Short-Conversation Interactive Diagnosis (V1 Required)

- [ ] AuthProvider OIDC integration
- [ ] RBAC checks pass for owner and admin (leader deferred)
- [ ] DiagnosisRoomWorkflow uses Temporal signals, queries, and durable timers
- [ ] bounded-turn enforcement is config-driven and cannot be bypassed by client
- [ ] fixed session lifetime + idle timeout fire correctly
- [ ] WebSocket proxy authenticates handshake before bridging
- [ ] chat turns persist to database with full audit trail
- [ ] unsafe-instruction filter (deny-list) active
- [ ] audit events logged for session lifecycle
- [ ] final group notification sent on session close
- [ ] no automatic compression in V1 path
