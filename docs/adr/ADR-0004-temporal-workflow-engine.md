---
id: ADR-0004
title: "Temporal Workflow Engine"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0004: Temporal Workflow Engine

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

Alert governance requires timers, retries, human interaction, long-running sessions,
fan-out/fan-in report generation, and durable failure handling. A hand-written
state machine would be fragile.

## Decision Drivers

* durable workflow state
* retry and timeout support
* synchronous human-in-the-loop Updates (M5 short-conversation diagnosis)
* deterministic workflow replay
* clear separation between orchestration and activities
* SDK must support Workflow Update (>= 1.21)

## Why Not a Simpler Job Queue

A PostgreSQL-native job queue (e.g. River + sqlc) would be sufficient for
M0-M2 (alert grouping, evidence build, headless report fan-out/fan-in). It
would not satisfy M5 short-conversation interactive diagnosis without
reimplementing core workflow primitives:

* synchronous Updates from a human user delivered into a running session
  (request-response semantics; caller blocks until workflow handler completes)
* queries against in-flight session state
* durable timers for session lifetime and idle timeout
* deterministic replay across crash recovery

Because M5 is a V1 commitment (see ADR-0006), introducing two engines or
migrating later costs more than carrying Temporal from M0. The trade-off is
more operational weight in M0-M2 in exchange for a single coherent engine
through M5.

## Re-evaluation Trigger

If the V1 scope ever drops M5, this ADR must be revisited. River + sqlc would
then become the more proportionate choice for the report-only path. The 1-week
spike comparison documented in DEFERRED_FOLLOWUPS.md remains a useful
cross-check.

## Decision Outcome

**Chosen option**: use Temporal for durable workflow orchestration. Go services
own business decisions, Temporal owns orchestration, retry, timer, Update,
Signal, and activity execution semantics.

### Communication Mechanisms

| Mechanism | Purpose | M5 Usage |
|-----------|---------|----------|
| **Update** (primary) | synchronous request-response into running workflow | user message → per-turn Activity → response returned to WS handler |
| **Signal** (secondary) | fire-and-forget notification | close/cancel session, fallback when Update unavailable |
| **Query** | read-only state inspection | retrieve missed turns on reconnect |

Workflow Update (Temporal SDK >= 1.21) provides the synchronous push semantics
required by M5 interactive diagnosis: the WS handler sends an Update and blocks
until the per-turn Activity completes, then pushes the result to the browser.

### Consequences

* Good, because retries and timers are explicit and testable.
* Good, because headless report fan-out/fan-in maps cleanly to workflows.
* Good, because M5 short-conversation diagnosis lands on the same engine
  through Updates, Signals, and Queries (no engine migration).
* Neutral, because Temporal adds an operational dependency.
* Bad, because developers must respect deterministic workflow restrictions.
* Bad, because M0-M2 carries more orchestration weight than a simple job
  queue would require.

### SDK Version Constraint

Temporal Go SDK must be pinned to >= 1.21 (Workflow Update support). The
Temporal Server must also support Update (>= 1.21). This is validated by
the first real workflow that lands during M1-PR3 (the `DiagnosisWorkflow`
shell), per the ADR-0012 amendment that moved this validation out of M0
(the Go SDK enters `go.mod` only at that PR per the first-import rule).

### Confirmation

* no hand-written distributed state machine for diagnosis lifecycle
* workflow tests cover timeout, retry, Update, and Signal paths
* activities contain external I/O, workflows contain orchestration
* M1-PR3 first-real-workflow integration test validates Update round-trip
  (send Update -> handler executes Activity -> result returned to caller);
  the Go SDK itself enters `go.mod` only at that PR per the first-import rule

## More Information

### Related Decisions

* ADR-0013 — per-turn container invocation model (uses Update for synchronous turn dispatch)
* ADR-0002 — agent black-box boundary (Temporal mediates, never exposes agent internals)

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Document River+sqlc alternative; tie selection to M5 V1 commitment; add re-evaluation trigger |
| 2026-05-19 | jindyzhao | Signal → Update as primary M5 path; add SDK version constraint (>= 1.21); add communication mechanisms table |
| 2026-05-22 | jindyzhao | Update round-trip validation home moved from M0 to M1-PR3 first real workflow (`DiagnosisWorkflow` shell), aligning with the ADR-0012 amendment and the first-import rule (Temporal Go SDK enters `go.mod` only when M1-PR3 production code first imports it) |
