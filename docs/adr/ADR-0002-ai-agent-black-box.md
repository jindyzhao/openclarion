---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0002: AI Agent Black-Box Runtime Boundary

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion needs AI-assisted diagnosis and reporting, but agent runtimes evolve
quickly and may carry powerful tool execution models. The project must decide
whether to modify agent internals or treat agents as replaceable black-box
runtimes.

## Decision Drivers

* preserve upstream upgrade paths
* keep Go control flow deterministic
* allow multiple LLM or agent backends
* constrain tool execution and output parsing
* support testable fallback providers

## Decision Outcome

**Chosen option**: use AI agents as black-box runtimes behind an `LLMProvider` or
sandboxed `ContainerProvider`. OpenClarion owns prompts, inputs, outputs,
timeouts, retries, audit, and lifecycle. It does not modify agent internals.

### Consequences

* Good, because agent upgrades do not require a fork.
* Good, because direct LLM providers can validate the reporting loop before any
  agent sandbox runtime is stable.
* Neutral, because prompt and output contracts become first-class artifacts.
* Bad, because black-box behavior can vary; golden prompt tests and structured
  output validation are mandatory mitigations.

### Confirmation

* core packages do not import agent runtime internals
* report generation goes through `LLMProvider` or `ContainerProvider`
* all AI output is parsed and validated before persistence

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
