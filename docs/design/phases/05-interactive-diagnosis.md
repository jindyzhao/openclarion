# Phase 05: Short-Conversation Interactive Diagnosis (M5)

> Realises the per-turn container invocation contract from
> [ADR-0013](../../adr/ADR-0013-per-turn-container-invocation.md). All
> per-turn lifecycle, isolation, and budget rules below derive from
> that ADR; conflicts must be resolved in favour of the ADR.

## Status

V1 required. Minimum-viable scope only. Long-session features are deferred.

## Goal

Implement a short-conversation diagnosis room where authorized users converse
with a sandboxed agent for a bounded number of turns within a fixed session
lifetime. The control plane owns identity, RBAC, audit, and lifecycle. The
sandbox owns the agent process.

## V1 Scope Boundary

| In Scope (V1) | Deferred Beyond V1 |
|---------------|--------------------|
| short conversation, bounded turn count (e.g. <= 20 turns) | unbounded long sessions |
| fixed session lifetime (e.g. 30 minutes) + idle timeout | multi-day rooms |
| owner / admin RBAC enforcement | leader-tier multi-stakeholder approval |
| chat persistence and audit trail | conversation compression and summarization |
| basic unsafe-instruction filter (deny list) | adaptive policy / model-graded safety |
| Temporal workflow with signals, updates, and queries | per-tenant workflow isolation |
| WS ticket-based authenticated handshake | distributed session state across regions |

When the configured turn or time limit is reached, the workflow transitions to
a final notification step (re-using the M2 IMProvider) and persists session
closure metadata. No automatic compression is attempted in V1.

## Prerequisites

- M4 ContainerProvider produces stable, audit-friendly sandbox runs
- AuthProvider design is agreed (OIDC, owner / admin roles)
- Temporal basics are exercised by M1/M2 workflows; M5 adds dedicated
  Update/query/timer coverage for `DiagnosisRoomWorkflow`

## Deliverables

- AuthProvider interface and OIDC implementation
- RBAC enforcement: owner and admin (leader is deferred)
- Next.js short-conversation diagnosis page
- WebSocket connection: browser <-> Go control plane (per-turn file contract via container)
- Temporal workflow that owns session lifecycle (Update + durable timer)
- ChatSession and ChatTurn Ent schemas
- bounded-turn enforcement at the workflow level
- unsafe-instruction filter (deny-list, defense-in-depth)
- audit logging for session lifecycle events
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
    |-- RBAC: check session ownership (owner/admin)
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
                    -> writes /workspace/out/output.json
                    -> Go validates + persists ChatTurn
    |
    v
Docker Sandbox (ContainerProvider, from M4)
    |-- /workspace/evidence.json      (readonly, original evidence)
    |-- /workspace/conversation.json  (readonly, all previous turns)
    |-- /workspace/message.json       (readonly, latest user message)
    |-- /workspace/agent_config/      (readonly, from agents/diagnosis-assistant/)
    |-- /workspace/out/output.json    (agent writes response here, writable tmpfs)
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

### WebSocket Authentication

Browser `new WebSocket(url)` cannot set custom HTTP headers. V1 uses
ticket-based auth:

1. Browser calls `POST /api/ws-ticket` (with OIDC Bearer token in header).
2. Server validates OIDC token, issues short-lived ticket (UUID, TTL=30s,
   single-use).
3. Browser opens `new WebSocket("wss://host/ws/diagnosis?ticket=xxx")`.
4. Go WS handler validates ticket (exists, not expired, not used), consumes it.
5. If invalid: reject upgrade with 401.

Do NOT put long-lived JWT in query string (appears in server logs, referrer
headers, and browser history).

### Context Budget

Total context mounted into the container includes evidence, conversation
history, tool outputs, system prompt, and the latest message. A byte/token
budget is enforced at the workflow level before mounting:

- if total exceeds budget: truncate oldest conversation turns (keep first +
  last N) or reject the turn with an explicit "context limit reached" response.
- never silently pass oversized context to the LLM (causes truncation or
  failure).

## Security Model

- authorization is fail-closed (deny by default)
- sandbox inherits M4 security posture (non-root, resource limits, network
  allowlist)
- unsafe-instruction filter runs server-side before forwarding to sandbox
- all session actions are auditable
- bounded turns and lifetime cap blast radius without requiring compression

## Acceptance

- authorized user opens a diagnosis room in the browser
- short-conversation exchange completes within configured turn and time limits
- chat turns persist to PostgreSQL with full audit trail
- unauthorized access is denied
- session close triggers final group notification
- audit events are queryable

## Out-of-Scope Confirmation

- no automatic conversation compression or summarization
- no leader-tier approval flows
- no multi-day or multi-region session state
- no streaming token-level partial responses (turn-by-turn is sufficient)
