# Phase 05: Short-Conversation Interactive Diagnosis (M5)

> Realises the per-turn container invocation contract from
> [ADR-0013](../../adr/ADR-0013-per-turn-container-invocation.md). All
> per-turn lifecycle, isolation, and budget rules below derive from
> that ADR; conflicts must be resolved in favour of the ADR. Transient preview
> delivery is an optional provider capability under
> [ADR-0003](../../adr/ADR-0003-provider-extension-interfaces.md) and does not
> alter the authoritative `output.json` contract.

## Status

V1 required with post-V1 approval extensions. Long-session features remain
deferred.

## Goal

Implement a short-conversation diagnosis room where authorized users converse
with a sandboxed agent for a bounded number of turns within a fixed session
lifetime. The control plane owns identity, RBAC, audit, and lifecycle. The
sandbox owns the agent process.

M5 is the first Agent Workspace surface, not an extension of the headless
Insight Pipeline workflow. It may read `EvidenceSnapshot` and `FinalReport`
context, but its `ChatSession`, `ChatTurn`, output schema, timers, and
authorization boundary stay separate from report fan-out/fan-in workflows. See
[insight-pipeline-agent-workspace.md](../insight-pipeline-agent-workspace.md).

## V1 Scope Boundary

| In Scope (V1) | Deferred Beyond V1 |
|---------------|--------------------|
| short conversation, bounded turn count (e.g. <= 20 turns) | unbounded long sessions |
| fixed session lifetime (e.g. 30 minutes) + idle timeout | multi-day rooms |
| owner/admin room access plus configurable single or owner-and-leader conclusion approval | arbitrary quorum policies and external governance integration |
| chat persistence, audit trail, and lifecycle-end deterministic compression | periodic semantic compression for unbounded long sessions |
| basic unsafe-instruction filter (deny list) | adaptive policy / model-graded safety |
| Temporal workflow with signals, updates, and queries | per-tenant workflow isolation |
| WS ticket-based authenticated handshake | distributed session state across regions |

When the configured turn or time limit is reached, the workflow creates an
immutable source-bound conversation summary, persists session closure metadata,
and then runs the final notification step through the M2 IMProvider. Original
ChatTurn rows remain complete.

## Prerequisites

- M4 ContainerProvider produces stable, audit-friendly sandbox runs
- AuthProvider design is agreed (OIDC, owner / admin roles)
- Temporal basics are exercised by M1/M2 workflows; M5 adds dedicated
  Update/query/timer coverage for `DiagnosisRoomWorkflow`

## Deliverables

- AuthProvider interface and OIDC implementation
- RBAC enforcement: owner/admin room access plus dedicated owner/leader
  conclusion approval
- Next.js short-conversation diagnosis page
- WebSocket connection: browser <-> Go control plane (per-turn file contract via container)
- Temporal workflow that owns session lifecycle (Update + durable timer)
- ChatSession, ChatTurn, ChatSessionApproval, and immutable versioned
  ChatSessionSummary Ent schemas
- bounded-turn enforcement at the workflow level
- unsafe-instruction filter (deny-list, defense-in-depth)
- audit logging for session lifecycle events
- close audit payload with the latest assistant conclusion snapshot
- final group notification on session close

## Architecture

M5 uses per-turn container invocation, reusing the M4 batch model. Each user
message triggers a separate container run. No long-running container process.
See [architecture.md](../architecture.md) M5 Interactive Model and
[phases/04-ai-integration.md](04-ai-integration.md) M5 Per-Turn Call Chain.

```text
Browser (WebSocket)
    |
    v
Go Control Plane
    |-- AuthProvider: verify identity and role (OIDC)
    |-- RBAC: check room access and conclusion approval authority
    |-- Deny-list filter: block unsafe instructions before relay
    |-- Audit: log all lifecycle events to PostgreSQL
    |
    +-- Temporal DiagnosisRoomWorkflow (one instance per session)
            |-- Update: submit user message (primary path, synchronous response)
            |-- Signal: session terminated by user / close / cancel
            |-- Query: return current turn count, state, latest response
            |-- Timer: session lifetime ceiling
            |-- Timer: idle timeout
            |
            +-- Per-turn Activity:
                    ContainerProvider.Run("diagnosis-assistant", {
                      evidence, conversation_history, latest_message
                    })
                    -> container reads agent_config/ for role/skills/tools
                    -> agent reasons within single invocation
                    -> optionally appends semantic message previews to
                       /workspace/out/stream.ndjson
                    -> writes /workspace/out/output.json
                    -> Go validates + persists ChatTurn
    |
    v
Docker Sandbox (ContainerProvider, from M4)
    |-- /workspace/evidence.json      (readonly, original evidence)
    |-- /workspace/conversation.json  (readonly, all previous turns)
    |-- /workspace/message.json       (readonly, latest user message)
    |-- /workspace/agent_config/      (readonly, from agents/diagnosis-assistant/)
    |-- /workspace/out/stream.ndjson  (optional bounded transient preview)
    |-- /workspace/out/output.json    (agent writes response here, writable capped output mount)
    |-- timeout: turn-level (e.g. 2 min) + session-level ceiling
    |-- cleanup: deterministic after each turn
```

