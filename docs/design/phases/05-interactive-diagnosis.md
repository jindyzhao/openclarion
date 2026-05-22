# Phase 05: Short-Conversation Interactive Diagnosis (M5)

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
| Temporal workflow with signals and queries | per-tenant workflow isolation |
| WebSocket proxy with authenticated handshake | distributed session state across regions |

When the configured turn or time limit is reached, the workflow transitions to
a final notification step (re-using the M2 IMProvider) and persists session
closure metadata. No automatic compression is attempted in V1.

## Prerequisites

- M4 ContainerProvider produces stable, audit-friendly sandbox runs
- AuthProvider design is agreed (OIDC, owner / admin roles)
- Temporal basics are exercised by M1/M2 workflows; M5 adds dedicated
  signal/query/timer coverage for `DiagnosisRoomWorkflow`

## Deliverables

- AuthProvider interface and OIDC implementation
- RBAC enforcement: owner and admin (leader is deferred)
- Next.js short-conversation diagnosis page
- WebSocket proxy: browser <-> Go control plane <-> sandbox stdin/stdout
- Temporal workflow that owns session lifecycle (signals + durable timer)
- ChatSession and ChatTurn Ent schemas
- bounded-turn enforcement at the workflow level
- unsafe-instruction filter (deny-list, defense-in-depth)
- audit logging for session lifecycle events
- final group notification on session close

## Architecture

```text
Browser (WebSocket)
    |
    v
Go Control Plane
    |-- AuthProvider: verify identity and role
    |-- RBAC: check session ownership
    |-- WebSocket proxy: bidirectional message relay
    |-- Audit: log lifecycle events
    |-- Filter: deny-list unsafe instructions before forward
    |
    +-- Temporal workflow (DiagnosisRoomWorkflow)
            |-- Signal: user message in
            |-- Signal: session terminated by user
            |-- Query: current turn count and state
            |-- Timer: session lifetime ceiling
            |-- Timer: idle timeout
            |-- Activity: forward to sandbox, persist turn
    |
    v
Docker Sandbox (ContainerProvider, from M4)
    |-- stdin: filtered user message
    |-- stdout: agent response (validated and persisted)
    |-- timeout: turn-level + session-level ceiling
    |-- cleanup: deterministic on session close
```

## Workflow Contract

The DiagnosisRoomWorkflow embodies the human-in-the-loop pattern that justifies
Temporal selection (see ADR-0004):

* a single workflow instance equals a single session
* signals deliver user messages
* queries return current state to the WebSocket proxy
* durable timers implement lifetime and idle timeouts
* activities own all external I/O (sandbox call, persistence, notification)

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
