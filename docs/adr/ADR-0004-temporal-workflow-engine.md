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
* human-in-the-loop signals
* deterministic workflow replay
* clear separation between orchestration and activities

## Decision Outcome

**Chosen option**: use Temporal for durable workflow orchestration. Go services
own business decisions, Temporal owns orchestration, retry, timer, signal, and
activity execution semantics.

### Consequences

* Good, because retries and timers are explicit and testable.
* Good, because headless report fan-out/fan-in maps cleanly to workflows.
* Neutral, because Temporal adds an operational dependency.
* Bad, because developers must respect deterministic workflow restrictions.

### Confirmation

* no hand-written distributed state machine for diagnosis lifecycle
* workflow tests cover timeout, retry, and signal paths
* activities contain external I/O, workflows contain orchestration

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