### Why Per-Turn (Not Long-Running)

* No stdin/stdout streaming protocol to design or maintain
* Crash recovery is trivial: replay from last persisted ChatTurn
* Container startup cost (~1-3s per turn) acceptable for V1 short-conversation
* Conversation state lives in Temporal workflow state (durable across crashes)
* Agent loads full context fresh each turn, consistent with stateless agent
  framework patterns
* Post-V1 optimization: switch to persistent container with HTTP endpoint
  inside, behind the same ContainerProvider interface

## Workflow Contract

The DiagnosisRoomWorkflow embodies the human-in-the-loop pattern that justifies
Temporal selection (see ADR-0004):

* a single workflow instance equals a single session
* Temporal Update delivers user messages and synchronously returns agent response
* queries return current state for reconnection
* durable timers implement lifetime and idle timeouts
* activities own all external I/O (sandbox call, persistence, notification)

### Policy Boundary

`internal/usecases/diagnosisroom` owns the pure M5 policy checks that will be
called by the WebSocket handler and the Temporal Update validator:

- configured maximum turns, session lifetime, idle timeout, turn timeout, and
  context/message byte caps
- duplicate `message_id` rejection before a turn is accepted
- in-flight turn rejection before starting another Activity
- session and idle timeout checks from caller-supplied `now` values
- deterministic context byte accounting for `/workspace/evidence.json`,
  `/workspace/conversation.json`, and `/workspace/message.json`
- basic unsafe-instruction denylist matching
- strict V1 sandbox `output.json` schema validation through
  `diagnosisroom.ParseTurnOutput`

The WebSocket relay performs frame-level validation only. The workflow Update
Validator remains the authoritative policy boundary, so reconnects and retries
cannot bypass turn limits, duplicate-message checks, unsafe-message checks, or
context-budget checks.

The focused gate is:

```bash
make diagnosis-room-policy-test
```

This is a policy and schema foundation used by the workflow and transport
relay.

### Temporal Room Workflow

[diagnosis_room_workflow.go](../../../internal/orchestrator/temporal/diagnosis_room_workflow.go)
owns the M5 room state machine through `DiagnosisRoomWorkflow`:

- `submit-turn` Update is the primary user-message path
- the Update Validator calls `diagnosisroom.ValidateSubmitTurn` so duplicate
  `message_id`, max-turn, unsafe-message, timeout, and context-budget checks
  reject before accepted Update history is written
- startup calls `EnsureDiagnosisChatSession` so the persisted session exists
  before Update handlers can write transcript rows; the Activity also records
  an idempotent `diagnosis_room.opened` audit event
- accepted Updates call `RunDiagnosisTurn`, which mounts the frozen evidence,
  prior conversation, and latest user message into `ContainerProvider.Run`
  with network-none defaults and a policy-derived turn timeout
- new workflow histories enable the optional streaming provider extension via
  a Temporal version marker; Activity heartbeats report only bounded progress
  metadata and preview text remains outside Workflow history and persistence
- sandbox output is accepted only after the V1 diagnosis-turn JSON Schema
  validates
- accepted user and assistant turns are persisted through
  `PersistDiagnosisTurn`, which is idempotent on per-session message IDs and
  advances `ChatSession.turn_count`; the Activity also records an idempotent
  `diagnosis_room.turn_persisted` audit event
- only after sandbox output and persistence both succeed does the workflow
  append the user+assistant turn pair to workflow state
- `state` Query returns reconnect/read state without mutating workflow state
- `close` and `cancel` Signals terminate the room explicitly; operator-requested
  close records `closed_by` without treating that actor as a conclusion
  confirmer
- durable timers close the room on fixed session lifetime or idle timeout
- an accepted Workflow Update protects the room from idle timeout until its
  handler and persistence Activities finish
- terminal close calls `CloseDiagnosisChatSession`, which persists
  `ChatSession.closed_at` / `close_reason` and records an idempotent
  `diagnosis_room.closed` audit event with a bounded `final_conclusion`
  snapshot from the latest persisted assistant turn, or `not_available` when
  the room closes before any assistant turn exists
