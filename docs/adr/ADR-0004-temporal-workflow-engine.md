---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0004: Temporal Workflow Engine

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

Alert governance requires timers, retries, human signals, long-running sessions,
fan-out/fan-in report generation, and durable failure handling. A hand-written
state machine would be fragile.

## Decision Drivers

* durable workflow state
* retry and timeout support
* human-in-the-loop signals (M5 short-conversation diagnosis)
* deterministic workflow replay
* clear separation between orchestration and activities

## Why Not a Simpler Job Queue

A PostgreSQL-native job queue (e.g. River + sqlc) would be sufficient for
M0-M2 (alert grouping, evidence build, headless report fan-out/fan-in). It
would not satisfy M5 short-conversation interactive diagnosis without
reimplementing core workflow primitives:

* signals from a human user delivered into a running session
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
own business decisions, Temporal owns orchestration, retry, timer, signal, and
activity execution semantics.

### Consequences

* Good, because retries and timers are explicit and testable.
* Good, because headless report fan-out/fan-in maps cleanly to workflows.
* Good, because M5 short-conversation diagnosis lands on the same engine
  through signals and queries (no engine migration).
* Neutral, because Temporal adds an operational dependency.
* Bad, because developers must respect deterministic workflow restrictions.
* Bad, because M0-M2 carries more orchestration weight than a simple job
  queue would require.

### Confirmation

* no hand-written distributed state machine for diagnosis lifecycle
* workflow tests cover timeout, retry, and signal paths
* activities contain external I/O, workflows contain orchestration

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Document River+sqlc alternative; tie selection to M5 V1 commitment; add re-evaluation trigger |
