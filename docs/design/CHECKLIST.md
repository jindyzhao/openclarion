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
- [x] Ent / Atlas / Temporal SDK pinned (deferred to M1 per first-import rule)

## M1: Control Plane

### M1-PR1: Persistence Foundation

- [x] `entgo.io/ent v0.14.6` pinned in `go.mod` (direct require + Go 1.24+ `tool` directive)
- [x] Atlas CLI pinned via `arigaio/atlas:1.2.0` Docker image (`ATLAS_IMAGE` in `Makefile`); `latest` and rolling tags forbidden
- [x] `internal/persistence/ent/generate.go` wires `go tool entgo.io/ent/cmd/ent generate ./schema`
- [x] `AlertEvent` Ent schema landed (source / source_fingerprint / canonical_fingerprint / labels (JSONB+GIN) /
  annotations (JSONB) / raw_payload (JSONB) / status / starts_at / ends_at / created_at; UNIQUE (source,
  canonical_fingerprint, starts_at))
- [x] `make ent-generate` / `make ent-fresh` / `make atlas-migrate-diff` / `make atlas-drift` / `make atlas-smoke` targets exist
- [x] `make ent-fresh` and `make atlas-drift` are CI-blocking jobs (`ent-checks` and `atlas-drift` in
  `.github/workflows/ci.yml`); `atlas-drift` job uses `actions/setup-go`; `workflow-parity` passes
- [x] `docs/design/DEPENDENCIES.md` records concrete Ent / Atlas pins and the Atlas CLI Integration Policy
- [x] `docs/design/database/migrations.md` documents the toolchain, drift gate, smoke protocol, and the redesigned wrapper
  contract (with the empirical findings that drove the redesign)
- [x] **Atlas wrapper redesign**: original `--dev-url docker://...` + mounted Docker socket attempt is unusable (image lacks
  Docker CLI and Go runtime); redesigned wrapper (`scripts/lib_atlas.sh` plus three thin entry scripts) launches
  per-invocation `postgres:18-alpine` from host on a dedicated Docker network, mounts host Go toolchain read-only into
  Atlas container at `/usr/local/go`, runs as `$(id -u):$(id -g)`, uses plain `postgres://` dev-url
- [x] AlertEvent uses Ent default `bigserial` primary key (UUID is reserved for security-sensitive single-use tokens such as
  the WS ticket per `docs/design/SECURITY_CODING.md`; switching entity primary keys to UUID/ULID is deferred and is gated
  on a concrete need such as sharding or client-side ID generation)
- [x] `make atlas-smoke` passes on host docker (manual one-shot acceptance gate; runs the redesigned wrapper end-to-end;
  verified locally 2026-05-22, produced 2 files; cleanup clean -- no residual `.atlas-*` directories, `atlas-pg-*`
  containers, or `atlas-net-*` networks)
- [x] `AlertGroup`, `EvidenceSnapshot`, `DiagnosisTask`, `DiagnosisTaskEvent` Ent schemas land after smoke passes (landed
  2026-05-22; AlertEvent <-M2N-> AlertGroup, AlertGroup -1:N-> EvidenceSnapshot, EvidenceSnapshot -1:N-> DiagnosisTask,
  DiagnosisTask -1:N-> DiagnosisTaskEvent; all bigserial PKs; FK columns surfaced as explicit `field.Int` so index column
  ordering is `(parent_id, secondary)` per docs intent)
- [x] First migration cut via `make atlas-migrate-diff NAME=initial_schema`; `atlas.sum` committed (5 entity tables + 1 M2N
  join table; `make atlas-drift` reports synced)

### M1-PR2 onward

- [x] MetricsProvider interface and Prometheus implementation compile
- [x] fake providers support workflow tests
- [x] active alerts can be read
- [x] alert windows can be replayed
- [x] grouping is deterministic
- [x] `EvidenceSnapshot` records are persisted
- [x] Temporal workflow starts from a snapshot
- [x] API: list alerts and list snapshots return data

## M2: Report Loop

- [x] `LLMProvider` interface exists
- [x] OpenAI-compatible provider calls `/chat/completions`
- [x] LLMProvider detects provider capability (strict schema vs json_object fallback)
- [x] fake LLM provider produces deterministic output for tests
- [x] prompt request templates cover single alert, cascade, and alert storm
- [x] JSON report parser validates output against schema
- [x] SubReport and FinalReport draft JSON contracts exist before Ent persistence
- [x] SubReport and FinalReport Ent persistence schemas + repository exist
- [x] retry mechanism feeds validation errors back to LLM (up to 3 attempts)
- [x] `finish_reason` / refusal / truncation checked before accepting output
- [x] failed AI output is marked and retryable
- [x] LLM Activity carries idempotency key (`snapshotID + groupIndex`)
- [x] ReportBatchWorkflow fans out SubReport child workflows and fans in to FinalReportWorkflow
- [x] alert replay exposes persisted EvidenceSnapshot refs for report dispatch
- [x] report trigger usecase maps replay refs to an idempotent ReportBatchWorkflow start request
- [x] HTTP report trigger endpoint starts replay-window report generation when Prometheus wiring is configured
- [x] CLI one-shot report trigger starts replay-window report generation when Prometheus wiring is configured
- [x] production worker can inject OpenAI-compatible LLM and Webhook IM providers from env config
- [x] Webhook Activity carries idempotency key (prevents duplicate notifications)
- [x] FinalReport persistence Activity succeeds **before** IMProvider notification starts
- [x] notification delivery log persists pending/delivered/failed state independently of FinalReport
- [x] golden prompt tests pass (structure, not content)
- [x] IMProvider Webhook sends report notification
- [x] report read APIs expose FinalReport list/detail with linked SubReport evidence snapshot IDs
- [x] local end-to-end: HTTP replay trigger -> report workflow -> webhook notification -> persisted delivery log
- [x] manual live smoke gate exists for real Prometheus -> Temporal -> Webhook verification
- [x] manual live smoke proof validates checked timestamp, replay request
  metadata, replay stats, snapshot/SubReport consistency, notification
  idempotency key traceability, and successful notification status
  (`make report-live-smoke-output-test`)