- after close persistence succeeds, `SendDiagnosisRoomCloseNotification`
  reuses the M2 `IMProvider` with a diagnosis-task-scoped idempotency key,
  sends the alert-group close notification, and records
  `diagnosis_room.close_notification_sent`

The focused gate is:

```bash
make diagnosis-room-workflow-test
```

This is the Temporal control-plane plus per-turn sandbox, transcript
persistence, lifecycle audit, and final close-notification Activity boundary.

### Update Runtime Constraints

* **Update timeout**: WS handler sets a context timeout (~3 minutes) on the
  UpdateWorkflow call. If exceeded, the handler informs the user "still
  processing"; the workflow continues executing the turn.
* **Concurrent-turn rejection**: the Update Validator checks whether a turn
  Activity is already in flight and rejects with "turn in progress" if so.
* **Turn idempotency**: each turn carries a unique `message_id`. The Validator
  rejects duplicate IDs to prevent replayed messages on retry.
* **WS disconnect during wait**: the workflow completes the turn regardless.
  On reconnect, the WS handler calls Temporal Query to retrieve missed turns.

`internal/transport/http/diagnosis_ws_relay.go` implements this V1 relay:

- an authenticated connection receives a `ready` frame after ticket consumption
- `submit_turn` frames call `ports.DiagnosisRoomWorkflowClient.SubmitDiagnosisTurn`
  and return a `turn_result` frame; while the Update runs, validated transient
  snapshots may arrive as `turn_stream` frames
- `query_state` frames call `ports.DiagnosisRoomWorkflowClient.QueryDiagnosisRoom`
  and return a `state` frame for reconnect/read flows
- authorized `close_room` frames require the room-administer capability, inject
  the authenticated ticket subject as the close actor, and use a server-owned
  `user_requested` reason before waiting for terminal workflow state
- submit-turn waits use a bounded context decoupled from WebSocket disconnects;
  on timeout the client receives an `error` frame with `turn_still_processing`
- the process-local preview hub keeps one latest snapshot per subscriber, so a
  slow or disconnected browser cannot backpressure the Activity; reconnect
  recovery still comes from Temporal Query and persisted turns
- a preview write failure releases the WebSocket handler immediately while the
  bounded workflow Update continues independently to its durable outcome

`internal/orchestrator/temporal/diagnosis_room_client.go` is the Temporal
adapter behind that port. It uses `UpdateWorkflow` with
`WorkflowUpdateStageCompleted`, decodes the update result with
`WorkflowUpdateHandle.Get`, and decodes reconnect state with `QueryWorkflow`.

### Frontend Route

`web/src/app/diagnosis-room/page.tsx` is the thin App Router route for the M5
browser entry point. It delegates the ticket/bootstrap/transcript UI to
`web/src/features/diagnosis-room/`:

- ticket issuance uses the generated OpenAPI TypeScript contract for
  `POST /api/v1/diagnosis/ws-ticket`
- non-OpenAPI WebSocket frame types stay local to the diagnosis-room feature
- transient `turn_stream` snapshots render as a replaceable assistant draft;
  an explicit generation `reset` frame removes an invalid retry draft before
  corrected text arrives, and `turn_result` replaces the final draft
- operators with room-administer permission can explicitly close an idle room
  without approving its conclusion; close and confirmation requests are
  mutually blocked in the browser while either operation is pending
- exact room detail projects a bounded, sanitized audit timeline from immutable
  lifecycle events; room lists omit the timeline to avoid per-row audit reads
- the route smoke runs against a mocked API/WebSocket endpoint and proves
  `ready`, `state`, `submit_turn`, and `turn_result` in a production Next.js
  server

The focused browser gate is:

```bash
npm run smoke --prefix web
```

The live browser acceptance harness is separate from the mocked route smoke:

```bash
make diagnosis-live-browser-smoke
```

It requires `OPENCLARION_LIVE_API_BASE_URL`,
`OPENCLARION_LIVE_BEARER_TOKEN`, and either
`OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID` for an existing room or
`OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID` so the harness can create a room via
`POST /api/v1/diagnosis/rooms`. The target runs `npm run smoke:live` with
`web/playwright.live.config.ts` and writes a JSON proof after one browser
connect/query/submit/turn-result round trip against a real backend/worker
stack. The Playwright test writes structured browser observations for state
load, `turn_result`, submitted-message visibility, connected status after the
turn, browser-submitted message length and SHA-256 digest, assistant-turn count
increment, and user+assistant transcript pair increment. The proof is validated by
`scripts/diagnosis_live_smoke_output` before the target reports success; the
validator also checks that the digest matches the top-level retained metadata,
that session, workflow, and run IDs are single-line, whitespace-free, and
bounded, that the retained `evidence` summary is a bounded single-line
statement mentioning `turn_result`, that the completed turn log number matches
the assistant-turn count, and that transcript counts stay consistent with the
user+assistant pair model. A local retained proof has exercised this path against
a real backend/worker stack. Target-deployment acceptance still requires a fresh
retained proof when the backend, worker image, sandbox runtime, or routing
changes from the proven local stack.

