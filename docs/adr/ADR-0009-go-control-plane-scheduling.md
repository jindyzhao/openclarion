---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0009: Go Control Plane Scheduling

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

The project needs deterministic alert ingestion, sharding, grouping, evidence
creation, and workflow dispatch. These concerns should not be delegated to an AI
agent.

## Decision Outcome

**Chosen option**: implement the control plane in Go. Go owns polling, sharding,
replay, grouping, idempotency, workflow start, and lifecycle decisions. AI
providers only analyze prepared evidence.

## Control Plane Responsibilities

* metrics provider polling
* alert fingerprinting and idempotent writes
* window replay harness
* sharding and grouping
* evidence snapshot creation
* workflow dispatch
* retry and failure marking
* report distribution handoff

### Confirmation

* replay tests cover at least 20 alert windows
* no AI provider is used for routing or lifecycle decisions
* workflow activity boundaries are explicit

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