- [ ] live external end-to-end: real Prometheus -> Temporal -> Webhook notification

## M3: Frontend and Observability

- [x] report list page renders generated API types
- [x] report detail page links back to evidence snapshot
- [x] dashboard page renders alert counters and report success rate from generated API types
- [x] Playwright route smoke covers dashboard, report list, and report detail with mocked API data
- [x] route pages remain thin
- [x] OpenTelemetry HTTP server instrumentation foundation exists
- [x] HTTP request correlation ID middleware and error-log attributes exist
- [x] HTTP access logs and outbound request-id propagation exist
- [x] OpenTelemetry traces visible in collector
- [x] outbound HTTP trace propagation exists
- [x] Temporal workflow trace propagation exists
- [x] Prometheus metrics endpoint exposes key counters

## M4: Agent Sandbox (Exploration)

- [x] ContainerProvider interface exists
- [x] Docker sandbox security spec validation gate exists
- [x] agent runtime selection gate exists before OpenClaw/Hermes/custom runner
  adoption
- [x] manual runtime adapter smoke harness exists (`make agent-runtime-smoke`)
- [x] Docker Provider live execution smoke exists (`make container-provider-smoke`)
- [x] Docker Provider timeout cleanup smoke exists (`make container-provider-timeout-smoke`)
- [x] Docker Provider output cap smoke exists (`make container-provider-output-cap-smoke`)
- [x] custom thin runner smoke proves one candidate contract/lifecycle
  runtime adapter against ADR-0013
- [x] metric query and topology lookup helper contracts are tested
  (`make agent-tool-scripts-test`)
- [x] custom thin runner image packages tool helpers and proves topology helper
  execution inside the digest-pinned image
- [x] custom thin runner smoke can retain canonical runtime-smoke artifacts
  during the same ephemeral registry run
- [x] code-level M4/M5 sandbox baseline audit exists
  (`make sandbox-baseline-audit`)
- [x] manual retained baseline audit target exists
  (`make sandbox-m4-baseline-audit`)
- [x] offline sandbox/direct SubReport comparison helper validates both outputs
  through production schema parsing (`make sandbox-quality-compare-test`)
- [x] manual persisted SubReport sample export target exists
  (`make sandbox-m4-quality-sample-export`)
- [x] manual quality manifest preparation from retained direct/sandbox pairs
  exists (`make sandbox-m4-quality-manifest-prepare`)
- [x] manual retained quality comparison target exists
  (`make sandbox-m4-quality-compare`)
- [x] M4 proceed/iterate/defer decision evidence gate exists
  (`make sandbox-m4-decision-test`; manual `make sandbox-m4-decision`)
- [x] M4 retained runtime-smoke artifact collection target exists
  (`make sandbox-m4-runtime-smoke-artifacts`)
- [x] fail-closed M4 review-evidence template helper exists
  (`make sandbox-m4-review-evidence-template`)
- [x] M4 decision evidence packet assembler exists
  (`make sandbox-m4-evidence-packet-test`; manual
  `make sandbox-m4-evidence-packet`)
- [x] non-root sandbox with resource limits and `no-new-privileges`
- [x] fixed timeout with deterministic cleanup
- [x] network egress control enforced through the Docker allowlist enforcer and
  egress-proxy smoke; SaaS LLM targets still require an operator-managed
  domain proxy
- [x] egress control design tested before M4 acceptance (not just documented)
- [x] allowlist egress targets are normalized and checked against a configured subset before create
- [x] evidence injected via mounted files (read-only bind mount)
- [x] output extracted from `/workspace/out/output.json` (only writable mount, capped 10MB)
- [x] container image referenced by digest (`@sha256:...`), not mutable tag
- [x] allowlist network mode fails closed unless an egress enforcer is injected
- [x] short-lived API credentials injected (TTL <= container timeout); no long-lived secrets
- [x] Docker daemon access boundary documented (V1: host socket; post-V1: rootless or dedicated)
- [x] M5 minimum sandbox baseline proved by code-level audit plus live smokes:
  file I/O, helper packaging, timeout cleanup, output cap, and egress allow/deny