### Runtime Wiring

`cmd/openclarion` now wires the M5 runtime path from environment variables:

- `OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL` and
  `OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID` enable the OIDC `AuthProvider`
- optional `OPENCLARION_DIAGNOSIS_OIDC_ROLE_CLAIM`,
  `OPENCLARION_DIAGNOSIS_OIDC_OWNER_ROLES`,
  `OPENCLARION_DIAGNOSIS_OIDC_ADMIN_ROLES`, and
  `OPENCLARION_DIAGNOSIS_OIDC_SIGNING_ALGS` customize role and verifier policy
- `OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS` enables exact-origin CORS for the
  ticket endpoint and WebSocket origin checks when the Next.js frontend and Go
  API are served from different browser origins
- `OPENCLARION_SANDBOX_IMAGE_REF` and
  `OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT` enable the Docker-backed
  `ContainerProvider` for per-turn `RunDiagnosisTurn` Activities
- optional `OPENCLARION_SANDBOX_WORKSPACE_ROOT`,
  `OPENCLARION_SANDBOX_EGRESS_ALLOWED`, and
  `OPENCLARION_SANDBOX_EGRESS_NETWORK` customize the sandbox runtime boundary
  when per-turn egress is enabled. The egress network must name a dedicated
  Docker network backed by the reviewed proxy or firewall boundary; Docker
  special modes such as `host`, `bridge`, and `none` are rejected for
  allowlist mode. Egress-enabled images must expose their runner through an
  exec-form image `ENTRYPOINT` and implement the `readiness` subcommand;
  `OPENCLARION_SANDBOX_COMMAND_JSON` overrides are rejected because they cannot
  preserve a portable image-level readiness contract.
- optional `OPENCLARION_AUTO_DIAGNOSIS_MAX_ROOMS_PER_TRIGGER` bounds automatic
  auto-room fan-out per alert intake or report-policy replay; it defaults to 3
  and may be raised up to 100 only when the LLM and sandbox capacity can absorb
  the extra concurrent first-turn workload

The runtime uses the same Temporal client for the worker,
`DiagnosisRoomClient`, and `DiagnosisRoomStarter`; the PostgreSQL-backed
diagnosis ticket store for short-lived WebSocket tickets; and a `ChatSession`
resolver keyed by the external `session_key`. `POST /api/v1/diagnosis/rooms`
authenticates the bearer principal, loads the frozen EvidenceSnapshot, starts
`DiagnosisRoomWorkflow`, waits until the workflow has materialized its
`DiagnosisTask` and `ChatSession`, and returns the session/workflow handle used
by ticket issuance and the live browser smoke harness.

### WebSocket Authentication

Browser `new WebSocket(url)` cannot set custom HTTP headers. V1 uses
ticket-based auth:

1. Browser calls `POST /api/v1/diagnosis/ws-ticket` (with OIDC Bearer
   token in header).
2. Server validates OIDC token, issues short-lived ticket (UUID, TTL=30s,
   single-use).
3. Browser opens
   `new WebSocket("wss://host/ws/diagnosis?session_id=<id>&ticket=xxx")`.
4. Go WS handler validates ticket (exists, not expired, not used), consumes it.
5. If invalid: reject upgrade with 401.

Do NOT put long-lived JWT in query string (appears in server logs, referrer
headers, and browser history).

`internal/usecases/diagnosisauth` owns the usecase-level boundary that backs
this handshake:

- `ports.AuthProvider` authenticates a bearer token into a provider-neutral
  principal with mapped roles
- `internal/providers/auth/oidc` implements that port by verifying signed OIDC
  ID tokens through issuer discovery/JWKS, enforcing client ID audience checks,
  and mapping configured role claims into owner/admin roles
- `internal/providers/auth/fake` provides deterministic scripted auth for
  transport/usecase tests
- owner/admin room access is enforced by `AuthorizeSessionAccess`
- local directory RBAC exposes a dedicated `diagnosis_room.approve` permission;
  the room owner receives that capability for the exact session and a leader
  may receive it through the persisted directory role
