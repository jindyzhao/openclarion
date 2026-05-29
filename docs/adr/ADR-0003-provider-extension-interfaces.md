---
id: ADR-0003
title: "Provider Extension Interfaces"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0003: Provider Extension Interfaces

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

Different deployments use different metrics systems, CMDBs, IM tools, identity
providers, approval processes, container runtimes, and LLM backends. OpenClarion
needs stable seams without hard-coding one organization's systems into the core.

## Decision Drivers

* keep the default distribution complete and runnable
* make integrations replaceable through typed interfaces
* avoid runtime plugin complexity in the first phase
* keep tests independent from external systems
* support public provider contributions later

## Decision Outcome

**Chosen option**: compile-time Provider interfaces with registry wiring and fake
implementations for tests.

### Provider Set

| Provider | Purpose |
|----------|---------|
| `MetricsProvider` | active alerts and metric queries |
| `CMDBProvider` | ownership and topology enrichment |
| `IMProvider` | report and escalation distribution |
| `AuthProvider` | identity and session bootstrap |
| `ApprovalProvider` | human approval for risky operations |
| `ContainerProvider` | sandbox lifecycle |
| `LLMProvider` | headless report generation |

### Consequences

* Good, because core workflows do not depend on concrete external systems.
* Good, because tests can run with fake providers.
* Neutral, because replacing providers requires rebuilding the binary.
* Bad, because cross-language providers require an adapter layer.

### Confirmation

* core workflow code depends on provider interfaces, not implementation packages
* default providers exist for local development
* fake providers exist for workflow tests

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