- [ ] quality delta measured vs M2 direct LLM output
- [ ] M4 proceed/iterate/defer decision recorded from real baseline, quality,
  runtime-smoke, and human-review evidence

## M5: Short-Conversation Interactive Diagnosis (V1 Required)

> Implements the per-turn container invocation contract from
> [ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

- [x] pure diagnosis room policy boundary exists for turn/time/context/message
  limits, duplicate message IDs, in-flight rejection, and unsafe denylist
  (`make diagnosis-room-policy-test`)
- [x] AuthProvider/RBAC/WS ticket usecase boundary exists for owner/admin
  authorization and short-lived single-use tickets (`make diagnosis-auth-test`)
- [x] AuthProvider OIDC integration verifies signed ID tokens and maps role
  claims (`make diagnosis-auth-test`)
- [x] RBAC checks pass for owner and admin (leader deferred)
- [x] WS ticket-based auth usecase rule: single-use, TTL<=30s, consumed once,
  no long-lived JWT in ticket
- [x] WS ticket PostgreSQL store persists only token hash and enforces one
  concurrent consume winner (`make diagnosis-auth-test`)
- [x] WS ticket endpoint and upgrade handler consume the usecase ticket
- [x] ChatSession and ChatTurn Ent schemas plus repository boundary exist
  (`make diagnosis-chat-persistence-test`)
- [x] DiagnosisRoomWorkflow uses Temporal Update (primary), Signals
  (close/cancel), Queries, durable timers (`make diagnosis-room-workflow-test`)
- [x] per-turn `RunDiagnosisTurn` Activity calls `ContainerProvider.Run` and
  accepts only schema-valid diagnosis-turn output
  (`make diagnosis-room-workflow-test`)
- [x] DiagnosisRoomWorkflow creates/reuses ChatSession and persists accepted
  user+assistant ChatTurn pairs idempotently
  (`make diagnosis-room-workflow-test`)
- [x] Temporal Update timeout: WS relay uses a bounded context and informs the
  user when the turn is still processing (`make diagnosis-auth-test`)
- [x] Concurrent-turn rejection: Update Validator rejects if turn already in flight
  (`make diagnosis-room-workflow-test`)
- [x] Turn idempotency: each turn carries unique `message_id`; duplicates rejected by Validator
  (`make diagnosis-room-workflow-test`)
- [x] WS disconnect handling: submit-turn wait is decoupled from WebSocket
  disconnect and reconnect state is restored via Query (`make diagnosis-auth-test`)
- [x] bounded-turn enforcement is config-driven and cannot be bypassed by client
  at the workflow Update boundary (`make diagnosis-room-workflow-test`)
- [x] context byte budget enforced before mounting into container; V1 rejects on exceed
  (`make diagnosis-room-policy-test`, `make diagnosis-room-workflow-test`)
- [x] fixed session lifetime + idle timeout fire correctly
  (`make diagnosis-room-workflow-test`)
- [x] WebSocket handshake requires valid ticket before upgrade
- [x] WebSocket relay forwards authenticated `submit_turn` frames to Temporal
  Update and `query_state` frames to Temporal Query (`make diagnosis-auth-test`)
- [x] runtime wiring exists for OIDC auth, ticket store, WebSocket relay,
  room starter, browser origin policy, and Docker-backed per-turn sandbox provider
  (`go test ./cmd/openclarion`)
- [x] diagnosis room Next.js route renders the ticket/bootstrap/transcript UI
  and completes a mocked browser WebSocket exchange (`npm run smoke`)
- [x] manual live browser smoke harness exists for real backend/worker proof
  (`make diagnosis-live-browser-smoke`)
- [x] live smoke can create a real room from a frozen EvidenceSnapshot before
  connecting (`POST /api/v1/diagnosis/rooms`, `make diagnosis-live-browser-smoke`)
- [x] live smoke proof has an offline validator so retained evidence cannot be
  malformed or log-polluted (`make diagnosis-live-smoke-output-test`)
- [x] live smoke proof preserves structured browser observations for state load,
  `turn_result`, transcript growth, connected status, completed-turn number,
  and user+assistant pair consistency
  (`make diagnosis-live-smoke-output-test`)
- [x] live smoke proof binds the exercised request mode, session id, evidence
  snapshot id, message length, and submitted-message SHA-256 digest to the
  retained browser artifact without retaining message plaintext
  (`make diagnosis-live-smoke-output-test`)
- [x] chat turns persist to database from the workflow path
  (`make diagnosis-room-workflow-test`)
- [x] full audit trail recorded for session lifecycle events
  (`make diagnosis-room-workflow-test`)
- [x] unsafe-instruction filter (deny-list) active at the workflow/sandbox boundary
  (`make diagnosis-room-workflow-test`)
- [x] audit events logged for session lifecycle
  (`make diagnosis-room-workflow-test`)
- [x] final group notification sent on session close
  (`make diagnosis-room-workflow-test`)
- [ ] live browser acceptance evidence captured against a real backend/worker
  stack
- [x] no automatic compression in V1 path
