# Delivery Checklist

## M0: Foundation

- [x] license and governance files exist
- [x] CI workflow exists
- [x] ADR index is current
- [x] documentation contains no non-English governed text
- [x] forbidden-method gates landed (imports / latest / oapi-v2 / sqlite-in-tests)
- [x] `make pr` entry point exists and runs all M0 gates
- [x] Go module initialized
- [x] Docker Compose starts PostgreSQL and Temporal (tag-pinned; digest only at M4)
- [x] oapi-codegen-exp generation chain works (`v0.1.0` pinned in `go.mod`)
- [x] `make pr` passes against committed Go module
- [x] health endpoint returns 200
- [x] `vacuum` OpenAPI lint runs as a real blocking gate (not soft skip)
- [ ] Ent / Atlas / Temporal SDK pinned (deferred to M1 per first-import rule)

## M1: Control Plane

### M1-PR1: Persistence Foundation

- [x] `entgo.io/ent v0.14.6` pinned in `go.mod` (direct require + Go 1.24+ `tool` directive)
- [x] Atlas CLI pinned via `arigaio/atlas:1.2.0` Docker image (`ATLAS_IMAGE` in `Makefile`); `latest` and rolling tags forbidden
- [x] `internal/persistence/ent/generate.go` wires `go tool entgo.io/ent/cmd/ent generate ./schema`
- [x] `AlertEvent` Ent schema landed (source / source_fingerprint / canonical_fingerprint / labels (JSONB+GIN) / annotations (JSONB) / raw_payload (JSONB) / status / starts_at / ends_at / created_at; UNIQUE (source, canonical_fingerprint, starts_at))
- [x] `make ent-generate` / `make ent-fresh` / `make atlas-migrate-diff` / `make atlas-drift` / `make atlas-smoke` targets exist
- [x] `make ent-fresh` and `make atlas-drift` are CI-blocking jobs (`ent-checks` and `atlas-drift` in `.github/workflows/ci.yml`); `atlas-drift` job uses `actions/setup-go`; `workflow-parity` passes
- [x] `docs/design/DEPENDENCIES.md` records concrete Ent / Atlas pins and the Atlas CLI Integration Policy
- [x] `docs/design/database/migrations.md` documents the toolchain, drift gate, smoke protocol, and the redesigned wrapper contract (with the empirical findings that drove the redesign)
- [x] **Atlas wrapper redesign**: original `--dev-url docker://...` + mounted Docker socket attempt is unusable (image lacks Docker CLI and Go runtime); redesigned wrapper (`scripts/lib_atlas.sh` plus three thin entry scripts) launches per-invocation `postgres:18-alpine` from host on a dedicated Docker network, mounts host Go toolchain read-only into Atlas container at `/usr/local/go`, runs as `$(id -u):$(id -g)`, uses plain `postgres://` dev-url
- [x] AlertEvent uses Ent default `bigserial` primary key (UUID is reserved for security-sensitive single-use tokens such as the WS ticket per `docs/design/SECURITY_CODING.md`; switching entity primary keys to UUID/ULID is deferred and is gated on a concrete need such as sharding or client-side ID generation)
- [x] `make atlas-smoke` passes on host docker (manual one-shot acceptance gate; runs the redesigned wrapper end-to-end; verified locally 2026-05-22, produced 2 files; cleanup clean -- no residual `.atlas-*` directories, `atlas-pg-*` containers, or `atlas-net-*` networks)
- [x] `AlertGroup`, `EvidenceSnapshot`, `DiagnosisTask`, `DiagnosisTaskEvent` Ent schemas land after smoke passes (landed 2026-05-22; AlertEvent <-M2N-> AlertGroup, AlertGroup -1:N-> EvidenceSnapshot, EvidenceSnapshot -1:N-> DiagnosisTask, DiagnosisTask -1:N-> DiagnosisTaskEvent; all bigserial PKs; FK columns surfaced as explicit `field.Int` so index column ordering is `(parent_id, secondary)` per docs intent)
- [x] First migration cut via `make atlas-migrate-diff NAME=initial_schema`; `atlas.sum` committed (5 entity tables + 1 M2N join table; `make atlas-drift` reports synced)

### M1-PR2 onward

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