- `Service.IssueTicket` creates cryptographically random URL-safe tickets with
  TTL <= 30s after RBAC passes
- `Service.ConsumeTicket` atomically consumes one ticket and rechecks scope,
  session, expiry, and RBAC
- `internal/persistence/repository.NewDiagnosisAuthTicketStore` persists only
  `sha256(token)` plus ticket metadata in PostgreSQL and uses a conditional
  update so concurrent consumers produce only one winner
- `internal/transport/http` exposes the generated
  `POST /api/v1/diagnosis/ws-ticket` endpoint and the non-OpenAPI
  `GET /ws/diagnosis` upgrade route; the upgrade path validates same-origin
  browser requests before ticket consumption and hands off only redacted tickets
- `MemoryStore` remains available for pure tests/local development

The focused gate is:

```bash
make diagnosis-auth-test
```

This is the authentication, authorization, ticket, persistence, transport
handshake, and submit/query relay boundary. Lifecycle audit and close
notification are covered by `make diagnosis-room-workflow-test`, not this
auth gate.

### Chat Persistence and Conclusion Approval

`ChatSession`, `ChatTurn`, and `ChatSessionApproval` persist the M5
diagnosis-room lifecycle, transcript, and conclusion decisions behind
`ports.DiagnosisRepository`. The schema remains tied to the alert diagnosis
path by anchoring each `ChatSession` to one `DiagnosisTask`; `session_key` is
the external room id used by WebSocket ticket issuance and reconnect flows.

Persistence rules:

- `ChatSession.session_key` is immutable and globally unique
- one `DiagnosisTask` owns one `ChatSession` in V1
- `owner_subject` is immutable and backs owner/admin RBAC resolution
- `approval_mode` is immutable and selects `single` or `owner_and_leader`
- `ChatTurn` is append-only
- `(chat_session_id, message_id)` rejects duplicate submitted messages
- `(chat_session_id, sequence)` preserves canonical transcript order
- `ChatSessionApproval` is append-only and binds one actor and authority to the
  exact assistant turn and SHA-256 conclusion digest
- `(chat_session_id, conclusion_digest, actor_subject)` rejects replayed or
  conflicting approval by the same actor
- `(chat_session_id, conclusion_digest, authority)` allows only one actor to
  occupy each quorum slot

The focused gate is:

```bash
make diagnosis-chat-persistence-test
```

The schema/repository boundary and Temporal workflow wiring are both present:
`DiagnosisRoomWorkflow` creates/reuses the session and persists each accepted
user+assistant turn pair. The WebSocket relay now feeds submit/query frames
into this workflow, and lifecycle events are recorded in `DiagnosisTaskEvent`
with stable idempotency keys.

### Context Budget

Total context mounted into the container includes evidence, conversation
history, tool outputs, system prompt, and the latest message. A byte/token
budget is enforced at the workflow/Activity boundary before mounting:

- V1 rejects oversized turns with an explicit context-limit error.
- lifecycle-end compression creates a bounded retained read model but does not
  bypass the active-session context limit or discard source turns.
- future long-session validation may add periodic checkpoints and deterministic
  context selection before each turn.
- never silently pass oversized context to the LLM (causes truncation or
  failure).

## Security Model

- authorization is fail-closed (deny by default)
- sandbox inherits M4 security posture (non-root, resource limits, network
  allowlist)
- unsafe-instruction filter runs server-side before forwarding to sandbox
- all session actions are auditable
- terminal human-confirmed close re-reads the latest persisted assistant turn
  and its immutable approvals, then rejects close until the configured quorum
  is satisfied by distinct actors
- explicit operator close records the authenticated close actor separately and
  never manufactures a conclusion confirmation
- bounded turns and lifetime cap bound active-session context independently of
  lifecycle-end compression

## Acceptance

- authorized user opens a diagnosis room in the browser
- short-conversation exchange completes within configured turn and time limits
- chat turns persist to PostgreSQL with full audit trail
- unauthorized access is denied
- configured conclusion approval blocks terminal close until the latest
  assistant conclusion has the required distinct owner/leader approvals
- an authorized operator can explicitly close an idle room without confirming
  its conclusion, with the close actor retained in lifecycle audit
- session close triggers final group notification
- exact room detail exposes a bounded sanitized projection of queryable audit
  events

## Out-of-Scope Confirmation

- no periodic active-session compression for multi-day rooms
- no arbitrary quorum policy or external approval-governance integration
- no multi-day or multi-region session state
- no streaming token-level partial responses (turn-by-turn is sufficient)
